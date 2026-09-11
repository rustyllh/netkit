.PHONY: build test install-test vet fmt-check release clean

VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short=12 HEAD 2>/dev/null || echo none)
BUILD_TIME ?= $(shell git log -1 --format=%cI 2>/dev/null || echo unknown)
LDFLAGS = -s -w -X github.com/rustyllh/netkit/internal/buildinfo.version=$(VERSION) -X github.com/rustyllh/netkit/internal/buildinfo.commit=$(COMMIT) -X github.com/rustyllh/netkit/internal/buildinfo.buildTime=$(BUILD_TIME)
BUILD_FLAGS = -trimpath -buildvcs=false -ldflags "$(LDFLAGS)"

build:
	go build $(BUILD_FLAGS) -o bin/netkit ./cmd/netkit

test:
	go test -race ./...

install-test:
	sh tests/install_test.sh

vet:
	go vet ./...

fmt-check:
	test -z "$$(gofmt -l $$(find cmd internal -name '*.go' -type f))"

release:
	mkdir -p dist/package_amd64/completions dist/package_arm64/completions
	go run ./cmd/netkit-completion --output dist/completions
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(BUILD_FLAGS) -o dist/package_amd64/netkit ./cmd/netkit
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build $(BUILD_FLAGS) -o dist/package_arm64/netkit ./cmd/netkit
	cp dist/completions/netkit.bash dist/completions/_netkit dist/completions/netkit.fish dist/package_amd64/completions/
	cp dist/completions/netkit.bash dist/completions/_netkit dist/completions/netkit.fish dist/package_arm64/completions/
	tar -C dist/package_amd64 -czf dist/netkit_linux_amd64.tar.gz netkit completions
	tar -C dist/package_arm64 -czf dist/netkit_linux_arm64.tar.gz netkit completions
	cd dist && shasum -a 256 netkit_linux_amd64.tar.gz netkit_linux_arm64.tar.gz > SHA256SUMS

clean:
	rm -rf bin dist
