module go.astrophena.name/cloudshell

go 1.27

require (
	go.astrophena.name/base v0.26.0
	golang.org/x/crypto v0.58.0
	golang.org/x/oauth2 v0.37.0
	golang.org/x/term v0.47.0
)

require (
	cloud.google.com/go/compute/metadata v0.10.0 // indirect
	github.com/BurntSushi/toml v1.6.0 // indirect
	github.com/go4org/hashtriemap v0.0.0-20260925222741-44e5305f85d9 // indirect
	github.com/lmittmann/tint v1.2.1 // indirect
	golang.org/x/exp/typeparams v0.0.0-20260820142414-ca536658362e // indirect
	golang.org/x/mod v0.41.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	golang.org/x/tools v0.51.0 // indirect
	honnef.co/go/tools v0.8.2 // indirect
)

tool (
	go.astrophena.name/base/devtools/addcopyright
	go.astrophena.name/base/devtools/pre-commit
)

tool honnef.co/go/tools/cmd/staticcheck
