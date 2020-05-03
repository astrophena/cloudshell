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
