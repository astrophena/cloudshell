# Google Cloud Shell CLI

This is [Google Cloud Shell] CLI, written in [Go].

## Requirements

* [Git]
* [Go] 1.13

`cloudshell` is using [Go Modules] to manage dependencies.

## Building from source

```sh
$ git clone https://github.com/astrophena/cloudshell.git
$ cd cloudshell
# Build binary for your platform.
$ script/build
# Build binary for Windows.
$ GOOS=windows GOARCH=amd64 script/build
# Built binaries are placed in the `./bin` directory.
```

[Google Cloud Shell]: https://cloud.google.com/shell/
[Git]: https://git-scm.com
[Go]: https://golang.org
[Go Modules]: https://github.com/golang/go/wiki/Modules
