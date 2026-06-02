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
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode"

	"go.astrophena.name/base/cli"
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
	privateKeyPath string

	// flags
	jsonOutput     bool
	profile        string
	localForward   string
	dynamicForward string

	// initialized by initClient
	authed      bool
	httpc       *http.Client
	logf        func(format string, args ...any)
	oauthConfig *oauth2.Config
}

func (a *app) Flags(fs *flag.FlagSet) {
	fs.BoolVar(&a.jsonOutput, "json", false, "Output in JSON format.")
	fs.StringVar(&a.profile, "profile", "default", "Profile to use.")
	fs.StringVar(&a.localForward, "L", "", "Local port forwarding (e.g., 8080:localhost:8080).")
	fs.StringVar(&a.dynamicForward, "D", "", "Dynamic port forwarding (SOCKS5 proxy, e.g., 1080).")
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
		return a.ssh(ctx, args...)
	case "ssh-proxy":
		return a.sshProxy(ctx, args...)
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

	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return fmt.Errorf("failed to generate RSA key: %w", err)
	}

	privateKeyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}
	if err := os.WriteFile(a.privateKeyPath, pem.EncodeToMemory(privateKeyPEM), 0o600); err != nil {
		return fmt.Errorf("failed to write private key: %w", err)
	}

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

	var tokenFile string
	if a.profile == "default" || a.profile == "" {
		tokenFile = filepath.Join(a.stateDir, "token.json")
	} else {
		tokenFile = filepath.Join(a.stateDir, "profiles", a.profile+".json")
	}

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

	var (
		codeCh     = make(chan string)
		shutdownCh = make(chan struct{})
	)

	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			code := r.URL.Query().Get("code")
			if code == "" {
				http.Error(w, "code not found", http.StatusBadRequest)
				return
			}
			fmt.Fprintln(w, "Authentication successful! You can close this window now.")
			codeCh <- code
			shutdownCh <- struct{}{}
		}),
	}

	go func() {
		if err := srv.Serve(l); err != http.ErrServerClosed {
			a.logf("local server error: %v", err)
		}
	}()
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
		if err := os.MkdirAll(filepath.Dir(tokenFile), 0o700); err != nil {
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

	a.logf = log.New(env.Stderr, "", 0).Printf

	xdgStateDir := env.Getenv("XDG_STATE_HOME")
	if xdgStateDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		xdgStateDir = filepath.Join(home, ".local", "state")
	}
	a.stateDir = filepath.Join(xdgStateDir, "cloudshell")
	if runtime.GOOS == "windows" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return err
		}
		a.stateDir = filepath.Join(configDir, "cloudshell")
	}
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

	if a.jsonOutput {
		enc := json.NewEncoder(cli.GetEnv(ctx).Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(env)
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

func (a *app) ssh(ctx context.Context, args ...string) error {
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
	return a.sshExec(ctx, env, args...)
}

func (a *app) sshProxy(ctx context.Context, args ...string) error {
	if err := a.initClient(ctx); err != nil {
		return err
	}
	if err := a.start(ctx); err != nil {
		return err
	}
	e, err := a.getEnvironment(ctx)
	if err != nil {
		return err
	}
	if e.SSHHost == "" || e.SSHPort == 0 {
		return errors.New("connection with SSH is unavailable")
	}

	addr := net.JoinHostPort(e.SSHHost, fmt.Sprintf("%d", e.SSHPort))
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to connect to SSH host: %w", err)
	}
	defer conn.Close()

	env := cli.GetEnv(ctx)

	errc := make(chan error, 2)
	go func() {
		_, err := io.Copy(conn, env.Stdin)
		errc <- err
	}()
	go func() {
		_, err := io.Copy(env.Stdout, conn)
		errc <- err
	}()

	return <-errc
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

func (a *app) sshExec(ctx context.Context, e environment, args ...string) error {
	if e.SSHHost == "" || e.SSHPort == 0 || e.SSHUsername == "" {
		return errors.New("connection with SSH is unavailable")
	}

	if sshBin, err := exec.LookPath("ssh"); err == nil {
		return a.sshExecBinary(ctx, sshBin, e, args...)
	}

	return a.sshExecGo(ctx, e, args...)
}

func (a *app) sshExecBinary(ctx context.Context, sshBin string, e environment, args ...string) error {
	env := cli.GetEnv(ctx)

	cmdArgs := []string{
		"-i", a.privateKeyPath,
		"-p", fmt.Sprintf("%d", e.SSHPort),
		"-o", "StrictHostKeyChecking=no",
	}

	if a.localForward != "" {
		cmdArgs = append(cmdArgs, "-L", a.localForward)
	}

	if a.dynamicForward != "" {
		cmdArgs = append(cmdArgs, "-D", a.dynamicForward)
	}

	target := fmt.Sprintf("%s@%s", e.SSHUsername, e.SSHHost)
	cmdArgs = append(cmdArgs, target)
	cmdArgs = append(cmdArgs, args...)

	cmd := exec.CommandContext(ctx, sshBin, cmdArgs...)
	cmd.Stdin = env.Stdin
	cmd.Stdout = env.Stdout
	cmd.Stderr = env.Stderr

	return cmd.Run()
}

func (a *app) sshExecGo(ctx context.Context, e environment, args ...string) error {
	env := cli.GetEnv(ctx)

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

	if a.localForward != "" {
		go a.startLocalForward(client, a.localForward)
	}
	if a.dynamicForward != "" {
		go a.startDynamicForward(client, a.dynamicForward)
	}

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	session.Stdout = env.Stdout
	session.Stderr = env.Stderr
	session.Stdin = env.Stdin

	if len(args) > 0 {
		return session.Run(strings.Join(args, " "))
	}

	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return errors.New("standard input is not a terminal, cannot start interactive ssh session")
	}

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("failed to set terminal to raw mode: %w", err)
	}
	defer term.Restore(fd, oldState)

	width, height, err := term.GetSize(fd)
	if err != nil {
		return fmt.Errorf("failed to get terminal size: %w", err)
	}

	if err := session.RequestPty("xterm-256color", height, width, ssh.TerminalModes{}); err != nil {
		return fmt.Errorf("failed to request pty: %w", err)
	}

	notifySigWinch(fd, session)

	if err := session.Shell(); err != nil {
		return fmt.Errorf("failed to start shell: %w", err)
	}

	return session.Wait()
}

