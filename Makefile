SHELL := /bin/sh
NPM := env -u NODE_OPTIONS npm
GO_GENERATOR := github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.9.0

.PHONY: install web build run test e2e generate check-contracts dev-web dev-go notices dist bench

install:
	$(NPM) --prefix web ci
	go mod download

web:
	$(NPM) --prefix web run build

build: web
	go build -trimpath -ldflags="-s -w" -o bin/sshw ./cmd/sshw

run: build
	./bin/sshw

test: check-contracts
	go test -race ./...
	go vet ./...
	$(NPM) --prefix web run typecheck
	$(NPM) --prefix web test

e2e:
	$(NPM) --prefix web run test:e2e

generate:
	$(NPM) --prefix web run generate:api
	go run $(GO_GENERATOR) -generate types,skip-prune -package contract -o internal/transport/httpapi/generated/api.gen.go api/openapi.json

check-contracts:
	$(NPM) --prefix web run check:api
	sh scripts/check-go-contract.sh

dev-web:
	$(NPM) --prefix web run dev

dev-go:
	go run ./cmd/sshw --foreground --dev-origin http://127.0.0.1:5173

notices:
	node scripts/notices.mjs

# Cross-compiled archives with SHA-256 checksums. Building for a platform is
# not the same as testing on it; see docs/06-validation.md.
dist: web notices
	sh scripts/dist.sh

# Resource and throughput checks against the loopback fixture (minutes).
bench:
	SSHWB_BENCH=1 go test -run 'TestResource' -count=1 -timeout 30m -v ./tests/resource/
