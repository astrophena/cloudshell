// © 2019 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the MIT
// license that can be found in the LICENSE.md file.

// Package auth handles authentication with the Google APIs.
package auth // import "go.astrophena.me/cloudshell/internal/auth"

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"go.astrophena.me/cloudshell/internal/config"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	cloudshell "google.golang.org/api/cloudshell/v1alpha1"
	userinfo "google.golang.org/api/oauth2/v2"
)

// TODO: Use normal error handling instead of log.Fatal().

// Service returns the Cloud Shell API client.
func Service() (service *cloudshell.Service, err error) {
	c := client()

	service, err = cloudshell.New(c)
	if err != nil {
		return nil, err
	}

	return service, nil
}

// Email retrieves the email of authorized user, then returns it.
func Email() (email string, err error) {
	c := client()
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

// client retrieves a token, saves the token, then returns the generated client.
func client() *http.Client {
	b, err := ioutil.ReadFile(config.ClientSecretsFile())
	if err != nil {
		log.Fatalf("unable to read client secrets file: %v", err)
	}

	cfg, err := google.ConfigFromJSON(b,
		fmt.Sprintf("%s email", cloudshell.CloudPlatformScope))
	if err != nil {
		log.Fatalf("unable to parse client secret file to config: %v", err)
	}

	tok, err := tokenFromFile(config.CredsFile())
	if err != nil {
		tok = token(cfg)
		saveToken(config.CredsFile(), tok)
	}
	return cfg.Client(context.Background(), tok)
}

// token requests a token, then returns the retrieved token.
func token(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: %v\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("unable to read authorization code: %v", err)
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		log.Fatalf("unable to retrieve token: %v", err)
	}
	return tok
}

// tokenFromFile retrieves a token from a local file.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// saveToken saves a token to a file path.
func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}
