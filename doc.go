// © 2025 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

/*
cloudshell gives access to the Google Cloud Shell from the terminal.

# Usage

	$ cloudshell <command>

Where <command> is one of the following:

  - info: Display the current status and details of the Cloud Shell environment,
    including the Docker image and SSH connection information.
  - ssh: Establish an SSH connection to the Cloud Shell environment. If the
    environment is not running, it will be started automatically.
  - start: Start the Cloud Shell environment and wait until it is running.
  - key <subcommand>: Manage public SSH keys for the environment.

Where key <subcommand> is one of the following:

  - key list: Show all public keys authorized for SSH access.
  - key add '<key>': Add a new public key. The key should be provided as a
    string, e.g., "$(cat ~/.ssh/id_rsa.pub)".
  - key remove '<key>': Remove a previously authorized public key.

# Setup

To use cloudshell, you need to configure Google Cloud API access:

 1. Create a project in the Google API Console.
 2. Enable the Cloud Shell API for your project.
 3. Create OAuth 2.0 credentials. Go to the "Credentials" page, click "Create
    Credentials," and select "OAuth client ID." Choose "Desktop app" as the
    application type.
 4. Download the credentials as a JSON file and save it as client_secret.json.
 5. Place this file in the application's state directory ($XDG_STATE_HOME/cloudshell/client_secret.json, typically ~/.local/state/cloudshell/client_secret.json)

# Authentication

The first time you run any command, cloudshell will initiate an OAuth
authentication flow. You will be prompted to open a URL in your browser, grant
the application access to your Google account, and paste an authorization code
back into the terminal.

Upon successful authentication, an access token is saved to token.json in the
state directory. This token will be used for all subsequent API requests.

# SSH Key Management

Before you can connect to your Cloud Shell environment using SSH, you must add
your public SSH key. This can be done with the 'key add' command.

Example:

	$ cloudshell key add "$(cat ~/.ssh/id_rsa.pub)"
*/
package main

import (
	_ "embed"

	"go.astrophena.name/base/cli"
)

//go:embed doc.go
var doc []byte

func init() { cli.SetDocComment(doc) }
