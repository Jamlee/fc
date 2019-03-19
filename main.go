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
			Name:  "start",
			Usage: "start the vpn server and connect to the vpn",
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
