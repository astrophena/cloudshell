// © 2019 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the MIT
// license that can be found in the LICENSE.md file.

// Package environment implements functions for managing Cloud Shell.
package environment // import "go.astrophena.me/cloudshell/internal/environment"

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"go.astrophena.me/cloudshell/internal/auth"

	cloudshell "google.golang.org/api/cloudshell/v1alpha1"
)

// Name returns a name of the default environment or an error.
func Name() (name string, err error) {
	email, err := auth.Email()
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("users/%s/environments/default", email), nil
}

// Start starts the default environment.
func Start(s *cloudshell.Service) (err error) {
	name, err := Name()
	if err != nil {
		return err
	}

	r := &cloudshell.StartEnvironmentRequest{}

	if _, err := s.Users.Environments.Start(name, r).Do(); err != nil {
		return err
	}

	return nil
}

// Connect connects to the default environment via SSH.
func Connect(s *cloudshell.Service) (err error) {
	name, err := Name()
	if err != nil {
		return err
	}

	env, err := s.Users.Environments.Get(name).Do()
	if err != nil {
		return err
	}

	host := fmt.Sprintf("%s@%s", env.SshUsername, env.SshHost)
	port := strconv.FormatInt(env.SshPort, 10)

	path, err := exec.LookPath("ssh")
	if err != nil {
		return err
	}

	cmd := exec.Command(path, host, "-p", port, "-o", "StrictHostKeyChecking=no")

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}
