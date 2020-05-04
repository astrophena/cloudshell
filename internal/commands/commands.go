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

const startingMsg = "Cloud Shell is starting. Run \"cloudshell connect\" again in a few minutes."

// Info implements the "info" command.
func Info(c *cli.Context) (err error) {
	s, err := auth.Service()
	if err != nil {
		return err
	}

	name, err := environment.Name()
	if err != nil {
		return err
	}

	env, err := s.Users.Environments.Get(name).Do()
	if err != nil {
		return err
	}

	fmt.Printf("State: %s\n", env.State)
	fmt.Printf("Docker Image: %s\n", env.DockerImage)
	if env.SshHost != "" {
		fmt.Printf("SSH Host: %s\n", env.SshHost)
	}
	if env.SshPort != 0 {
		fmt.Printf("SSH Port: %d\n", env.SshPort)
	}
	if env.SshUsername != "" {
		fmt.Printf("SSH Username: %s\n", env.SshUsername)
	}
	if env.WebHost != "" {
		fmt.Printf("Web Host: %s\n", env.WebHost)
	}

	return nil
}

// Connect implements the "connect" command.
func Connect(c *cli.Context) (err error) {
	s, err := auth.Service()
	if err != nil {
		return err
	}

	name, err := environment.Name()
	if err != nil {
		return err
	}

	env, err := s.Users.Environments.Get(name).Do()
	if err != nil {
		return err
	}

	switch env.State {
	case "RUNNING":
		if err := environment.Connect(s); err != nil {
			return err
		}
	case "STARTING":
		log.Println(startingMsg)
	case "DISABLED":
		if err := environment.Start(s); err != nil {
			return err
		}
		log.Println(startingMsg)
	}

	return nil
}

// KeyList implements the "key list" command.
func KeyList(c *cli.Context) (err error) {
	s, err := auth.Service()
	if err != nil {
		return err
	}

	name, err := environment.Name()
	if err != nil {
		return err
	}

	env, err := s.Users.Environments.Get(name).Do()
	if err != nil {
		return err
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"ID", "Format", "Key"})
	table.SetBorder(false)

	for _, publicKey := range env.PublicKeys {
		id := strings.Split(publicKey.Name, "/")
		preview := publicKey.Key[:10] + "..."
		table.Append([]string{id[5], publicKey.Format, preview})
	}

	table.Render()

	return nil
}

// KeyAdd implements the "key add" command.
func KeyAdd(c *cli.Context) (err error) {
	s, err := auth.Service()
	if err != nil {
		return err
	}

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

	name, err := environment.Name()
	if err != nil {
		return err
	}

	_, err = s.Users.Environments.PublicKeys.Create(name, r).Do()
	if err != nil {
		return err
	}

	return nil
}

// KeyDelete implements the "key delete" command.
func KeyDelete(c *cli.Context) (err error) {
	s, err := auth.Service()
	if err != nil {
		return err
	}

	keyID := c.Args().Get(0)
	if keyID == "" {
		return errors.New("key id is required")
	}

	name, err := environment.Name()
	if err != nil {
		return err
	}

	id := fmt.Sprintf("%s/publicKeys/%s", name, keyID)

	_, err = s.Users.Environments.PublicKeys.Delete(id).Do()
	if err != nil {
		return err
	}

	return nil
}
