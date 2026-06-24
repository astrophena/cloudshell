module go.astrophena.name/cloudshell

go 1.26.4

require (
	go.astrophena.name/base v0.22.2
	golang.org/x/crypto v0.52.0
	golang.org/x/oauth2 v0.36.0
	golang.org/x/term v0.44.0
)

require (
	cloud.google.com/go/compute/metadata v0.9.0 // indirect
	github.com/BurntSushi/toml v1.6.0 // indirect
	github.com/go4org/hashtriemap v0.0.0-20251130024219-545ba229f689 // indirect
	github.com/lmittmann/tint v1.1.3 // indirect
	golang.org/x/exp/typeparams v0.0.0-20260312153236-7ab1446f8b90 // indirect
	golang.org/x/mod v0.34.0 // indirect
	golang.org/x/sync v0.21.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
	golang.org/x/tools v0.43.0 // indirect
	honnef.co/go/tools v0.7.0 // indirect
)

tool (
	go.astrophena.name/base/devtools/addcopyright
	go.astrophena.name/base/devtools/pre-commit
)

tool honnef.co/go/tools/cmd/staticcheck
