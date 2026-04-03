GO ?= go
BINARY ?= bin/møbius
CMD_DIR := ./cmd/mobius
PKGS := ./...
GOFILES := $(shell find cmd internal -name '*.go' -type f | sort)
GOCACHE ?= /tmp/mobius-gocache
GOMODCACHE ?= /tmp/mobius-gomodcache

export GOCACHE
export GOMODCACHE

.PHONY: build test fmt tidy run clean verify help

build: $(BINARY)

$(BINARY): $(GOFILES) go.mod go.sum
	mkdir -p $(dir $(BINARY))
	$(GO) build -o $(BINARY) $(CMD_DIR)

test:
	$(GO) test $(PKGS)

fmt:
	gofmt -w $(GOFILES)

tidy:
	$(GO) mod tidy

run:
	./$(BINARY) diff

verify: fmt test build

clean:
	rm -f $(BINARY)

help:
	@printf '%s\n' \
		'build   Build the møbius binary at bin/møbius' \
		'test    Run Go tests' \
		'fmt     Format Go sources with gofmt' \
		'tidy    Sync Go module dependencies' \
		'run     Run "bin/møbius diff"' \
		'verify  Format, test, and build' \
		'clean   Remove the built binary'
