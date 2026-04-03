GO ?= go
BINARY ?= bin/møbius
CMD_DIR := ./cmd/mobius
PKGS := ./...
GOFILES := $(shell find cmd internal -name '*.go' -type f | sort)
GOCACHE ?= /tmp/mobius-gocache
GOMODCACHE ?= /tmp/mobius-gomodcache

export GOCACHE
export GOMODCACHE

.PHONY: build test fmt tidy run clean verify help diff-markdown comment

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

diff-markdown:
	./$(BINARY) diff --output-format markdown

comment:
	./$(BINARY) comment

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
		'diff-markdown  Run "bin/møbius diff --output-format markdown"' \
		'comment Run "bin/møbius comment"' \
		'verify  Format, test, and build' \
		'clean   Remove the built binary'
