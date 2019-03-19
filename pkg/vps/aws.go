package vps

import (
	"net"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"golang.org/x/crypto/ssh"

	"fmt"
	"log"
)

const region = "us-east-2"
const instanceName = "fastvpn"
const command = `
	sudo sed -i -e '$a deb [trusted=yes] http://shadowvpn.org/debian wheezy main' /etc/apt/sources.list
	sudo apt-get update 
	sudo apt-get install iproute2 shadowvpn -y
	sudo service shadowvpn restart
`

var svc *ec2.EC2

func init() {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(region)},
	)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	svc = ec2.New(sess)
}

func findRunningVM() []*ec2.Instance {
	runResult, _ := svc.DescribeInstances(&ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			&ec2.Filter{
				Name:   aws.String("tag:name"),
				Values: []*string{aws.String(instanceName)},
			},
		},
	})
	var instances []*ec2.Instance
	for _, reservations := range runResult.Reservations {
		vm := reservations.Instances[0]
		if *vm.State.Code == *aws.Int64(16) {
			instances = append(instances, vm)
		}
	}
	return instances
}

func createKey() []byte {
	log.Print("begin create key pari for vm")
	result, err := svc.CreateKeyPair(&ec2.CreateKeyPairInput{
		KeyName: aws.String(instanceName),
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok && aerr.Code() == "InvalidKeyPair.Duplicate" {
			deleteKey()
			return createKey()
		} else {
			log.Printf("Unable to create key pair: %s, %v.", instanceName, err)
			os.Exit(1)
		}
	}
	return []byte(*result.KeyMaterial)
}

func deleteKey() {
	svc.DeleteKeyPair(&ec2.DeleteKeyPairInput{
		KeyName: aws.String(instanceName),
	})
}

func createSc() error {
	deleteSc()

	log.Print("try begin create security for vm")
	result, err := svc.DescribeVpcs(nil)
	if err != nil {
		log.Fatalf("Unable to describe VPCs, %v", err)
	}
	if len(result.Vpcs) == 0 {
		log.Fatalf("No VPCs found to associate security group with.")
	}
	vpcID := aws.StringValue(result.Vpcs[0].VpcId)
	_, err = svc.CreateSecurityGroup(&ec2.CreateSecurityGroupInput{
		GroupName:   aws.String(instanceName),
		Description: aws.String(instanceName),
		VpcId:       aws.String(vpcID),
	})
	log.Printf("create sc for vpc: %s", vpcID)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case "InvalidVpcID.NotFound":
				log.Fatal(err)
			case "InvalidGroup.Duplicate":
				deleteSc()
				return createSc()
			}
		}
		log.Fatal(err)
	}

	_, err = svc.AuthorizeSecurityGroupIngress(&ec2.AuthorizeSecurityGroupIngressInput{
		GroupName: aws.String(instanceName),
		IpPermissions: []*ec2.IpPermission{
			(&ec2.IpPermission{}).
				SetIpProtocol("-1").
				SetFromPort(0).
				SetToPort(65535).
				SetIpRanges([]*ec2.IpRange{
					{CidrIp: aws.String("0.0.0.0/0")},
				}),
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	return nil
}

func deleteSc() {
	_, err := svc.DeleteSecurityGroup(&ec2.DeleteSecurityGroupInput{
		GroupName: aws.String(instanceName),
	})
	if err != nil {
		switch err.(awserr.Error).Code() {
		case "InvalidGroup.NotFound":
			return
		case "DependencyViolation":
			log.Println("wait for vm to be delete")
			time.Sleep(5 * time.Second)
		default:
			log.Fatalln(err.(awserr.Error).Code())
		}
	}
}

// create and start the interface
func StartInstance() error {
	// clear
	vms := findRunningVM()
	for _, vm := range vms {
		svc.TerminateInstances(&ec2.TerminateInstancesInput{
			InstanceIds: []*string{vm.InstanceId},
		})
	}

	// create security group
	createSc()

	// set ssh client
	auth := make([]ssh.AuthMethod, 0)
	key := createKey()
	log.Println("\n========================\n" + string(key))
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		log.Fatal(err)
	}
	auth = append(auth, ssh.PublicKeys(signer))
	config := &ssh.ClientConfig{
		// Change to your username
		User:    "ubuntu",
		Timeout: 10 * time.Second,
		Auth:    auth,
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			return nil
		},
	}

	// start vm
	runResult, err := svc.RunInstances(&ec2.RunInstancesInput{
		ImageId:        aws.String("ami-0653e888ec96eab9b"),
		InstanceType:   aws.String("t2.nano"),
		MinCount:       aws.Int64(1),
		MaxCount:       aws.Int64(1),
		KeyName:        aws.String(instanceName),
		SecurityGroups: []*string{aws.String(instanceName)},
	})
	if err != nil {
		log.Fatal(err)
		return err
	}

	// add tags to the created instance
	_, errtag := svc.CreateTags(&ec2.CreateTagsInput{
		Resources: []*string{runResult.Instances[0].InstanceId},
		Tags: []*ec2.Tag{
			{
				Key:   aws.String("name"),
				Value: aws.String(instanceName),
			},
		},
	})
	if errtag != nil {
		log.Fatal(errtag)
	}
	log.Printf("%s created\n", *runResult.Instances[0].InstanceId)

	// init the vm with init script
	var vm *ec2.Instance
	for {
		log.Println("waiting for vm start")
		vms := findRunningVM()
		if len(vms) >= 1 {
			vm = vms[0]
			break
		}
		time.Sleep(5 * time.Second)
	}
	addr := *vm.PublicIpAddress + ":22"
	log.Printf("connect to %s", addr)
	var sshClient *ssh.Client
	for {
		sshClient, err = ssh.Dial("tcp", addr, config)
		if err == nil {
			break
		}
		log.Println(err)
		log.Println("wait for ssh server starting")
		time.Sleep(5 * time.Second)
	}

	session, err := sshClient.NewSession()
	defer session.Close()

	session.Stdout = os.Stdout
	session.Stderr = os.Stderr
	session.Stdin = os.Stdin

	log.Println("run command")
	err = session.Run(command)

	if err != nil {
		log.Panic(err)
	}
	return nil
}

// get vm running status
func StatusInstance() {
	vms := findRunningVM()
	for _, vm := range vms {
		log.Printf("%s %s %s \n", *vm.InstanceId, *vm.State.Name, *vm.PublicIpAddress)
	}
}

// stio and delete interface
func StopInstance() {
	deleteKey()
	vms := findRunningVM()
	for _, vm := range vms {
		svc.TerminateInstances(&ec2.TerminateInstancesInput{
			InstanceIds: []*string{vm.InstanceId},
		})
		log.Printf("%s stopped\n", *vm.InstanceId)
	}
	deleteSc()
}
