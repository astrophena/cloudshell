// © 2019 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the MIT
// license that can be found in the LICENSE.md file.

// cloudshell is the Google Cloud Shell CLI.
package main // import "go.astrophena.me/cloudshell"

import (
	"fmt"
	"os"

	"go.astrophena.me/cloudshell/internal/commands"

	"github.com/urfave/cli"
)

var Version = "dev"

func main() {
	app := cli.NewApp()

	app.Name = "cloudshell"
	app.Usage = "Manage Google Cloud Shell."
	app.EnableBashCompletion = true
	app.Version = Version // Generated with `script/build`.
	app.Authors = []cli.Author{
		cli.Author{
			Name:  "Ilya Mateyko",
			Email: "me@astrophena.me",
		},
	}
	app.Copyright = "(c) 2019 Ilya Mateyko. Licensed under the MIT License."

	app.Commands = []cli.Command{
		{
			Name:    "info",
			Aliases: []string{"i"},
			Usage:   "Print information about the environment",
			Action:  commands.Info,
		},
		{
			Name:    "ssh",
			Aliases: []string{"s"},
			Usage:   "Establish an interactive SSH session with Cloud Shell",
			Action:  commands.SSH,
		},
		{
			Name:    "key",
			Aliases: []string{"k"},
			Usage:   "Manage public keys associated with the Cloud Shell",
			Subcommands: []cli.Command{
				{
					Name:    "list",
					Aliases: []string{"l"},
					Usage:   "List public keys associated with the Cloud Shell",
					Action:  commands.KeyList,
				},
				{
					Name:    "add",
					Aliases: []string{"a"},
					Usage:   "Add a public SSH key to the Cloud Shell",
					Action:  commands.KeyAdd,
				},
				{
					Name:    "delete",
					Aliases: []string{"d"},
					Usage:   "Remove a public SSH key from the Cloud Shell",
					Action:  commands.KeyDelete,
				},
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
