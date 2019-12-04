// Copyright (c) 2019 Ilya Mateyko
//
// The MIT License (MIT)
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

// cloudshell is the Google Cloud Shell CLI.
package main // import "github.com/astrophena/cloudshell"

import (
	"fmt"
	"os"

	"github.com/astrophena/cloudshell/internal/commands"

	"github.com/urfave/cli"
)

var Version = "dev"

func main() {
	app := cli.NewApp()

	app.Name = "cloudshell"
	app.Usage = "manage Google Cloud Shell"
	app.EnableBashCompletion = true
	app.Version = Version // Generated with `script/build`.
	app.Authors = []cli.Author{
		cli.Author{
			Name:  "Ilya Mateyko",
			Email: "inbox@astrophena.me",
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
					Usage:   "Adds a public SSH key to an Cloud Shell",
					Flags: []cli.Flag{
						cli.StringFlag{
							Name:     "format, f",
							Usage:    "Format of the key's content",
							Required: true,
						},
						cli.StringFlag{
							Name:     "key, k",
							Usage:    "Content of the key",
							Required: true,
						},
					},
					Action: commands.KeyAdd,
				},
				{
					Name:    "delete",
					Aliases: []string{"d"},
					Usage:   "Removes a public SSH key from an Cloud Shell",
					Flags: []cli.Flag{
						cli.StringFlag{
							Name:     "id, i",
							Usage:    "ID of a public key to delete",
							Required: true,
						},
					},
					Action: commands.KeyDelete,
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
