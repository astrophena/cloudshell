<div align="center">
  <h1>cloudshell</h1>
</div>

`cloudshell` gives access to the [Google Cloud Shell] from the terminal.

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

[Google Cloud Shell]: https://cloud.google.com/shell/
[releases page]: https://github.com/astrophena/cloudshell/releases
[Go]: https://golang.org/dl
