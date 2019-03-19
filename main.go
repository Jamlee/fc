package main

import (
	"log"
	"os"

	"github.com/Jamlee/fastvpn/pkg/vpn"
	"github.com/Jamlee/fastvpn/pkg/vps"
	"github.com/urfave/cli"
)

func main() {
	app := cli.NewApp()
	app.Usage = "creating a vpn server fastly"
	app.Version = "0.2.0"
	app.Commands = []cli.Command{
		{
			Name:  "start",
			Usage: "start the fastvpn instance",
			Action: func(c *cli.Context) error {
				vps.StartInstance()
				return nil
			},
		},
		{
			Name:  "status",
			Usage: "get running vm status",
			Action: func(c *cli.Context) error {
				vps.StatusInstance()
				return nil
			},
		},
		{
			Name:  "stop",
			Usage: "stop running vm",
			Action: func(c *cli.Context) error {
				vps.StopInstance()
				return nil
			},
		},
		{
			Name:  "start-server",
			Usage: "start the vpn server",
			Action: func(c *cli.Context) error {
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
