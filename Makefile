BINARY  := build/bin/sheen
PKG     := ./cmd/sheen
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo 0.1.0)
LDFLAGS := -s -w -X main.Version=$(VERSION)

.PHONY: all build test vet fmt lint install clean publish

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

# push the current branch and cut the next minor release tag; the Release
# workflow then builds and publishes the binaries
publish:
	@test -z "$$(git status --porcelain --untracked-files=no)" || { echo "Error: working tree has uncommitted changes"; exit 1; }
	@git push origin HEAD
	@git fetch --tags --quiet origin
	@last=$$(git tag --list 'v*' --sort=-v:refname | head -1); \
	  [ -n "$$last" ] || last=v0.0.0; \
	  ver=$${last#v}; \
	  major=$${ver%%.*}; \
	  rest=$${ver#*.}; \
	  minor=$${rest%%.*}; \
	  next="v$$major.$$((minor + 1)).0"; \
	  echo "Tagging $$next (previous: $$last)"; \
	  git tag "$$next" && git push origin "$$next"; \
	  echo "Pushed $$next; the release workflow will build and publish the binaries"