func (a *app) startLocalForward(client *ssh.Client, forward string) {
	parts := strings.Split(forward, ":") // format is [bind_address:]port:host:hostport
	var localAddr, remoteAddr string

	if len(parts) == 3 {
		localAddr = "localhost:" + parts[0]
		remoteAddr = parts[1] + ":" + parts[2]
	} else if len(parts) == 4 {
		localAddr = parts[0] + ":" + parts[1]
		remoteAddr = parts[2] + ":" + parts[3]
	} else {
		a.logf("Invalid local forward format: %s", forward)
		return
	}

	l, err := net.Listen("tcp", localAddr)
	if err != nil {
		a.logf("Failed to listen for local forward on %s: %v", localAddr, err)
		return
	}
	defer l.Close()

	for {
		conn, err := l.Accept()
		if err != nil {
			a.logf("Local forward accept error: %v", err)
			continue
		}
		go a.handleLocalForwardConn(client, conn, remoteAddr)
	}
}

func (a *app) handleLocalForwardConn(client *ssh.Client, localConn net.Conn, remoteAddr string) {
	defer localConn.Close()

	remoteConn, err := client.Dial("tcp", remoteAddr)
	if err != nil {
		a.logf("Local forward failed to connect to remote %s: %v", remoteAddr, err)
		return
	}
	defer remoteConn.Close()

	errc := make(chan error, 2)
	go func() {
		_, err := io.Copy(remoteConn, localConn)
		errc <- err
	}()
	go func() {
		_, err := io.Copy(localConn, remoteConn)
		errc <- err
	}()

	<-errc
}

func (a *app) startDynamicForward(client *ssh.Client, forward string) {
	addr := forward
	if !strings.Contains(addr, ":") {
		addr = "localhost:" + addr
	}

	l, err := net.Listen("tcp", addr)
	if err != nil {
		a.logf("Failed to listen for dynamic forward on %s: %v", addr, err)
		return
	}
	defer l.Close()

	for {
		conn, err := l.Accept()
		if err != nil {
			a.logf("Dynamic forward accept error: %v", err)
			continue
		}
		go a.handleDynamicForwardConn(client, conn)
	}
}

func (a *app) handleDynamicForwardConn(client *ssh.Client, conn net.Conn) {
	defer conn.Close()

	// SOCKS5 Handshake
	buf := make([]byte, 256)
	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return
	}
	if buf[0] != 0x05 { // SOCKS5
		return
	}
	nmethods := int(buf[1])
	if _, err := io.ReadFull(conn, buf[:nmethods]); err != nil {
		return
	}
	// Select NO AUTHENTICATION REQUIRED (0x00)
	if _, err := conn.Write([]byte{0x05, 0x00}); err != nil {
		return
	}

	// SOCKS5 Request
	if _, err := io.ReadFull(conn, buf[:4]); err != nil {
		return
	}
	if buf[0] != 0x05 || buf[1] != 0x01 { // VER=5, CMD=CONNECT
		return
	}

	var addr string
	switch buf[3] { // ATYP
	case 0x01: // IPv4
		if _, err := io.ReadFull(conn, buf[:4]); err != nil {
			return
		}
		addr = net.IP(buf[:4]).String()
	case 0x03: // Domain name
		if _, err := io.ReadFull(conn, buf[:1]); err != nil {
			return
		}
		n := int(buf[0])
		if _, err := io.ReadFull(conn, buf[:n]); err != nil {
			return
		}
		addr = string(buf[:n])
	case 0x04: // IPv6
		if _, err := io.ReadFull(conn, buf[:16]); err != nil {
			return
		}
		addr = net.IP(buf[:16]).String()
	default:
		return
	}

	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return
	}
	port := (int(buf[0]) << 8) | int(buf[1])
	dest := net.JoinHostPort(addr, fmt.Sprintf("%d", port))

	remoteConn, err := client.Dial("tcp", dest)
	if err != nil {
		// Connection failed.
		// In SOCKS5, REP=0x01 means general failure.
		conn.Write([]byte{0x05, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	defer remoteConn.Close()

	// Connection successful.
	// REP=0x00 (succeeded), ATYP=0x01 (IPv4), BND.ADDR=0.0.0.0, BND.PORT=0
	if _, err := conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
		return
	}

	errc := make(chan error, 2)
	go func() {
		_, err := io.Copy(remoteConn, conn)
		errc <- err
	}()
	go func() {
		_, err := io.Copy(conn, remoteConn)
		errc <- err
	}()

	<-errc
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

	if a.jsonOutput {
		enc := json.NewEncoder(env.Stdout)
		enc.SetIndent("", "  ")
		if e.PublicKeys == nil {
			e.PublicKeys = []string{}
		}
		return enc.Encode(e.PublicKeys)
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

	if a.jsonOutput {
		fmt.Fprintln(cli.GetEnv(ctx).Stdout, "{}")
		return nil
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

	if a.jsonOutput {
		fmt.Fprintln(cli.GetEnv(ctx).Stdout, "{}")
		return nil
	}

	a.logf("Public key removed successfully.")
	return nil
}
