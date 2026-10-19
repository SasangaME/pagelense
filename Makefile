BINARY  := pagelense
BIN_DIR := bin
PKG     := ./cmd/pagelense
COVER   := coverage.out

# Load local settings from .env if it exists (see .env.example).
ifneq (,$(wildcard .env))
include .env
export $(shell sed -n 's/^\([A-Z_][A-Z0-9_]*\)=.*/\1/p' .env)
endif

.PHONY: run build test cover lint clean

run:
	go run $(PKG)

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BIN_DIR)/$(BINARY) $(PKG)

test:
	go test -race ./...

cover:
	go test -race -coverprofile=$(COVER) ./...
	go tool cover -func=$(COVER) | tail -n 1
	go tool cover -html=$(COVER) -o coverage.html

lint:
	go tool golangci-lint run ./...

clean:
	rm -rf $(BIN_DIR) $(COVER) coverage.html
