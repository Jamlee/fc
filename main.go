package main

import (
	"log"
	"os"

	"github.com/Jamlee/fastvpn/pkg/vpn"
	"github.com/urfave/cli"
)

func main() {
	app := cli.NewApp()
	app.Usage = "creating a vpn server fastly"
	app.Version = "0.2.0"
	app.Commands = []cli.Command{
		{
			Name:  "server",
			Usage: "start the vpn server service",
			Action: func(c *cli.Context) error {
				// start the vpn server
				// upload the vpn server program
				// start the vpn
				server, err := vpn.NewServer("0.0.0.0", "9001", "192.168.45.1/24", "tun0")
				if err == nil {
					server.Run()
				}
				return err
			},
		},
		{
			Name:  "run",
			Usage: "deploy the vpn server and start the vpn client ",
			Action: func(c *cli.Context) error {
				// run in the local host
				vpn.NewServer("127.0.0.1", "9000", "0.0.0.0/8", "tun0")
				return nil
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
