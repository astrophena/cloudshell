// © 2025 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"go.astrophena.name/base/cli"
	"go.astrophena.name/base/logger"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	cloudshell "google.golang.org/api/cloudshell/v1"
	"google.golang.org/api/option"
)

func main() { cli.Main(new(app)) }

type app struct {
	// configuration
	stateDir string

	// initialized by Run
	logf        logger.Logf
	oauthConfig *oauth2.Config
	svc         *cloudshell.Service
}

func (a *app) Run(ctx context.Context) error {
	env := cli.GetEnv(ctx)

	a.logf = env.Logf

	xdgStateDir := env.Getenv("XDG_STATE_HOME")
	if xdgStateDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		xdgStateDir = filepath.Join(home, ".local", "state")
	}
	a.stateDir = filepath.Join(xdgStateDir, "cloudshell")
	if err := os.MkdirAll(a.stateDir, 0o700); err != nil {
		return err
	}

	clientSecret, err := os.ReadFile(filepath.Join(a.stateDir, "client_secret.json"))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("client_secret.json is missing in %s, see https://go.astrophena.name/cloudshell#hdr-Setup for setup instructions", a.stateDir)
		}
		return err
	}
	a.oauthConfig, err = google.ConfigFromJSON(clientSecret, cloudshell.CloudPlatformScope)
	if err != nil {
		return err
	}
	tok, err := a.getToken(ctx)
	if err != nil {
		return err
	}
	a.svc, err = cloudshell.NewService(ctx, option.WithTokenSource(a.oauthConfig.TokenSource(ctx, tok)))
	if err != nil {
		return err
	}

	if len(env.Args) == 0 {
		return fmt.Errorf("%w: command is required, see -help for usage", cli.ErrInvalidArgs)
	}
	command := env.Args[0]
	args := env.Args[1:]

	switch command {
	case "info":
		return a.info()
	case "ssh":
		return a.ssh(ctx)
	case "start":
		return a.start(ctx)
	case "key":
		if len(args) == 0 {
			return fmt.Errorf("%w: subcommand for 'key' is required (list, add, remove)", cli.ErrInvalidArgs)
		}
		subcommand := args[0]
		subargs := args[1:]
		switch subcommand {
		case "list":
			return a.keyList()
		case "add":
			if len(subargs) == 0 {
				return fmt.Errorf("%w: public key is required", cli.ErrInvalidArgs)
			}
			return a.keyAdd(subargs[0])
		case "remove":
			if len(subargs) == 0 {
				return fmt.Errorf("%w: public key is required", cli.ErrInvalidArgs)
			}
			return a.keyRemove(subargs[0])
		default:
			return fmt.Errorf("%w: unknown subcommand %q for key", cli.ErrInvalidArgs, subcommand)
		}
	default:
		return fmt.Errorf("%w: no such command %q", cli.ErrInvalidArgs, command)
	}
}

func (a *app) getToken(ctx context.Context) (*oauth2.Token, error) {
	env := cli.GetEnv(ctx)

	tokenFile := filepath.Join(a.stateDir, "token.json")

	tokb, err := os.ReadFile(tokenFile)
	if err == nil {
		var tok oauth2.Token
		if err := json.Unmarshal(tokb, &tok); err == nil {
			return &tok, nil
		}
	}

	authURL := a.oauthConfig.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Fprintf(env.Stderr, "Go to the following link in your browser then type the authorization code: %v\n", authURL)

	var authCode string
	if _, err := fmt.Fscan(env.Stdin, &authCode); err != nil {
		return nil, err
	}

	newtok, err := a.oauthConfig.Exchange(ctx, authCode)
	if err != nil {
		return nil, err
	}
	tokb, err = json.MarshalIndent(newtok, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(tokenFile, tokb, 0o600); err != nil {
		return nil, err
	}

	return newtok, nil
}

const envName = "users/me/environments/default"

func (a *app) info() error {
	env, err := a.svc.Users.Environments.Get(envName).Do()
	if err != nil {
		return err
	}

	state := strings.ToLower(env.State)
	state = lowercaseFirst(state) + "."
	a.logf(state)

	a.logf("Docker image: %s", env.DockerImage)

	if env.SshHost != "" && env.SshPort != 0 && env.SshUsername != "" {
		a.logf("SSH connection details:")
		a.logf("  Host:     %s", env.SshHost)
		a.logf("  Port:     %d", env.SshPort)
		a.logf("  Username: %s", env.SshUsername)
	} else {
		a.logf("SSH is unavailable.")
	}

	return nil
}

func lowercaseFirst(s string) string {
	if len(s) == 0 {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

func (a *app) ssh(ctx context.Context) error {
	env, err := a.svc.Users.Environments.Get(envName).Do()
	if err != nil {
		return err
	}

	if env.State != "RUNNING" {
		if err := a.start(ctx); err != nil {
			return err
		}
		env, err = a.svc.Users.Environments.Get(envName).Do()
		if err != nil {
			return err
		}
	}

	return a.sshExec(env)
}

func (a *app) start(ctx context.Context) error {
	if _, err := a.svc.Users.Environments.Start(envName, &cloudshell.StartEnvironmentRequest{}).Do(); err != nil {
		return err
	}

	a.logf("Environment is starting...")

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		e, err := a.svc.Users.Environments.Get(envName).Do()
		if err != nil {
			return err
		}

		if e.State == "RUNNING" {
			a.logf("Environment has started.")
			return nil
		}

		select {
		case <-ticker.C:
			continue
		case <-ctx.Done():
			return nil
		}
	}
}

func (a *app) sshExec(e *cloudshell.Environment) error {
	if e.SshHost == "" || e.SshPort == 0 || e.SshUsername == "" {
		return errors.New("ssh is unavailable")
	}

	host := e.SshUsername + "@" + e.SshHost
	port := strconv.FormatInt(e.SshPort, 10)

	path, err := exec.LookPath("ssh")
	if err != nil {
		return err
	}

	cmd := exec.Command(path, host, "-p", port, "-o", "StrictHostKeyChecking=no")

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	return cmd.Run()
}

func (a *app) keyList() error {
	e, err := a.svc.Users.Environments.Get(envName).Do()
	if err != nil {
		return err
	}

	if len(e.PublicKeys) == 0 {
		a.logf("No public keys found.")
		return nil
	}

	for _, k := range e.PublicKeys {
		a.logf(k)
	}

	return nil
}

func (a *app) keyAdd(key string) error {
	_, err := a.svc.Users.Environments.AddPublicKey(envName, &cloudshell.AddPublicKeyRequest{
		Key: strings.TrimSuffix(key, "\n"),
	}).Do()
	if err != nil {
		return err
	}

	a.logf("Public key added successfully.")
	return nil
}

func (a *app) keyRemove(key string) error {
	if _, err := a.svc.Users.Environments.RemovePublicKey(envName, &cloudshell.RemovePublicKeyRequest{Key: key}).Do(); err != nil {
		return err
	}

	a.logf("Public key removed successfully.")
	return nil
}
