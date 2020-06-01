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

// Version is a version of cloudshell.
var Version = "dev"

func main() {
	log.SetFlags(0)

	app := &cli.App{
		Name:                 "cloudshell",
		Usage:                "Manage Google Cloud Shell.",
		EnableBashCompletion: true,
		Version:              Version,
		HideHelpCommand:      true,
		Commands: []*cli.Command{
			{
				Name:    "connect",
				Aliases: []string{"c"},
				Usage:   "Establish an interactive SSH session with Cloud Shell",
				Action:  commands.Connect,
			},
			{
				Name:    "info",
				Aliases: []string{"i"},
				Usage:   "Print information about the environment",
				Action:  commands.Info,
			},
			{
				Name:            "key",
				Aliases:         []string{"k"},
				Usage:           "Manage public keys associated with the Cloud Shell",
				HideHelpCommand: true,
				Subcommands: []*cli.Command{
					{
						Name:    "list",
						Aliases: []string{"l"},
						Usage:   "List public keys associated with the Cloud Shell",
						Action:  commands.KeyList,
					},
					{
						Name:      "add",
						Aliases:   []string{"a"},
						Usage:     "Add a public SSH key to the Cloud Shell",
						ArgsUsage: "[public key, e.g. $(cat ~/.ssh/id_rsa.pub)]",
						Action:    commands.KeyAdd,
					},
					{
						Name:      "delete",
						Aliases:   []string{"d"},
						Usage:     "Remove a public SSH key from the Cloud Shell",
						ArgsUsage: "[key id]",
						Action:    commands.KeyDelete,
					},
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
