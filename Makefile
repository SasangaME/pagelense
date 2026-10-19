BIN_DIR := bin
COVER   := coverage.out

# Load local settings from .env if it exists (see .env.example).
ifneq (,$(wildcard .env))
include .env
export $(shell sed -n 's/^\([A-Z_][A-Z0-9_]*\)=.*/\1/p' .env)
endif

# Both services read PORT and ADMIN_PORT, so each target sets its own on top of .env.
API_PORT       := 8081
API_ADMIN_PORT := 9091
WEB_PORT       := 8080
WEB_ADMIN_PORT := 9090

.PHONY: run-api run-web build test cover lint clean

run-api:
	PORT=$(API_PORT) ADMIN_PORT=$(API_ADMIN_PORT) go run ./cmd/api

run-web:
	PORT=$(WEB_PORT) ADMIN_PORT=$(WEB_ADMIN_PORT) go run ./cmd/web

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BIN_DIR)/pagelense-api ./cmd/api
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BIN_DIR)/pagelense-web ./cmd/web

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
