// © 2019 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the MIT
// license that can be found in the LICENSE.md file.

// cloudshell is the Google Cloud Shell CLI.
package main // import "go.astrophena.me/cloudshell"

import (
	"log"
	"os"

	"go.astrophena.me/cloudshell/internal/commands"

	"github.com/urfave/cli/v2"
)

var Version = "dev"

func main() {
	log.SetFlags(0)

	app := cli.NewApp()

	app.Name = "cloudshell"
	app.Usage = "Manage Google Cloud Shell."
	app.EnableBashCompletion = true
	app.Version = Version
	app.HideHelpCommand = true
	app.Commands = []*cli.Command{
		&cli.Command{
			Name:    "connect",
			Aliases: []string{"c"},
			Usage:   "Establish an interactive SSH session with Cloud Shell",
			Action:  commands.Connect,
		},
		&cli.Command{
			Name:    "info",
			Aliases: []string{"i"},
			Usage:   "Print information about the environment",
			Action:  commands.Info,
		},
		&cli.Command{
			Name:            "key",
			Aliases:         []string{"k"},
			Usage:           "Manage public keys associated with the Cloud Shell",
			HideHelpCommand: true,
			Subcommands: []*cli.Command{
				&cli.Command{
					Name:    "list",
					Aliases: []string{"l"},
					Usage:   "List public keys associated with the Cloud Shell",
					Action:  commands.KeyList,
				},
				&cli.Command{
					Name:      "add",
					Aliases:   []string{"a"},
					Usage:     "Add a public SSH key to the Cloud Shell",
					ArgsUsage: "[key format] [key]",
					Action:    commands.KeyAdd,
				},
				&cli.Command{
					Name:      "delete",
					Aliases:   []string{"d"},
					Usage:     "Remove a public SSH key from the Cloud Shell",
					ArgsUsage: "[key id]",
					Action:    commands.KeyDelete,
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
