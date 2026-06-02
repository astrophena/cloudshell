// © 2025 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
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
	"slices"
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
	tokenSource oauth2.TokenSource
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
		return a.key(ctx, args...)
	default:
		return fmt.Errorf("%w: no such command %q", cli.ErrInvalidArgs, command)
	}
}

func (a *app) key(ctx context.Context, args ...string) error {
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
}

// environment is the subset of the Cloud Shell Environment resource this tool uses.
type environment struct {
	Name        string   `json:"name"`
	ID          string   `json:"id"`
	DockerImage string   `json:"dockerImage"`
	State       string   `json:"state"`
	WebHost     string   `json:"webHost"`
	SSHUsername string   `json:"sshUsername"`
	SSHHost     string   `json:"sshHost"`
	SSHPort     int      `json:"sshPort"`
	PublicKeys  []string `json:"publicKeys"`
}

type operation struct {
	Name     string                   `json:"name"`
	Metadata startEnvironmentMetadata `json:"metadata"`
	Done     bool                     `json:"done"`
	Error    *operationStatus         `json:"error"`
	Response startEnvironmentResponse `json:"response"`
}

type operationStatus struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (s operationStatus) Error() string {
	if s.Message == "" {
		return fmt.Sprintf("status code %d", s.Code)
	}
	return fmt.Sprintf("status code %d: %s", s.Code, s.Message)
}

type startEnvironmentMetadata struct {
	State string `json:"state"`
}

type startEnvironmentResponse struct {
	Environment environment `json:"environment"`
}

// initClient prepares local state, loads OAuth client credentials, and creates
// an authenticated HTTP client. It is idempotent so commands can call it freely.
func (a *app) initClient(ctx context.Context) error {
	if a.authed {
		return nil
	}

	env := cli.GetEnv(ctx)
	a.logf = log.New(env.Stderr, "", 0).Printf

	stateDir, err := stateDir(env)
	if err != nil {
		return err
	}
	a.stateDir = stateDir
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
	a.tokenSource = a.oauthConfig.TokenSource(ctx, tok)
	a.httpc = oauth2.NewClient(ctx, a.tokenSource)
	a.authed = true
	return nil
}

func stateDir(env *cli.Env) (string, error) {
	if runtime.GOOS == "windows" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(configDir, "cloudshell"), nil
	}

	xdgStateDir := env.Getenv("XDG_STATE_HOME")
	if xdgStateDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		xdgStateDir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(xdgStateDir, "cloudshell"), nil
}

func (a *app) getToken(ctx context.Context) (*oauth2.Token, error) {
	env := cli.GetEnv(ctx)
	tokenFile, err := a.tokenFile()
	if err != nil {
		return nil, err
	}

	if tok, ok := readToken(tokenFile); ok {
		return tok, nil
	}

	// The desktop OAuth flow redirects the browser back to a short-lived local
	// loopback server. The random state ties that browser request to this CLI run.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("could not start local server: %w", err)
	}
	defer l.Close()
	a.oauthConfig.RedirectURL = fmt.Sprintf("http://%s", l.Addr().String())

	state, err := randomOAuthState()
	if err != nil {
		return nil, err
	}
	codeCh := make(chan string, 1)
	srv := a.oauthCallbackServer(state, codeCh)
	defer shutdownServer(srv, a.logf)

	go func() {
		if err := srv.Serve(l); err != http.ErrServerClosed {
			a.logf("local server error: %v", err)
		}
	}()

	authURL := a.oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline)
	if !openBrowser(authURL) {
		fmt.Fprintf(env.Stderr, "Go to the following link in your browser: %v\n", authURL)
	}

	select {
	case authCode := <-codeCh:
		return a.exchangeAndSaveToken(ctx, authCode, tokenFile)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (a *app) tokenFile() (string, error) {
	if a.profile == "default" || a.profile == "" {
		return filepath.Join(a.stateDir, "token.json"), nil
	}
	if !validProfileName(a.profile) {
		return "", fmt.Errorf("%w: invalid profile name %q", cli.ErrInvalidArgs, a.profile)
	}
	return filepath.Join(a.stateDir, "profiles", a.profile+".json"), nil
}

func readToken(path string) (*oauth2.Token, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var tok oauth2.Token
	if err := json.Unmarshal(b, &tok); err != nil {
		return nil, false
	}
	return &tok, true
}

func (a *app) oauthCallbackServer(state string, codeCh chan<- string) *http.Server {
	return &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("state") != state {
				http.Error(w, "invalid state", http.StatusBadRequest)
				return
			}

			code := r.URL.Query().Get("code")
			if code == "" {
				http.Error(w, "code not found", http.StatusBadRequest)
				return
			}

			select {
			case codeCh <- code:
				fmt.Fprintln(w, "Authentication successful! You can close this window now.")
			default:
				http.Error(w, "authentication already completed", http.StatusConflict)
			}
		}),
	}
}

