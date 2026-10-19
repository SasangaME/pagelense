BIN_DIR   := bin
COVER     := coverage.out
COVER_MIN := 70

# Load .env if it exists.
ifneq (,$(wildcard .env))
include .env
export $(shell sed -n 's/^\([A-Z_][A-Z0-9_]*\)=.*/\1/p' .env)
endif

API_PORT       := 8081
API_ADMIN_PORT := 9091
WEB_PORT       := 8080
WEB_ADMIN_PORT := 9090

.PHONY: run-api run-web build test cover lint clean docker-build compose-up compose-down

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
	go tool cover -html=$(COVER) -o coverage.html
	@go tool cover -func=$(COVER) | awk '/^total:/ { sub("%", "", $$3); \
		printf "total coverage: %s%% (minimum $(COVER_MIN)%%)\n", $$3; \
		if ($$3 + 0 < $(COVER_MIN)) { print "coverage below $(COVER_MIN)%"; exit 1 } }'

lint:
	go tool golangci-lint run ./...

clean:
	rm -rf $(BIN_DIR) $(COVER) coverage.html

docker-build:
	docker build --target api -t pagelense-api .
	docker build --target web -t pagelense-web .

compose-up:
	docker compose up -d --build

compose-down:
	docker compose down
