GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
LDFLAGS = "-s -w -X main.Version=$(VERSION) -buildid="

VERSION ?= $(shell git describe --abbrev=0 --tags | cut -c 2-)-next

DISTDIR = ./dist

.PHONY: build clean dist help

build: ## Build
	@ GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=0 go build -trimpath -ldflags=$(LDFLAGS)

clean: ## Clean
	@ go clean

dist: ## Build with GoReleaser
	@ goreleaser --snapshot --skip-publish

help: ## Show help
	@ grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "\033[0;32m%-30s\033[0m %s\n", $$1, $$2}'
