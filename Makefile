BINARY := dist/ffbatch$(shell go env GOEXE)
VERSION ?= 0.2.1
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
PKG := github.com/Bilalmesk/ffmpeg-batch-convert/internal/version
LDFLAGS := -s -w -X $(PKG).Version=$(VERSION) -X $(PKG).Commit=$(COMMIT) -X $(PKG).Date=$(DATE)

.PHONY: all build test vet fmt run serve clean install release-check

all: build

build:
	@mkdir -p dist
	go build -trimpath -ldflags "$(LDFLAGS)" -o "$(BINARY)" .

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

run:
	go run . --help

serve:
	go run . serve --open

install:
	go install -trimpath -ldflags "$(LDFLAGS)" .

clean:
	rm -rf dist

release-check:
	goreleaser check
