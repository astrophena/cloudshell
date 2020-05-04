# `cloudshell`

> **Work in Progress**: `cloudshell` is not finished and has many, many
> rough edges. I don't know when `cloudshell` will be finished. Maybe never.

cloudshell is the [Google Cloud Shell] CLI, written in [Go].

## Installation

1. Install [Go] 1.14 if you haven't yet.

2. Two installation options are supported:

    * Install with `go get`:

           $ pushd $(mktemp -d); go mod init tmp; go get go.astrophena.me/cloudshell; popd

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

## License

[MIT] © Ilya Mateyko

[Google Cloud Shell]: https://cloud.google.com/shell/
[Go]: https://golang.org
[MIT]: LICENSE.md