func shutdownServer(srv *http.Server, logf func(string, ...any)) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logf("local server shutdown error: %v", err)
	}
}

func openBrowser(url string) bool {
	var opener string
	switch runtime.GOOS {
	case "linux", "android":
		opener = "xdg-open"
	case "darwin":
		opener = "open"
	default:
		return false
	}

	if _, err := exec.LookPath(opener); err != nil {
		return false
	}
	return exec.Command(opener, url).Start() == nil
}

func (a *app) exchangeAndSaveToken(ctx context.Context, authCode, tokenFile string) (*oauth2.Token, error) {
	newtok, err := a.oauthConfig.Exchange(ctx, authCode)
	if err != nil {
		return nil, err
	}

	b, err := json.MarshalIndent(newtok, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(tokenFile), 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(tokenFile, b, 0o600); err != nil {
		return nil, err
	}
	return newtok, nil
}

const defaultEnvironmentName = "users/me/environments/default"

func (a *app) getEnvironment(ctx context.Context) (environment, error) {
	return makeEnvironmentRequest[environment](ctx, a.httpc, http.MethodGet, "", nil)
}

func makeEnvironmentRequest[Response any](ctx context.Context, httpc *http.Client, method, suffix string, body any) (Response, error) {
	return makeAPIRequest[Response](ctx, httpc, method, "v1/"+defaultEnvironmentName+suffix, body)
}

func makeAPIRequest[Response any](ctx context.Context, httpc *http.Client, method, path string, body any) (Response, error) {
	const baseURL = "https://cloudshell.googleapis.com/"
	return request.Make[Response](ctx, request.Params{
		Method:     method,
		URL:        baseURL + path,
		Body:       body,
		HTTPClient: httpc,
	})
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

func (a *app) start(ctx context.Context) error {
	_, err := a.startEnvironment(ctx)
	return err
}

func (a *app) startEnvironment(ctx context.Context) (environment, error) {
	if err := a.initClient(ctx); err != nil {
		return environment{}, err
	}
	if err := a.ensureSSHKey(); err != nil {
		return environment{}, fmt.Errorf("failed to ensure SSH key: %w", err)
	}

	req, err := a.startEnvironmentRequest()
	if err != nil {
		return environment{}, err
	}
	op, err := makeEnvironmentRequest[operation](ctx, a.httpc, http.MethodPost, ":start", req)
	if err != nil {
		return environment{}, err
	}

	op, err = a.waitOperation(ctx, op)
	if err != nil {
		return environment{}, err
	}

	env := op.Response.Environment
	if env.State == "" {
		// The discovery document says StartEnvironmentResponse contains the started
		// Environment. Keep a defensive fallback in case the backend omits it.
		return a.getEnvironment(ctx)
	}
	return env, nil
}

type startEnvironmentRequest struct {
	AccessToken string   `json:"accessToken,omitempty"`
	PublicKeys  []string `json:"publicKeys"`
}

func (a *app) startEnvironmentRequest() (startEnvironmentRequest, error) {
	pubKeyBytes, err := os.ReadFile(filepath.Join(a.stateDir, "key.pub"))
	if err != nil {
		return startEnvironmentRequest{}, fmt.Errorf("could not read managed public key: %w", err)
	}

	accessToken, err := a.currentAccessToken()
	if err != nil {
		return startEnvironmentRequest{}, err
	}
	return startEnvironmentRequest{
		// Passing the OAuth access token pre-authenticates gcloud inside Cloud Shell,
		// as documented by StartEnvironmentRequest.accessToken.
		AccessToken: accessToken,
		// Cloud Shell API returns Internal Server Error when the SSH public key has
		// a trailing newline, so normalize the generated authorized_keys line.
		PublicKeys: []string{strings.TrimSuffix(string(pubKeyBytes), "\n")},
	}, nil
}

func (a *app) currentAccessToken() (string, error) {
	if a.tokenSource == nil {
		return "", errors.New("OAuth token source is not initialized")
	}
	tok, err := a.tokenSource.Token()
	if err != nil {
		return "", err
	}
	if !tok.Valid() {
		return "", errors.New("OAuth token is invalid")
	}
	return tok.AccessToken, nil
}

func (a *app) waitOperation(ctx context.Context, op operation) (operation, error) {
	if op.Name == "" {
		return operation{}, errors.New("start operation did not include a name")
	}

	progress := startProgress{w: cli.GetEnv(ctx).Stderr}
	if !op.Done {
		progress.start()
	}

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		if op.Done {
			if op.Error != nil {
				progress.stop()
				return operation{}, fmt.Errorf("operation %s failed: %s", op.Name, op.Error)
			}
			progress.done()
			return op, nil
		}

		select {
		case <-ticker.C:
			progress.tick()
			var err error
			op, err = makeAPIRequest[operation](ctx, a.httpc, http.MethodGet, "v1/"+op.Name, nil)
			if err != nil {
				progress.stop()
				return operation{}, err
			}
		case <-ctx.Done():
			progress.stop()
			return operation{}, ctx.Err()
		}
	}
}

type startProgress struct {
	w      io.Writer
	active bool
}

func (p *startProgress) start() {
	p.active = true
	fmt.Fprint(p.w, "Environment is starting...")
}

func (p *startProgress) tick() {
	if p.active {
		fmt.Fprint(p.w, ".")
	}
}

func (p *startProgress) done() {
	if p.active {
		fmt.Fprintln(p.w)
		fmt.Fprintln(p.w, "Environment has started.")
	}
}

func (p *startProgress) stop() {
	if p.active {
		fmt.Fprintln(p.w)
	}
}

// ensureSSHKey keeps a dedicated key pair in the state directory. If the
// private key exists but key.pub is missing, it reconstructs the public key.
func (a *app) ensureSSHKey() error {
	a.privateKeyPath = filepath.Join(a.stateDir, "key")
	publicKeyPath := filepath.Join(a.stateDir, "key.pub")

	if _, err := os.Stat(a.privateKeyPath); err == nil {
		if _, err := os.Stat(publicKeyPath); err == nil {
			return nil
		}
		a.logf("Regenerating missing SSH public key for Cloud Shell...")
		return a.regenerateSSHPublicKey(publicKeyPath)
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
	if err := writeSSHPublicKey(&privateKey.PublicKey, publicKeyPath); err != nil {
		return err
	}

	a.logf("Key pair saved to %s and %s.", a.privateKeyPath, publicKeyPath)
	return nil
}

func (a *app) regenerateSSHPublicKey(publicKeyPath string) error {
	key, err := os.ReadFile(a.privateKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read private key: %w", err)
	}
	privateKey, err := ssh.ParseRawPrivateKey(key)
	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}
	rsaKey, ok := privateKey.(*rsa.PrivateKey)
	if !ok {
		return fmt.Errorf("unsupported private key type %T", privateKey)
	}
	return writeSSHPublicKey(&rsaKey.PublicKey, publicKeyPath)
}

func writeSSHPublicKey(publicKey *rsa.PublicKey, path string) error {
	pub, err := ssh.NewPublicKey(publicKey)
	if err != nil {
		return fmt.Errorf("failed to create public key: %w", err)
	}
	if err := os.WriteFile(path, ssh.MarshalAuthorizedKey(pub), 0o644); err != nil {
		return fmt.Errorf("failed to write public key: %w", err)
	}
	return nil
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

	if _, err := makeEnvironmentRequest[request.IgnoreResponse](ctx, a.httpc, http.MethodPost, ":addPublicKey", struct {
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

	if _, err := makeEnvironmentRequest[request.IgnoreResponse](ctx, a.httpc, http.MethodPost, ":removePublicKey", struct {
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

func (a *app) ssh(ctx context.Context, args ...string) error {
	env, err := a.startEnvironment(ctx)
	if err != nil {
		return err
	}
	return a.sshExec(ctx, env, args...)
}

func (a *app) sshProxy(ctx context.Context, args ...string) error {
	e, err := a.startEnvironment(ctx)
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
	return relayStdio(conn, env.Stdin, env.Stdout)
}

func (a *app) sshExec(ctx context.Context, e environment, args ...string) error {
	if e.SSHHost == "" || e.SSHPort == 0 || e.SSHUsername == "" {
		return errors.New("connection with SSH is unavailable")
	}

	// Prefer the system ssh(1) for exact OpenSSH behavior. The Go fallback keeps
	// the CLI usable in minimal environments where ssh(1) is not installed.
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
	client, err := a.sshClient(e)
	if err != nil {
		return err
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
		// ssh.Session.Run executes a remote shell command string, not an argv list.
		return session.Run(shellJoin(args))
	}
	return runInteractiveShell(session, env.Stdin)
}

func (a *app) sshClient(e environment) (*ssh.Client, error) {
	key, err := os.ReadFile(a.privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key: %w", err)
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	config := &ssh.ClientConfig{
		User: e.SSHUsername,
		Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)},
		// Equivalent to OpenSSH's StrictHostKeyChecking=no. The host/port come from
		// the authenticated Google Cloud Shell API rather than user input.
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	addr := net.JoinHostPort(e.SSHHost, fmt.Sprintf("%d", e.SSHPort))
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("failed to dial: %w", err)
	}
	return client, nil
}

func runInteractiveShell(session *ssh.Session, stdin io.Reader) error {
	stdinFile, ok := stdin.(*os.File)
	if !ok {
		return errors.New("standard input is not a terminal file, cannot start interactive ssh session")
	}
	fd := int(stdinFile.Fd())
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
	localAddr, remoteAddr, ok := parseLocalForward(forward)
	if !ok {
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

func parseLocalForward(forward string) (localAddr, remoteAddr string, ok bool) {
	parts := strings.Split(forward, ":") // [bind_address:]port:host:hostport
	switch len(parts) {
	case 3:
		return "localhost:" + parts[0], parts[1] + ":" + parts[2], true
	case 4:
		return parts[0] + ":" + parts[1], parts[2] + ":" + parts[3], true
	default:
		return "", "", false
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

	if err := relay(localConn, remoteConn); err != nil && !errors.Is(err, net.ErrClosed) {
		a.logf("Local forward copy error: %v", err)
	}
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

	dest, err := socks5Accept(conn)
	if err != nil {
		return
	}

	remoteConn, err := client.Dial("tcp", dest)
	if err != nil {
		socks5Reply(conn, socks5ReplyGeneralFailure)
		return
	}
	defer remoteConn.Close()

	if err := socks5Reply(conn, socks5ReplySucceeded); err != nil {
		return
	}
	if err := relay(conn, remoteConn); err != nil && !errors.Is(err, net.ErrClosed) {
		a.logf("Dynamic forward copy error: %v", err)
	}
}

func relayStdio(conn io.ReadWriteCloser, stdin io.Reader, stdout io.Writer) error {
	stdinErrc := make(chan error, 1)
	stdoutErrc := make(chan error, 1)
	go func() {
		_, err := io.Copy(conn, stdin)
		closeWrite(conn)
		stdinErrc <- err
	}()
	go func() {
		_, err := io.Copy(stdout, conn)
		stdoutErrc <- err
	}()

	// Stdin may stay open forever in ProxyCommand usage. If the SSH side closes
	// first, close conn to unblock the stdin copier and return promptly.
	select {
	case <-stdinErrc:
		return <-stdoutErrc
	case err := <-stdoutErrc:
		conn.Close()
		return err
	}
}

func relay(a, b io.ReadWriteCloser) error {
	errc := make(chan error, 2)
	go copyAndCloseWrite(errc, a, b)
	go copyAndCloseWrite(errc, b, a)

	var err error
	for range 2 {
		if copyErr := <-errc; copyErr != nil && err == nil {
			err = copyErr
		}
	}
	return err
}

func copyAndCloseWrite(errc chan<- error, dst, src io.ReadWriteCloser) {
	_, err := io.Copy(dst, src)
	// Preserve TCP half-close semantics: EOF from one direction should signal EOF
	// to the peer without immediately discarding data still flowing back.
	closeWrite(dst)
	errc <- err
}

func closeWrite(conn io.ReadWriteCloser) {
	if closer, ok := conn.(interface{ CloseWrite() error }); ok {
		closer.CloseWrite()
	}
}

const (
	socks5Version             = 0x05
	socks5NoAuth              = 0x00
	socks5NoAcceptableMethods = 0xff
	socks5Connect             = 0x01
	socks5AddrIPv4            = 0x01
	socks5AddrDomain          = 0x03
	socks5AddrIPv6            = 0x04
	socks5ReplySucceeded      = 0x00
	socks5ReplyGeneralFailure = 0x01
)

func socks5Accept(conn io.ReadWriter) (string, error) {
	if err := socks5Handshake(conn); err != nil {
		return "", err
	}
	return socks5ReadConnectRequest(conn)
}

func socks5Handshake(conn io.ReadWriter) error {
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		return err
	}
	if header[0] != socks5Version {
		return errors.New("unsupported SOCKS version")
	}

	methods := make([]byte, int(header[1]))
	if _, err := io.ReadFull(conn, methods); err != nil {
		return err
	}
	if slices.Contains(methods, socks5NoAuth) {
		_, err := conn.Write([]byte{socks5Version, socks5NoAuth})
		return err
	}

	_, err := conn.Write([]byte{socks5Version, socks5NoAcceptableMethods})
	if err != nil {
		return err
	}
	return errors.New("client did not offer SOCKS5 no-auth method")
}

func socks5ReadConnectRequest(conn io.Reader) (string, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return "", err
	}
	if header[0] != socks5Version {
		return "", errors.New("unsupported SOCKS version")
	}
	if header[1] != socks5Connect {
		return "", errors.New("unsupported SOCKS command")
	}
	if header[2] != 0x00 {
		return "", errors.New("invalid SOCKS reserved byte")
	}

	addr, err := socks5ReadAddr(conn, header[3])
	if err != nil {
		return "", err
	}

	portb := make([]byte, 2)
	if _, err := io.ReadFull(conn, portb); err != nil {
		return "", err
	}
	port := (int(portb[0]) << 8) | int(portb[1])
	return net.JoinHostPort(addr, fmt.Sprintf("%d", port)), nil
}

func socks5ReadAddr(conn io.Reader, atyp byte) (string, error) {
	switch atyp {
	case socks5AddrIPv4:
		addr := make([]byte, net.IPv4len)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return "", err
		}
		return net.IP(addr).String(), nil
	case socks5AddrDomain:
		length := make([]byte, 1)
		if _, err := io.ReadFull(conn, length); err != nil {
			return "", err
		}
		addr := make([]byte, int(length[0]))
		if _, err := io.ReadFull(conn, addr); err != nil {
			return "", err
		}
		return string(addr), nil
	case socks5AddrIPv6:
		addr := make([]byte, net.IPv6len)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return "", err
		}
		return net.IP(addr).String(), nil
	default:
		return "", errors.New("unsupported SOCKS address type")
	}
}

func socks5Reply(conn io.Writer, reply byte) error {
	// Bind address/port are irrelevant for this proxy, so report 0.0.0.0:0.
	_, err := conn.Write([]byte{socks5Version, reply, 0x00, socks5AddrIPv4, 0, 0, 0, 0, 0, 0})
	return err
}

func uppercaseFirst(s string) string {
	if len(s) == 0 {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func shellJoin(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = shellQuote(arg)
	}
	return strings.Join(quoted, " ")
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if strings.IndexFunc(s, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("@%_+=:,./-", r))
	}) == -1 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func validProfileName(profile string) bool {
	if profile == "" {
		return false
	}
	for _, r := range profile {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return profile != "." && profile != ".."
}

func randomOAuthState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate OAuth state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
