GO ?= go
BINARY ?= møbius
CMD_DIR := ./cmd/mobius
PKGS := ./...
GOFILES := $(shell find cmd internal -name '*.go' -type f | sort)

.PHONY: build test fmt tidy run clean verify help

build: $(BINARY)

$(BINARY): $(GOFILES) go.mod go.sum
	$(GO) build -o $(BINARY) $(CMD_DIR)

test:
	$(GO) test $(PKGS)

fmt:
	gofmt -w $(GOFILES)

tidy:
	$(GO) mod tidy

run:
	$(GO) run $(CMD_DIR) diff

verify: fmt test build

clean:
	rm -f $(BINARY)

help:
	@printf '%s\n' \
		'build   Build the møbius binary' \
		'test    Run Go tests' \
		'fmt     Format Go sources with gofmt' \
		'tidy    Sync Go module dependencies' \
		'run     Run "møbius diff" from source' \
		'verify  Format, test, and build' \
		'clean   Remove the built binary'
