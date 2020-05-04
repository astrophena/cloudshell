// © 2019 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the MIT
// license that can be found in the LICENSE.md file.

// Package environment implements functions for managing Cloud Shell.
package environment // import "go.astrophena.me/cloudshell/internal/environment"

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"time"

	"go.astrophena.me/cloudshell/internal/auth"

	cloudshell "google.golang.org/api/cloudshell/v1alpha1"
)

// Name returns a name of the default environment.
func Name() string {
	return fmt.Sprintf("users/%s/environments/default", auth.Email())
}

// Start starts an existing environment.
func Start(s *cloudshell.Service) {
	_, err := s.Users.Environments.Start(Name(), &cloudshell.StartEnvironmentRequest{}).Do()
	if err != nil {
		log.Fatal(err)
	}
}

// Connect connects to the environment via SSH.
func Connect(s *cloudshell.Service) {
	e, err := s.Users.Environments.Get(Name()).Do()
	if err != nil {
		log.Fatal(err)
	}

	host := fmt.Sprintf("%s@%s", e.SshUsername, e.SshHost)
	port := strconv.FormatInt(e.SshPort, 10)

	path, err := exec.LookPath("ssh")
	if err != nil {
		log.Fatal(err)
	}

	cmd := exec.Command(path, host, "-p", port, "-o", "StrictHostKeyChecking=no")

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}
}

// Wait polls for state of booting environment.
func Wait(s *cloudshell.Service) {
	e, err := s.Users.Environments.Get(Name()).Do()
	if err != nil {
		log.Fatal(err)
	}

	if e.State == "STARTING" {
		for {
			e, err := s.Users.Environments.Get(Name()).Do()
			if err != nil {
				log.Fatal(err)
			}

			if e.State == "RUNNING" {
				break
			} else {
				time.Sleep(3 * time.Second)
			}
		}
	}
}
