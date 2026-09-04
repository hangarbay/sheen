BINARY  := build/bin/sheen
PKG     := ./cmd/sheen
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo 0.1.0)
LDFLAGS := -s -w -X github.com/hangarbay/sheen/cmd/sheen.Version=$(VERSION)

.PHONY: all build test vet fmt lint install clean

all: build

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) $(PKG)

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

lint: vet fmt

install:
	go install -ldflags "$(LDFLAGS)" $(PKG)

clean:
	rm -rf build
