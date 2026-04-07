GO ?= go
BINARY ?= bin/møbius
CMD_DIR := ./cmd/mobius
PKGS := ./...
GOFILES := $(shell find cmd internal -name '*.go' -type f | sort)
GOCACHE ?= /tmp/mobius-gocache
GOMODCACHE ?= /tmp/mobius-gomodcache
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short=12 HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X github.com/sohooo/moebius/internal/buildinfo.Version=$(VERSION) -X github.com/sohooo/moebius/internal/buildinfo.Commit=$(COMMIT) -X github.com/sohooo/moebius/internal/buildinfo.Date=$(BUILD_DATE)

export GOCACHE
export GOMODCACHE

.PHONY: build test fmt tidy run clean verify help diff-markdown comment version schema-sync schema-verify

build: $(BINARY)

$(BINARY): $(GOFILES) go.mod go.sum
	mkdir -p $(dir $(BINARY))
	$(GO) build -ldflags '$(LDFLAGS)' -o $(BINARY) $(CMD_DIR)

test:
	$(GO) test $(PKGS)

fmt:
	gofmt -w $(GOFILES)

tidy:
	$(GO) mod tidy

run:
	./$(BINARY) diff

diff-markdown:
	./$(BINARY) diff --output-format markdown

comment:
	./$(BINARY) comment

version:
	./$(BINARY) version

verify: fmt test build

schema-sync:
	$(GO) run ./cmd/schema-sync

schema-verify:
	$(GO) run ./cmd/schema-sync --verify

clean:
	rm -f $(BINARY)

help:
	@printf '%s\n' \
		'build   Build the møbius binary at bin/møbius' \
		'test    Run Go tests' \
		'fmt     Format Go sources with gofmt' \
		'tidy    Sync Go module dependencies' \
		'run     Run "bin/møbius diff"' \
		'diff-markdown  Run "bin/møbius diff --output-format markdown"' \
		'comment Run "bin/møbius comment"' \
		'version Run "bin/møbius version"' \
		'schema-sync Import schema sources and regenerate the embedded schema bundle' \
		'schema-verify Verify the embedded schema bundle is up to date' \
		'verify  Format, test, and build' \
		'clean   Remove the built binary'
