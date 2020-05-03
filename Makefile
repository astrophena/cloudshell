# © 2020 Ilya Mateyko. All rights reserved.
# Use of this source code is governed by the MIT
# license that can be found in the LICENSE.md file.

PREFIX  ?= $(HOME)
VERSION ?= $(shell date -u +%Y.%m.%d)

BIN     = cloudshell
BINDIR  = $(PREFIX)/bin

LDFLAGS = "-s -w -X main.Version=$(VERSION) -buildid="

.PHONY: build install clean help

build: ## Build
	@ go build -o $(BIN) -trimpath -ldflags=$(LDFLAGS)

install: build ## Install
	@ mkdir -m755 -p $(BINDIR) && \
		install -m755 $(BIN) $(BINDIR)

clean: ## Clean
	@ rm -rf $(BIN) $(DISTDIR)

help: ## Show help
	@ grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "\033[0;32m%-30s\033[0m %s\n", $$1, $$2}'
