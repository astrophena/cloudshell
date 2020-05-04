// © 2019 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the MIT
// license that can be found in the LICENSE.md file.

// Package config handles configuration management.
package config // import "go.astrophena.me/cloudshell/internal/config"

import (
	"log"
	"os"
	"path/filepath"

	"go.astrophena.me/gen/pkg/fileutil"
)

// Dir returns path of the config directory, creating it if it doesn't exist.
func Dir() string {
	ucd, err := os.UserConfigDir()
	if err != nil {
		log.Fatal(err)
	}

	dir := filepath.Join(ucd, "cloudshell")

	if !fileutil.Exists(dir) {
		if err := fileutil.Mkdir(dir); err != nil {
			log.Fatal(err)
		}
	}

	return dir
}

// ClientSecretsFile returns path of the `client_secrets.json` file. It also
// checks if it does exist.
func ClientSecretsFile() string {
	path := filepath.Join(Dir(), "client_secrets.json")

	if !fileutil.Exists(path) {
		log.Fatal("client_secrets.json is missing. ")
	}

	return filepath.Join(Dir(), "client_secrets.json")
}

// CredsFile returns path to the JSON file
// with the authentication credentials.
func CredsFile() string {
	return filepath.Join(Dir(), "creds.json")
}
