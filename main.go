// Command cloudshell gives access to Google Cloud Shell from the terminal.
package main // import "go.astrophena.name/cloudshell"

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/urfave/cli/v2"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	cloudshell "google.golang.org/api/cloudshell/v1"
	"google.golang.org/api/option"
)

const envName = "users/me/environments/default"

var version = "devel"

func main() {
	if version == "devel" {
		bi, ok := debug.ReadBuildInfo()
		if ok {
			version = strings.TrimPrefix(bi.Main.Version, "v")
		}
	}

	log.SetFlags(0)

	app := &cli.App{
		Name:                 "cloudshell",
		Usage:                "Connect to Google Cloud Shell from the terminal.",
		EnableBashCompletion: true,
		Version:              version,
		HideHelpCommand:      true,
		Commands: []*cli.Command{
			{
				Name:    "connect",
				Aliases: []string{"c"},
				Usage:   "Establish an interactive SSH session with Cloud Shell",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "key",
						Aliases: []string{"k"},
						Usage:   "Path to the SSH key that should be used for authentication.",
					},
					&cli.StringFlag{
						Name:    "fwd",
						Aliases: []string{"f"},
						Usage:   "Forward local port to remote port: [local]:[remote]",
					},
				},
				Action: cmdConnect,
			},
			{
				Name:    "info",
				Aliases: []string{"i"},
				Usage:   "Print information about the environment",
				Action:  cmdInfo,
			},
			{
				Name:            "key",
				Aliases:         []string{"k"},
				Usage:           "Manage public keys associated with the Cloud Shell",
				HideHelpCommand: true,
				Subcommands: []*cli.Command{
					{
						Name:    "list",
						Aliases: []string{"l"},
						Usage:   "List public keys associated with the Cloud Shell",
						Action:  cmdKeyList,
					},
					{
						Name:      "add",
						Aliases:   []string{"a"},
						Usage:     "Add a public SSH key to the Cloud Shell",
						ArgsUsage: "[public key, e.g. $(cat ~/.ssh/id_rsa.pub)]",
						Action:    cmdKeyAdd,
					},
					{
						Name:      "remove",
						Aliases:   []string{"r"},
						Usage:     "Remove a public SSH key from the Cloud Shell",
						ArgsUsage: "[key]",
						Action:    cmdKeyRemove,
					},
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func cmdInfo(c *cli.Context) error {
	s, err := service()
	if err != nil {
		return err
	}

	e, err := s.Users.Environments.Get(envName).Do()
	if err != nil {
		return err
	}

	state := strings.ToLower(e.State)
	state = strings.Title(state) + "."
	fmt.Println(state)

	fmt.Printf("Docker Image: %s\n", e.DockerImage)

	if e.SshHost != "" && e.SshPort != 0 && e.SshUsername != "" {
		fmt.Println("SSH connection details:")
		fmt.Printf("  Host:     %s\n", e.SshHost)
		fmt.Printf("  Port:     %d\n", e.SshPort)
		fmt.Printf("  Username: %s\n", e.SshUsername)
	} else {
		fmt.Println("SSH is unavaliable.")
	}

	return nil
}

func cmdConnect(c *cli.Context) error {
	s, err := service()
	if err != nil {
		return err
	}

	e, err := s.Users.Environments.Get(envName).Do()
	if err != nil {
		return err
	}

	if e.State == "STARTING" || e.State == "SUSPENDED" {
		if err := start(e, s); err != nil {
			return err
		}
	}

	e, err = s.Users.Environments.Get(envName).Do()
	if err != nil {
		return err
	}

	return ssh(e, s, c.String("key"), c.String("fwd"))
}

func cmdKeyList(c *cli.Context) error {
	s, err := service()
	if err != nil {
		return err
	}

	e, err := s.Users.Environments.Get(envName).Do()
	if err != nil {
		return err
	}

	for _, k := range e.PublicKeys {
		log.Println(k)
	}

	return nil
}

func cmdKeyAdd(c *cli.Context) error {
	key := c.Args().Get(0)
	if key == "" {
		return errors.New("key is required")
	}

	s, err := service()
	if err != nil {
		return err
	}

	_, err = s.Users.Environments.AddPublicKey(envName, &cloudshell.AddPublicKeyRequest{Key: key}).Do()
	if err != nil {
		return err
	}

	return nil
}

func cmdKeyRemove(c *cli.Context) error {
	key := c.Args().Get(0)
	if key == "" {
		return errors.New("key is required")
	}

	s, err := service()
	if err != nil {
		return err
	}

	if _, err := s.Users.Environments.RemovePublicKey(envName, &cloudshell.RemovePublicKeyRequest{Key: key}).Do(); err != nil {
		return err
	}

	return nil
}

func start(e *cloudshell.Environment, s *cloudshell.Service) error {
	if _, err := s.Users.Environments.Start(e.Name, &cloudshell.StartEnvironmentRequest{}).Do(); err != nil {
		return err
	}

	log.Println("Environment is starting...")

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

loop:
	for {
		e, err := s.Users.Environments.Get(e.Name).Do()
		if err != nil {
			return err
		}

		if e.State == "RUNNING" {
			break loop
		}

		select {
		case <-ticker.C:
			continue
		case <-interrupt:
			log.Println("exiting")
			os.Exit(0)
		}
	}

	return nil
}

func ssh(e *cloudshell.Environment, s *cloudshell.Service, key, fwd string) error {
	if e.SshHost == "" || e.SshPort == 0 || e.SshUsername == "" {
		return errors.New("ssh is unavaliable")
	}

	host := e.SshUsername + "@" + e.SshHost
	port := strconv.FormatInt(e.SshPort, 10)

	path, err := exec.LookPath("ssh")
	if err != nil {
		return err
	}

	cmd := exec.Command(path, host, "-p", port, "-o", "StrictHostKeyChecking=no")

	if key != "" {
		cmd.Args = append(cmd.Args, "-i", key)
	}
	if fwd != "" {
		cmd.Args = append(cmd.Args, "-L", fwd)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}

func service() (*cloudshell.Service, error) {
	c, err := client()
	if err != nil {
		return nil, err
	}

	return cloudshell.NewService(context.Background(), option.WithHTTPClient(c))
}

func client() (*http.Client, error) {
	dir, err := configDir()
	if err != nil {
		return nil, err
	}

	path, err := clientSecretsFile(dir)
	if err != nil {
		return nil, err
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("unable to read client secrets file: %w", err)
	}

	cfg, err := google.ConfigFromJSON(b, cloudshell.CloudPlatformScope+" email")
	if err != nil {
		return nil, fmt.Errorf("unable to parse client secrets file: %w", err)
	}

	cf := credsFile(dir)

	tok, err := tokenFromFile(cf)
	if err != nil {
		tok, err = token(cfg)
		if err != nil {
			return nil, err
		}

		if err := saveToken(cf, tok); err != nil {
			return nil, err
		}
	}

	return cfg.Client(context.Background(), tok), nil
}

func token(config *oauth2.Config) (*oauth2.Token, error) {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: %v\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		return nil, fmt.Errorf("unable to read authorization code: %w", err)
	}

	tok, err := config.Exchange(context.Background(), authCode)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve token: %w", err)
	}

	return tok, nil
}

func tokenFromFile(path string) (*oauth2.Token, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var tok oauth2.Token

	return &tok, json.NewDecoder(f).Decode(&tok)
}

func saveToken(path string, token *oauth2.Token) error {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(token)
}

func configDir() (string, error) {
	ucd, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	dir := filepath.Join(ucd, "cloudshell")

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return "", err
		}
	}

	return dir, nil
}

func clientSecretsFile(dir string) (string, error) {
	path := filepath.Join(dir, "client_secrets.json")

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", fmt.Errorf(
			"client_secrets.json is missing in %v.\nSee https://github.com/astrophena/cloudshell#setup for setup instructions.",
			dir,
		)
	}

	return path, nil
}

func credsFile(dir string) string { return filepath.Join(dir, "creds.json") }
