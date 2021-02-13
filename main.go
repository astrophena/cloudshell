// Command cloudshell gives access to Google Cloud Shell from the terminal.
package main // import "go.astrophena.name/cloudshell"

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
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

	"github.com/olekukonko/tablewriter"
	"github.com/urfave/cli/v2"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	cloudshell "google.golang.org/api/cloudshell/v1alpha1"
	userinfo "google.golang.org/api/oauth2/v2"
	"google.golang.org/api/option"
)

// Version is the version of cloudshell.
var Version = "devel"

func main() {
	if Version == "devel" {
		bi, ok := debug.ReadBuildInfo()
		if ok {
			Version = strings.TrimPrefix(bi.Main.Version, "v")
		}
	}

	log.SetFlags(0)

	app := &cli.App{
		Name:                 "cloudshell",
		Usage:                "Manage Google Cloud Shell.",
		EnableBashCompletion: true,
		Version:              Version,
		HideHelpCommand:      true,
		Commands: []*cli.Command{
			{
				Name:    "connect",
				Aliases: []string{"c"},
				Usage:   "Establish an interactive SSH session with Cloud Shell",
				Flags: []cli.Flag{
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
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:    "format",
								Aliases: []string{"f"},
								Usage:   "Output format (text, table or json)",
								Value:   "table",
							},
						},
						Action: cmdKeyList,
					},
					{
						Name:      "add",
						Aliases:   []string{"a"},
						Usage:     "Add a public SSH key to the Cloud Shell",
						ArgsUsage: "[public key, e.g. $(cat ~/.ssh/id_rsa.pub)]",
						Action:    cmdKeyAdd,
					},
					{
						Name:      "delete",
						Aliases:   []string{"d"},
						Usage:     "Remove a public SSH key from the Cloud Shell",
						ArgsUsage: "[id]",
						Action:    cmdKeyDelete,
					},
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

//
// Commands
//

func cmdInfo(c *cli.Context) error {
	s, err := service()
	if err != nil {
		return err
	}

	n, err := name()
	if err != nil {
		return err
	}

	e, err := s.Users.Environments.Get(n).Do()
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

	n, err := name()
	if err != nil {
		return err
	}

	e, err := s.Users.Environments.Get(n).Do()
	if err != nil {
		return err
	}

	if e.State == "STARTING" || e.State == "DISABLED" {
		if err := start(e, s); err != nil {
			return err
		}
	}

	e, err = s.Users.Environments.Get(n).Do()
	if err != nil {
		return err
	}

	return ssh(e, s, c.String("fwd"))
}

func cmdKeyList(c *cli.Context) error {
	s, err := service()
	if err != nil {
		return err
	}

	n, err := name()
	if err != nil {
		return err
	}

	e, err := s.Users.Environments.Get(n).Do()
	if err != nil {
		return err
	}

	switch c.String("format") {
	case "table":
		table := tablewriter.NewWriter(os.Stdout)
		table.SetHeader([]string{"ID", "Type"})
		table.SetBorder(false)

		for _, pk := range e.PublicKeys {
			id := strings.Split(pk.Name, "/")
			table.Append([]string{id[5], pk.Format})
		}

		table.Render()

		return nil
	case "text":
		for _, pk := range e.PublicKeys {
			id := strings.Split(pk.Name, "/")
			fmt.Println(id[5])
		}
		return nil
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(e.PublicKeys); err != nil {
			return err
		}
		return nil
	}

	return errors.New("unsupported format")
}

func cmdKeyAdd(c *cli.Context) error {
	key := c.Args().Get(0)

	if key == "" {
		return errors.New("key is required")
	}

	fk := strings.Split(key, " ")

	if len(fk) < 2 {
		return errors.New("key is invalid")
	}

	kf := strings.ReplaceAll(fk[0], "-", "_")
	kf = strings.ToUpper(kf)

	r := &cloudshell.CreatePublicKeyRequest{
		Key: &cloudshell.PublicKey{
			Format: kf,
			Key:    fk[1],
		},
	}

	n, err := name()
	if err != nil {
		return err
	}

	s, err := service()
	if err != nil {
		return err
	}

	_, err = s.Users.Environments.PublicKeys.Create(n, r).Do()
	if err != nil {
		return err
	}

	return nil
}

func cmdKeyDelete(c *cli.Context) error {
	id := c.Args().Get(0)
	if id == "" {
		return errors.New("key id is required")
	}

	n, err := name()
	if err != nil {
		return err
	}

	s, err := service()
	if err != nil {
		return err
	}

	if _, err := s.Users.Environments.PublicKeys.Delete(
		fmt.Sprintf("%s/publicKeys/%s", n, id),
	).Do(); err != nil {
		return err
	}

	return nil
}

//
// Helper functions
//

func name() (string, error) {
	c, err := client()
	if err != nil {
		return "", err
	}

	s, err := userinfo.NewService(context.Background(), option.WithHTTPClient(c))
	if err != nil {
		return "", err
	}

	info, err := s.Tokeninfo().Do()
	if err != nil {
		return "", err
	}

	if info.Email == "" {
		return "", errors.New("no email present in token info")
	}

	return fmt.Sprintf("users/%s/environments/default", info.Email), nil
}

func start(e *cloudshell.Environment, s *cloudshell.Service) error {
	r := &cloudshell.StartEnvironmentRequest{}

	if _, err := s.Users.Environments.Start(e.Name, r).Do(); err != nil {
		return err
	}

	log.Println("Environment is starting…")

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

func ssh(e *cloudshell.Environment, s *cloudshell.Service, fwd string) error {
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

//
// Authentication
//

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

	b, err := ioutil.ReadFile(path)
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
