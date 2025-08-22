# `cloudshell`

`cloudshell` gives access to the [Google Cloud Shell] from the terminal.

See https://go.astrophena.name/cloudshell for documentation.

## Installation

### From binary

Download the precompiled binary from [releases page].

### From source

1. Install the latest version of [Go] if you haven't yet.

2. Install with `go install`:

        $ go install go.astrophena.name/cloudshell@latest

   `go install` puts binaries by default to `$GOPATH/bin` (e.g.
   `~/go/bin`).

   Use `GOBIN` environment variable to change this behavior.

## License

[ISC](LICENSE.md) © Ilya Mateyko

[Google Cloud Shell]: https://cloud.google.com/shell/
[releases page]: https://github.com/astrophena/cloudshell/releases
[Go]: https://golang.org/dl
