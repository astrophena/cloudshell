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
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"go.astrophena.name/base/cli"
	"go.astrophena.name/base/logger"
	"go.astrophena.name/base/request"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

func main() { cli.Main(new(app)) }

type app struct {
	// configuration
	stateDir string

	// initialized by Run
	httpc       *http.Client
	logf        logger.Logf
	oauthConfig *oauth2.Config
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
	a.oauthConfig, err = google.ConfigFromJSON(clientSecret, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return err
	}
	tok, err := a.getToken(ctx)
	if err != nil {
		return err
	}
	a.httpc = a.oauthConfig.Client(ctx, tok)

	if len(env.Args) == 0 {
		return fmt.Errorf("%w: command is required, see -help for usage", cli.ErrInvalidArgs)
	}
	command := env.Args[0]
	args := env.Args[1:]

	switch command {
	case "info":
		return a.info(ctx)
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
			return a.keyList(ctx)
		case "add":
			if len(subargs) == 0 {
				return fmt.Errorf("%w: public key is required", cli.ErrInvalidArgs)
			}
			return a.keyAdd(ctx, subargs[0])
		case "remove":
			if len(subargs) == 0 {
				return fmt.Errorf("%w: public key is required", cli.ErrInvalidArgs)
			}
			return a.keyRemove(ctx, subargs[0])
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

type environment struct {
	// Full path to the Docker image used to run this environment, e.g. "gcr.io/dev-con/cloud-devshell:latest".
	DockerImage string `json:"dockerImage"`

	// Output only:

	// Current execution state of this environment.
	State string `json:"state"`
	// Host to which clients can connect to initiate HTTPS or WSS connections with the environment.
	WebHost string `json:"webHost"`
	// Username that clients should use when initiating SSH sessions with the environment.
	SSHUsername string `json:"sshUsername"`
	// Host to which clients can connect to initiate SSH sessions with the environment.
	SSHHost string `json:"sshHost"`
	// Port to which clients can connect to initiate SSH sessions with the environment.
	SSHPort int `json:"sshPort"`
	// Public keys associated with the environment.
	PublicKeys []string `json:"publicKeys"`
}

func (a *app) getEnvironment(ctx context.Context) (environment, error) {
	return makeRequest[environment](ctx, a.httpc, http.MethodGet, "", nil)
}

func makeRequest[Response any](ctx context.Context, httpc *http.Client, method, url string, body any) (Response, error) {
	const baseURL = "https://cloudshell.googleapis.com/v1/users/me/environments/default"
	return request.Make[Response](ctx, request.Params{
		Method:     method,
		URL:        baseURL + url,
		Body:       body,
		HTTPClient: httpc,
	})
}

func (a *app) info(ctx context.Context) error {
	env, err := a.getEnvironment(ctx)
	if err != nil {
		return err
	}

	state := strings.ToLower(env.State)
	state = uppercaseFirst(state) + "."
	a.logf(state)

	a.logf("Docker image: %s", env.DockerImage)

	if env.SSHHost != "" && env.SSHPort != 0 && env.SSHUsername != "" {
		a.logf("SSH connection details:")
		a.logf("  Host:     %s", env.SSHHost)
		a.logf("  Port:     %d", env.SSHPort)
		a.logf("  Username: %s", env.SSHUsername)
	} else {
		a.logf("SSH is unavailable.")
	}

	return nil
}

func uppercaseFirst(s string) string {
	if len(s) == 0 {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func (a *app) ssh(ctx context.Context) error {
	env, err := a.getEnvironment(ctx)
	if err != nil {
		return err
	}

	if env.State != "RUNNING" {
		if err := a.start(ctx); err != nil {
			return err
		}
		env, err = a.getEnvironment(ctx)
		if err != nil {
			return err
		}
	}

	return a.sshExec(env)
}

func (a *app) start(ctx context.Context) error {
	if _, err := makeRequest[request.IgnoreResponse](ctx, a.httpc, http.MethodPost, ":start", struct{}{}); err != nil {
		return err
	}

	a.logf("Environment is starting...")

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		e, err := a.getEnvironment(ctx)
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

func (a *app) sshExec(e environment) error {
	if e.SSHHost == "" || e.SSHPort == 0 || e.SSHUsername == "" {
		return errors.New("ssh is unavailable")
	}

	host := e.SSHUsername + "@" + e.SSHHost
	port := strconv.FormatInt(int64(e.SSHPort), 10)

	cmd := exec.Command("ssh", host, "-p", port, "-o", "StrictHostKeyChecking=no")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func (a *app) keyList(ctx context.Context) error {
	e, err := a.getEnvironment(ctx)
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

func (a *app) keyAdd(ctx context.Context, key string) error {
	if _, err := makeRequest[request.IgnoreResponse](ctx, a.httpc, http.MethodPost, ":addPublicKey", struct {
		Key string `json:"key"`
	}{
		Key: strings.TrimSuffix(key, "\n"),
	}); err != nil {
		return err
	}
	a.logf("Public key added successfully.")
	return nil
}

func (a *app) keyRemove(ctx context.Context, key string) error {
	if _, err := makeRequest[request.IgnoreResponse](ctx, a.httpc, http.MethodPost, ":removePublicKey", struct {
		Key string `json:"key"`
	}{
		Key: strings.TrimSuffix(key, "\n"),
	}); err != nil {
		return err
	}
	a.logf("Public key removed successfully.")
	return nil
}
