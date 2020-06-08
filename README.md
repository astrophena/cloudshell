# `cloudshell`

> **Work in Progress**: `cloudshell` is not finished and has many rough edges.

cloudshell is the [Google Cloud Shell](https://cloud.google.com/shell/) CLI, written in Go.

## Installation

1. [Install Go](https://golang.org/dl) 1.14 if you haven't yet.

2. Two installation options are supported:

    * Install with `go get`:

           $ pushd $(mktemp -d); go mod init tmp; go get github.com/astrophena/cloudshell; popd

      `go get` puts binaries by default to `$GOPATH/bin` (e.g.
      `~/go/bin`).

      Use `GOBIN` environment variable to change this behavior.

    * Install with `make`:

           $ git clone https://github.com/astrophena/cloudshell
           $ cd cloudshell
           $ make install

        `make install` installs `cloudshell`  by default to `$HOME/bin`.

        Use `PREFIX` environment variable to change this behavior:

           $ make install PREFIX="$HOME/.local" # Installs to $HOME/.local/bin.

## Setup

* Create a project in the Google API Console.
* Enable the Cloud Shell API.
* Create credentials, download and place them to:
  * `$XDG_CONFIG_HOME/cloudshell/client_secrets.json` (Linux)
  * `$HOME/Library/Application Support/cloudshell/client_secrets.json` (macOS)
* Run any command (e.g. `cloudshell info`) to authenticate.
* Add your SSH key by running `cloudshell key add`.
* Try to connect: `cloudshell connect`.

## License

[MIT](LICENSE.md) © Ilya Mateyko
