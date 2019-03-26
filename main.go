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
				server, err := vpn.NewServer("0.0.0.0", "9001", "192.168.45.1/24", "tun1")
				if err == nil {
					server.Run()
				}
				return err
			},
		},
		{
			Name:  "client",
			Usage: "start the vpn client service",
			Action: func(c *cli.Context) error {
				server, err := vpn.NewServer("0.0.0.0", "9001", "192.168.45.1/24", "tun1")
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
				return nil
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
