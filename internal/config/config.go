// © 2019 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the MIT
// license that can be found in the LICENSE.md file.

// Package config handles configuration management.
package config // import "go.astrophena.me/cloudshell/internal/config"

import (
	"errors"
	"os"
	"path/filepath"

	"go.astrophena.me/gen/pkg/fileutil"
)

// Dir returns path of the config directory, creating it if it doesn't exist.
func Dir() (string, error) {
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

// ClientSecretsFile returns path of the `client_secrets.json` file.
// It also checks if it does exist.
func ClientSecretsFile() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}

	path := filepath.Join(dir, "client_secrets.json")

	if !fileutil.Exists(path) {
		return "", errors.New("config: client_secrets.json is missing")
	}

	return path, nil
}

// CredsFile returns path to the JSON file
// with the authentication credentials.
func CredsFile() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "creds.json"), nil
}
