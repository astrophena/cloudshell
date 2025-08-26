// © 2025 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unicode"

	"go.astrophena.name/base/cli"
	"go.astrophena.name/base/logger"
	"go.astrophena.name/base/request"

	"golang.org/x/crypto/ssh"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"golang.org/x/term"
)

func main() { cli.Main(new(app)) }

type app struct {
	// configuration
	stateDir       string
	privateKeyPath string // path to the managed private SSH key

	// initialized by Run
	httpc       *http.Client
	logf        logger.Logf
	oauthConfig *oauth2.Config
	authed      bool
}

func (a *app) Run(ctx context.Context) error {
	env := cli.GetEnv(ctx)

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

// ensureSSHKey checks for the existence of an RSA key pair in the state directory.
// If it doesn't exist, it generates a new 4096-bit RSA key pair.
func (a *app) ensureSSHKey() error {
	a.privateKeyPath = filepath.Join(a.stateDir, "key")
	publicKeyPath := filepath.Join(a.stateDir, "key.pub")

	if _, err := os.Stat(a.privateKeyPath); err == nil {
		return nil
	}

	a.logf("Generating a new SSH key pair for Cloud Shell...")

	// Generate private key.
	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return fmt.Errorf("failed to generate RSA key: %w", err)
	}

	// Encode private key to PEM format.
	privateKeyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}
	if err := os.WriteFile(a.privateKeyPath, pem.EncodeToMemory(privateKeyPEM), 0o600); err != nil {
		return fmt.Errorf("failed to write private key: %w", err)
	}

	// Generate and write public key in OpenSSH format.
	pub, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return fmt.Errorf("failed to create public key: %w", err)
	}
	publicKeyBytes := ssh.MarshalAuthorizedKey(pub)
	if err := os.WriteFile(publicKeyPath, publicKeyBytes, 0o644); err != nil {
		return fmt.Errorf("failed to write public key: %w", err)
	}

	a.logf("Key pair saved to %s and %s.", a.privateKeyPath, publicKeyPath)
	return nil
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

	// Start a local server to listen for the OAuth callback.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("could not start local server: %w", err)
	}
	defer l.Close()
	a.oauthConfig.RedirectURL = fmt.Sprintf("http://%s", l.Addr().String())

	// Channel to receive the authorization code.
	codeCh := make(chan string)
	// Channel to signal server shutdown.
	shutdownCh := make(chan struct{})

	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			code := r.URL.Query().Get("code")
			if code == "" {
				http.Error(w, "code not found", http.StatusBadRequest)
				return
			}
			fmt.Fprintln(w, "Authentication successful! You can close this window now.")
			codeCh <- code
			// Signal server to shutdown.
			shutdownCh <- struct{}{}
		}),
	}

	// Start the server in a goroutine.
	go func() {
		if err := srv.Serve(l); err != http.ErrServerClosed {
			a.logf("local server error: %v", err)
		}
	}()

	// Shutdown the server gracefully when signaled.
	go func() {
		select {
		case <-shutdownCh:
			if err := srv.Shutdown(ctx); err != nil {
				a.logf("local server shutdown error: %v", err)
			}
		case <-ctx.Done():
			return
		}
	}()

	authURL := a.oauthConfig.AuthCodeURL("state-token", oauth2.AccessTypeOffline)

	// Try to open the browser automatically.
	var opened bool
	switch runtime.GOOS {
	case "linux", "android":
		if _, err := exec.LookPath("xdg-open"); err == nil {
			if err := exec.Command("xdg-open", authURL).Start(); err == nil {
				opened = true
			}
		}
	case "darwin":
		if _, err := exec.LookPath("open"); err == nil {
			if err := exec.Command("open", authURL).Start(); err == nil {
				opened = true
			}
		}
	}

	if !opened {
		fmt.Fprintf(env.Stderr, "Go to the following link in your browser: %v\n", authURL)
	}

	select {
	case authCode := <-codeCh:
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
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type environment struct {
	DockerImage string   `json:"dockerImage"`
	State       string   `json:"state"`
	WebHost     string   `json:"webHost"`
	SSHUsername string   `json:"sshUsername"`
	SSHHost     string   `json:"sshHost"`
	SSHPort     int      `json:"sshPort"`
	PublicKeys  []string `json:"publicKeys"`
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

func (a *app) initClient(ctx context.Context) error {
	if a.authed {
		return nil
	}

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
	a.authed = true

	return nil
}

func (a *app) info(ctx context.Context) error {
	if err := a.initClient(ctx); err != nil {
		return err
	}

	env, err := a.getEnvironment(ctx)
	if err != nil {
		return err
	}

	state := strings.ToLower(env.State)
	state = uppercaseFirst(state) + "."
	a.logf(state)

	a.logf("Docker image: %s", env.DockerImage)

	if env.WebHost != "" {
		a.logf("Web host: %s", env.WebHost)
	}

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
	if err := a.initClient(ctx); err != nil {
		return err
	}
	if err := a.start(ctx); err != nil {
		return err
	}
	env, err := a.getEnvironment(ctx)
	if err != nil {
		return err
	}
	return a.sshExec(ctx, env)
}

func (a *app) start(ctx context.Context) error {
	if err := a.initClient(ctx); err != nil {
		return err
	}
	if err := a.ensureSSHKey(); err != nil {
		return fmt.Errorf("failed to ensure SSH key: %w", err)
	}

	publicKeyPath := filepath.Join(a.stateDir, "key.pub")
	pubKeyBytes, err := os.ReadFile(publicKeyPath)
	if err != nil {
		return fmt.Errorf("could not read managed public key: %w", err)
	}
	type startRequest struct {
		PublicKeys []string `json:"publicKeys"`
	}
	req := startRequest{
		// Cloud Shell API returns Internal Server Error when SSH public key has a
		// newline in the end. So trim it.
		PublicKeys: []string{strings.TrimSuffix(string(pubKeyBytes), "\n")},
	}
	if _, err := makeRequest[request.IgnoreResponse](ctx, a.httpc, http.MethodPost, ":start", req); err != nil {
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

// sshExec establishes an interactive SSH session using the native Go SSH client.
func (a *app) sshExec(ctx context.Context, e environment) error {
	env := cli.GetEnv(ctx)

	if e.SSHHost == "" || e.SSHPort == 0 || e.SSHUsername == "" {
		return errors.New("connection with SSH is unavailable")
	}

	// Read and parse the private key for authentication.
	key, err := os.ReadFile(a.privateKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read private key: %w", err)
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}

	config := &ssh.ClientConfig{
		User: e.SSHUsername,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		// Equivalent to "-o StrictHostKeyChecking=no". This is safe because
		// the host is provided by the trusted Google Cloud API.
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	addr := net.JoinHostPort(e.SSHHost, fmt.Sprintf("%d", e.SSHPort))
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return fmt.Errorf("failed to dial: %w", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	// Set up an interactive terminal.
	// Get the file descriptor for the standard input.
	fd := int(os.Stdin.Fd())
	// Check if we are running in a terminal.
	if !term.IsTerminal(fd) {
		return errors.New("standard input is not a terminal, cannot start interactive ssh session")
	}

	// Put the local terminal into "raw mode". This is crucial for passing
	// control characters (like Ctrl+C) to the remote shell.
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("failed to set terminal to raw mode: %w", err)
	}
	// Restore the terminal state when we're done.
	defer term.Restore(fd, oldState)

	// Get the terminal dimensions.
	width, height, err := term.GetSize(fd)
	if err != nil {
		return fmt.Errorf("failed to get terminal size: %w", err)
	}

	// Request a pseudo-terminal (PTY) from the remote server.
	// "xterm-256color" is a common and safe terminal type to request.
	if err := session.RequestPty("xterm-256color", height, width, ssh.TerminalModes{}); err != nil {
		return fmt.Errorf("failed to request pty: %w", err)
	}

	// Handle terminal resizing.
	// Create a channel to receive window change signals.
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	go func() {
		for range winch {
			w, h, err := term.GetSize(fd)
			if err != nil {
				continue
			}
			// Send a "window-change" request to the remote server.
			session.WindowChange(h, w)
		}
	}()

	// Connect local I/O to the remote session.
	session.Stdout = env.Stdout
	session.Stderr = env.Stderr
	session.Stdin = env.Stdin

	if err := session.Shell(); err != nil {
		return fmt.Errorf("failed to start shell: %w", err)
	}

	// Wait for the session to finish. The error returned by Wait()
	// is the exit status of the remote command.
	return session.Wait()
}

func (a *app) keyList(ctx context.Context) error {
	if err := a.initClient(ctx); err != nil {
		return err
	}

	env := cli.GetEnv(ctx)
	e, err := a.getEnvironment(ctx)
	if err != nil {
		return err
	}
	if len(e.PublicKeys) == 0 {
		a.logf("No public keys found.")
		return nil
	}
	for _, k := range e.PublicKeys {
		fmt.Fprintf(env.Stdout, "%s\n", k)
	}
	return nil
}

func (a *app) keyAdd(ctx context.Context, key string) error {
	if err := a.initClient(ctx); err != nil {
		return err
	}

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
	if err := a.initClient(ctx); err != nil {
		return err
	}

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
