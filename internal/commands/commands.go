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

// Package commands implements CLI commands.
package commands // import "go.astrophena.me/cloudshell/internal/commands"

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"go.astrophena.me/cloudshell/internal/auth"
	"go.astrophena.me/cloudshell/internal/environment"

	"github.com/olekukonko/tablewriter"
	"github.com/urfave/cli"
	cloudshell "google.golang.org/api/cloudshell/v1alpha1"
)

// Info implements `info` command.
func Info(c *cli.Context) error {
	s := auth.Service()

	e, err := s.Users.Environments.Get(environment.Name()).Do()
	if err != nil {
		return err
	}

	// Print information about the current environment.
	fmt.Printf("Docker image: %s\n", e.DockerImage)
	fmt.Printf("SSH host: %s\n", e.SshHost)
	fmt.Printf("SSH port: %s\n", strconv.FormatInt(e.SshPort, 10))
	fmt.Printf("SSH username: %s\n", e.SshUsername)
	fmt.Printf("State: %s\n", e.State)
	fmt.Printf("Web host: %s\n", e.WebHost)

	return nil
}

// SSH implements `ssh` command.
func SSH(c *cli.Context) error {
	s := auth.Service()

	e, err := s.Users.Environments.Get(environment.Name()).Do()
	if err != nil {
		return err
	}

	switch e.State {
	case "RUNNING":
		environment.Connect(s)
	case "STARTING":
		environment.Wait(s)
		environment.Connect(s)
	case "DISABLED":
		log.Println("==> Starting Cloud Shell...")
		environment.Start(s)
		environment.Wait(s)
		environment.Connect(s)
	}

	return nil
}

// KeyList implements `key list` command.
func KeyList(c *cli.Context) error {
	s := auth.Service()

	e, err := s.Users.Environments.Get(environment.Name()).Do()
	if err != nil {
		return err
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"ID", "Format", "Key"})
	table.SetBorder(false)

	for _, k := range e.PublicKeys {
		id := strings.Split(k.Name, "/")
		snip := k.Key[:10] + "..."
		table.Append([]string{id[5], k.Format, snip})
	}

	table.Render()

	return nil
}

// KeyAdd implements `key add` command.
func KeyAdd(c *cli.Context) error {
	s := auth.Service()

	format := c.Args().Get(0)
	key := c.Args().Get(1)

	if format == "" {
		return errors.New("key format is required")
	}
	if key == "" {
		return errors.New("key is required")
	}

	k := &cloudshell.PublicKey{
		Format: format,
		Key:    key,
	}

	r := &cloudshell.CreatePublicKeyRequest{
		Key: k,
	}

	_, err := s.Users.Environments.PublicKeys.Create(environment.Name(), r).Do()
	if err != nil {
		return err
	}

	return nil
}

// KeyDelete implements `key delete` command.
func KeyDelete(c *cli.Context) error {
	s := auth.Service()

	keyID := c.Args().Get(0)
	if keyID == "" {
		return errors.New("key id is required")
	}

	id := fmt.Sprintf("%s/publicKeys/%s", environment.Name(), keyID)
	_, err := s.Users.Environments.PublicKeys.Delete(id).Do()
	if err != nil {
		return err
	}

	return nil
}
