// © 2019 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the MIT
// license that can be found in the LICENSE.md file.

// cloudshell is the Google Cloud Shell CLI.
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
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/astrophena/gen/pkg/fileutil"
	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"github.com/urfave/cli/v2"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	cloudshell "google.golang.org/api/cloudshell/v1alpha1"
	userinfo "google.golang.org/api/oauth2/v2"
)

const defaultVersion = "devel"

var Version = defaultVersion

func init() {
	if Version == defaultVersion {
		bi, ok := debug.ReadBuildInfo()
		if ok {
			Version = strings.TrimPrefix(bi.Main.Version, "v")
		}
	}
}

func main() {
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
						Name:      "delete",
						Aliases:   []string{"d"},
						Usage:     "Remove a public SSH key from the Cloud Shell",
						ArgsUsage: "[key id]",
						Action:    cmdKeyDelete,
					},
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		color.Red(err.Error())
	}
}

func cmdInfo(c *cli.Context) (err error) {
	s, err := authService()
	if err != nil {
		return err
	}

	n, err := environmentName()
	if err != nil {
		return err
	}

	e, err := s.Users.Environments.Get(n).Do()
	if err != nil {
		return err
	}

	switch e.State {
	case "RUNNING":
		color.Green("Environment is running.")
	case "DISABLED":
		color.Red("Environment is stopped.")
	case "DELETING":
		color.Red("Environment is deleting.")
	case "STARTING":
		color.Blue("Environment is starting.")
	default:
		color.Yellow("Environment in an unknown state.")
	}

	fmt.Printf("Docker Image: %s\n", e.DockerImage)

	if e.SshHost != "" && e.SshPort != 0 && e.SshUsername != "" {
		color.Blue("SSH connection details:")
		fmt.Printf("  Host:     %s\n", e.SshHost)
		fmt.Printf("  Port:     %d\n", e.SshPort)
		fmt.Printf("  Username: %s\n", e.SshUsername)
	} else {
		color.Red("SSH is unavaliable.")
	}

	return nil
}

func cmdConnect(c *cli.Context) (err error) {
	s, err := authService()
	if err != nil {
		return err
	}

	n, err := environmentName()
	if err != nil {
		return err
	}

	e, err := s.Users.Environments.Get(n).Do()
	if err != nil {
		return err
	}

	const msg = "Cloud Shell is starting.\nRun \"cloudshell connect\" again in a minute or two."

	fwd := c.String("fwd")

	switch e.State {
	case "RUNNING":
		if err := connectToEnvironment(e, s, fwd); err != nil {
			return err
		}
	case "STARTING":
		log.Println(msg)
	case "DISABLED":
		if err := startEnvironment(e, s); err != nil {
			return err
		}
		log.Println(msg)
	}

	return nil
}

func cmdKeyList(c *cli.Context) (err error) {
	s, err := authService()
	if err != nil {
		return err
	}

	n, err := environmentName()
	if err != nil {
		return err
	}

	e, err := s.Users.Environments.Get(n).Do()
	if err != nil {
		return err
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"ID", "Type"})
	table.SetBorder(false)

	for _, pk := range e.PublicKeys {
		id := strings.Split(pk.Name, "/")
		table.Append([]string{id[5], pk.Format})
	}

	table.Render()

	return nil
}

func cmdKeyAdd(c *cli.Context) (err error) {
	key := c.Args().Get(0)

	if key == "" {
		return errors.New("key add: key is required")
	}

	ks := strings.Split(key, " ")

	// See https://cloud.google.com/shell/docs/reference/rest/Shared.Types/Format
	// for supported key types.
	var kf string
	switch ks[0] {
	case "ssh-dss":
		kf = "SSH_DSS"
	case "ssh-rsa":
		kf = "SSH_RSA"
	case "ecdsa-sha2-nistp256":
		kf = "ECDSA_SHA2_NISTP256"
	case "ecdsa-sha2-nistp384":
		kf = "ECDSA_SHA2_NISTP384"
	case "ecdsa-sha2-nistp521":
		kf = "ECDSA_SHA2_NISTP521"
	default:
		return errors.New("key add: format is unsupported")
	}

	s, err := authService()
	if err != nil {
		return err
	}

	r := &cloudshell.CreatePublicKeyRequest{
		Key: &cloudshell.PublicKey{
			Format: kf,
			Key:    ks[1],
		},
	}

	n, err := environmentName()
	if err != nil {
		return err
	}

	_, err = s.Users.Environments.PublicKeys.Create(n, r).Do()
	if err != nil {
		return err
	}

	return nil
}

func cmdKeyDelete(c *cli.Context) (err error) {
	id := c.Args().Get(0)
	if id == "" {
		return errors.New("key delete: key id is required")
	}

	n, err := environmentName()
	if err != nil {
		return err
	}

	s, err := authService()
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

func authService() (service *cloudshell.Service, err error) {
	c, err := client()
	if err != nil {
		return nil, err
	}

	service, err = cloudshell.New(c)
	if err != nil {
		return nil, err
	}

	return service, nil
}

func email() (email string, err error) {
	c, err := client()
	if err != nil {
		return "", err
	}

	s, err := userinfo.New(c)
	if err != nil {
		return "", err
	}

	ti, err := s.Tokeninfo().Do()
	if err != nil {
		return "", err
	}

	if ti.Email == "" {
		return "", errors.New("auth: no email present in the token info")
	}
	email = ti.Email

	return email, nil
}

func client() (*http.Client, error) {
	path, err := clientSecretsFile()
	if err != nil {
		return nil, err
	}

	b, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("auth: unable to read client secrets file: %w", err)
	}

	scopes := cloudshell.CloudPlatformScope + " email"
	cfg, err := google.ConfigFromJSON(b, scopes)
	if err != nil {
		return nil, fmt.Errorf("auth: unable to parse client secrets file: %w", err)
	}

	credsFile, err := credsFile()
	if err != nil {
		return nil, err
	}

	tok, err := tokenFromFile(credsFile)
	if err != nil {
		tok, err = token(cfg)
		if err != nil {
			return nil, err
		}

		if err := saveToken(credsFile, tok); err != nil {
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

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve token: %w", err)
	}

	return tok, nil
}

func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	tok := &oauth2.Token{}

	return tok, json.NewDecoder(f).Decode(tok)
}

func saveToken(path string, token *oauth2.Token) error {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("unable to cache OAuth token: %w", err)
	}
	defer f.Close()

	json.NewEncoder(f).Encode(token)

	return nil
}

func configDir() (string, error) {
	ucd, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	dir := filepath.Join(ucd, "cloudshell")

	if !fileutil.Exists(dir) {
		if err := fileutil.Mkdir(dir); err != nil {
			return "", err
		}
	}

	return dir, nil
}

func clientSecretsFile() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}

	path := filepath.Join(dir, "client_secrets.json")

	if !fileutil.Exists(path) {
		return "", fmt.Errorf(
			"client_secrets.json is missing in %v.\nSee https://github.com/astrophena/cloudshell#setup for setup instructions.",
			dir,
		)
	}

	return path, nil
}

func credsFile() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "creds.json"), nil
}

func environmentName() (name string, err error) {
	email, err := email()
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("users/%s/environments/default", email), nil
}

func startEnvironment(e *cloudshell.Environment, s *cloudshell.Service) (err error) {
	r := &cloudshell.StartEnvironmentRequest{}

	if _, err := s.Users.Environments.Start(e.Name, r).Do(); err != nil {
		return err
	}

	return nil
}

func connectToEnvironment(e *cloudshell.Environment, s *cloudshell.Service, fwd string) (err error) {
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
