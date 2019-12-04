// Copyright (c) 2019 Ilya Mateyko
//
// The MIT License (MIT)
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

// Package auth handles authentication with APIs.
package auth // import "github.com/astrophena/cloudshell/internal/auth"

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"github.com/astrophena/cloudshell/internal/config"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	cloudshell "google.golang.org/api/cloudshell/v1alpha1"
	userinfo "google.golang.org/api/oauth2/v2"
)

// Service returns the instance of Cloud Shell API service.
func Service() *cloudshell.Service {
	c := client()
	s, err := cloudshell.New(c)
	if err != nil {
		log.Fatal(err)
	}

	return s
}

// Email retrieves the email of authorized user, then returns it.
func Email() string {
	var e string

	c := client()
	s, err := userinfo.New(c)
	if err != nil {
		log.Fatal(err)
	}

	ti, err := s.Tokeninfo().Do()
	if err != nil {
		log.Fatal(err)
	}

	if ti.Email != "" {
		e = ti.Email
	} else {
		log.Fatal(errors.New("no email present in the token info"))
	}

	return e
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

	tok, err := tokenFromFile(config.TokFile())
	if err != nil {
		tok = token(cfg)
		saveToken(config.TokFile(), tok)
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
