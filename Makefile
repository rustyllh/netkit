.PHONY: build test vet fmt-check release clean

VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short=12 HEAD 2>/dev/null || echo none)
BUILD_TIME ?= $(shell git log -1 --format=%cI 2>/dev/null || echo unknown)
LDFLAGS = -s -w -X github.com/rustyllh/netkit/internal/buildinfo.version=$(VERSION) -X github.com/rustyllh/netkit/internal/buildinfo.commit=$(COMMIT) -X github.com/rustyllh/netkit/internal/buildinfo.buildTime=$(BUILD_TIME)
BUILD_FLAGS = -trimpath -buildvcs=false -ldflags "$(LDFLAGS)"

build:
	go build $(BUILD_FLAGS) -o bin/netkit ./cmd/netkit

test:
	go test -race ./...

vet:
	go vet ./...

fmt-check:
	test -z "$$(gofmt -l $$(find cmd internal -name '*.go' -type f))"

release:
	mkdir -p dist
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(BUILD_FLAGS) -o dist/netkit_linux_amd64 ./cmd/netkit
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build $(BUILD_FLAGS) -o dist/netkit_linux_arm64 ./cmd/netkit
	shasum -a 256 dist/netkit_linux_amd64 dist/netkit_linux_arm64 > dist/SHA256SUMS

clean:
	rm -rf bin dist
