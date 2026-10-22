# Ahoy language server

BIN := ahoy-lsp
INSTALL := $(HOME)/.local/bin/ahoy-lsp

.PHONY: build install test clean

build:
	go build -o $(BIN) .

# Install is a symlink, not a copy: rebuilding $(BIN) immediately updates the
# command editors run. Restart the editor (or its language server) after a
# rebuild; a running process keeps the binary image it was started with.
install: build
	mkdir -p $(dir $(INSTALL))
	ln -sfn $(abspath $(BIN)) $(INSTALL)
	@echo "Installed $(INSTALL) -> $(abspath $(BIN))"

test: build
	go test ./...

clean:
	rm -f $(BIN)
