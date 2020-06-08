// © 2019 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the MIT
// license that can be found in the LICENSE.md file.

// Package commands implements commands of the cloudshell's command
// line interface.
package commands // import "github.com/astrophena/cloudshell/internal/commands"

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/astrophena/cloudshell/internal/auth"
	"github.com/astrophena/cloudshell/internal/environment"

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
	table.SetHeader([]string{"ID", "Type"})
	table.SetBorder(false)

	for _, publicKey := range env.PublicKeys {
		id := strings.Split(publicKey.Name, "/")
		table.Append([]string{id[5], publicKey.Format})
	}

	table.Render()

	return nil
}

// KeyAdd implements the "key add" command.
func KeyAdd(c *cli.Context) (err error) {
	key := c.Args().Get(0)

	if key == "" {
		return errors.New("key add: key is required")
	}

	ks := strings.Split(key, " ")

	// See https://cloud.google.com/shell/docs/reference/rest/Shared.Types/Format
	// for supported key types.
	var kf string
	switch ks[0] {
	case "ssh-dss":
		kf = "SSH_DSS"
	case "ssh-rsa":
		kf = "SSH_RSA"
	case "ecdsa-sha2-nistp256":
		kf = "ECDSA_SHA2_NISTP256"
	case "ecdsa-sha2-nistp384":
		kf = "ECDSA_SHA2_NISTP384"
	case "ecdsa-sha2-nistp521":
		kf = "ECDSA_SHA2_NISTP521"
	default:
		return errors.New("key add: format is unsupported")
	}

	s, err := auth.Service()
	if err != nil {
		return err
	}

	r := &cloudshell.CreatePublicKeyRequest{
		Key: &cloudshell.PublicKey{
			Format: kf,
			Key:    ks[1],
		},
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
	id := c.Args().Get(0)
	if id == "" {
		return errors.New("id is required")
	}

	name, err := environment.Name()
	if err != nil {
		return err
	}

	s, err := auth.Service()
	if err != nil {
		return err
	}

	if _, err := s.Users.Environments.PublicKeys.Delete(
		fmt.Sprintf("%s/publicKeys/%s", name, id),
	).Do(); err != nil {
		return err
	}

	return nil
}
