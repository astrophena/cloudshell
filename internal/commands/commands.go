// © 2019 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the MIT
// license that can be found in the LICENSE.md file.

// Package commands implements commands of the cloudshell's command
// line interface.
package commands // import "go.astrophena.me/cloudshell/internal/commands"

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"go.astrophena.me/cloudshell/internal/auth"
	"go.astrophena.me/cloudshell/internal/environment"

	"github.com/olekukonko/tablewriter"
	"github.com/urfave/cli/v2"
	cloudshell "google.golang.org/api/cloudshell/v1alpha1"
)

// Info implements the "info" command.
func Info(c *cli.Context) (err error) {
	s := auth.Service()

	e, err := s.Users.Environments.Get(environment.Name()).Do()
	if err != nil {
		return err
	}

	fmt.Printf("State: %s\n", e.State)
	fmt.Printf("Docker Image: %s\n", e.DockerImage)
	if e.SshHost != "" {
		fmt.Printf("SSH Host: %s\n", e.SshHost)
	}
	if e.SshPort != 0 {
		fmt.Printf("SSH Port: %d\n", e.SshPort)
	}
	if e.SshUsername != "" {
		fmt.Printf("SSH Username: %s\n", e.SshUsername)
	}
	if e.WebHost != "" {
		fmt.Printf("Web Host: %s\n", e.WebHost)
	}

	return nil
}

// Connect implements the "connect" command.
func Connect(c *cli.Context) (err error) {
	s := auth.Service()

	env, err := s.Users.Environments.Get(environment.Name()).Do()
	if err != nil {
		return err
	}

	startingMsg := "Cloud Shell is starting. Run \"cloudshell connect\" again in a few minutes."

	switch env.State {
	case "RUNNING":
		if err := environment.Connect(s); err != nil {
			return err
		}
	case "STARTING":
		log.Println(startingMsg)
	case "DISABLED":
		environment.Start(s)
		log.Println(startingMsg)
	}

	return nil
}

// KeyList implements the "key list" command.
func KeyList(c *cli.Context) (err error) {
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

// KeyAdd implements the "key add" command.
func KeyAdd(c *cli.Context) (err error) {
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

	_, err = s.Users.Environments.PublicKeys.Create(environment.Name(), r).Do()
	if err != nil {
		return err
	}

	return nil
}

// KeyDelete implements the "key delete" command.
func KeyDelete(c *cli.Context) (err error) {
	s := auth.Service()

	keyID := c.Args().Get(0)
	if keyID == "" {
		return errors.New("key id is required")
	}

	id := fmt.Sprintf("%s/publicKeys/%s", environment.Name(), keyID)

	_, err = s.Users.Environments.PublicKeys.Delete(id).Do()
	if err != nil {
		return err
	}

	return nil
}
