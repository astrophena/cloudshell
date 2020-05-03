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

// Package config handles configuration management.
package config // import "go.astrophena.me/cloudshell/internal/config"

import (
	"errors"
	"log"
	"os"
	"path/filepath"
)

// Dir returns path of the config directory, creating it if it doesn't exist.
func Dir() string {
	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		log.Fatal(err)
	}

	dir := filepath.Join(userConfigDir, "astrophena", "cloudshell")

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		log.Printf("Creating config directory at %s", dir)
		if err := os.MkdirAll(dir, 0700); err != nil {
			log.Fatal(err)
		}
	}

	return dir
}

// ClientSecretsFile returns path of the `client_secrets.json` file. It also
// checks if it does exist.
func ClientSecretsFile() string {
	path := filepath.Join(Dir(), "client_secrets.json")

	if _, err := os.Stat(path); os.IsNotExist(err) {
		log.Fatal(errors.New("client_secrets.json is missing"))
	}

	return filepath.Join(Dir(), "client_secrets.json")
}

// TokFile returns path of the `token.json` file.
func TokFile() string {
	return filepath.Join(Dir(), "token.json")
}
