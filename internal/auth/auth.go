// © 2019 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the MIT
// license that can be found in the LICENSE.md file.

// Package auth handles authentication with the Google APIs.
package auth // import "github.com/astrophena/cloudshell/internal/auth"

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/astrophena/cloudshell/internal/config"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	cloudshell "google.golang.org/api/cloudshell/v1alpha1"
	userinfo "google.golang.org/api/oauth2/v2"
)

// Service returns the Cloud Shell API service.
func Service() (service *cloudshell.Service, err error) {
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

// Email retrieves the email of authorized user, then returns it.
func Email() (email string, err error) {
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
	path, err := config.ClientSecretsFile()
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

	credsFile, err := config.CredsFile()
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
		return nil, fmt.Errorf("auth: unable to read authorization code: %w", err)
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		return nil, fmt.Errorf("auth: unable to retrieve oauth token: %w", err)
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
		return fmt.Errorf("auth: unable to cache oauth token: %w", err)
	}
	defer f.Close()

	json.NewEncoder(f).Encode(token)

	return nil
}
