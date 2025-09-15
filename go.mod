module go.astrophena.name/cloudshell

go 1.25

require (
	go.astrophena.name/base v0.11.0
	golang.org/x/crypto v0.42.0
	golang.org/x/oauth2 v0.31.0
	golang.org/x/term v0.35.0
)

require (
	cloud.google.com/go/compute/metadata v0.8.0 // indirect
	github.com/BurntSushi/toml v1.4.1-0.20240526193622-a339e1f7089c // indirect
	golang.org/x/exp/typeparams v0.0.0-20231108232855-2478ac86f678 // indirect
	golang.org/x/mod v0.26.0 // indirect
	golang.org/x/sync v0.16.0 // indirect
	golang.org/x/sys v0.36.0 // indirect
	golang.org/x/tools v0.35.0 // indirect
	golang.org/x/tools/go/expect v0.1.1-deprecated // indirect
	honnef.co/go/tools v0.6.0 // indirect
)

tool (
	go.astrophena.name/base/devtools/addcopyright
	go.astrophena.name/base/devtools/pre-commit
)

tool honnef.co/go/tools/cmd/staticcheck
