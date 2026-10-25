MODULE  := github.com/m10rten/go-canary-cli
VERSION ?= $(shell git describe --tags --always --dirty)
COMMIT  := $(shell git rev-parse --short HEAD)
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
	-X $(MODULE)/internal/buildinfo.Version=$(VERSION) \
	-X $(MODULE)/internal/buildinfo.Commit=$(COMMIT) \
	-X $(MODULE)/internal/buildinfo.Date=$(DATE)

TARGETS      := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64
DIST_VERSION  = $(VERSION:v%=%)

.PHONY: build test clean dist release-classify release-next-nightly release-prune-nightlies release-dist-tags

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/canary ./cmd/canary

test:
	go vet ./...
	go test ./...

clean:
	rm -rf bin dist

# Requires zip (Windows archives) and sha256sum or shasum. Binaries are staged
# in bin/ so dist/ holds only archives and checksums.txt; run `make clean` first
# to drop archives left over from another version.
dist:
	mkdir -p dist
	@set -e; archives=""; for target in $(TARGETS); do \
		os=$${target%/*}; arch=$${target#*/}; \
		bin=canary; [ "$$os" = windows ] && bin=canary.exe; \
		stage=bin/$${os}_$${arch}; \
		archive=canary_$(DIST_VERSION)_$${os}_$${arch}; \
		mkdir -p $$stage; \
		echo "build $$archive"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "$(LDFLAGS)" -o $$stage/$$bin ./cmd/canary; \
		if [ "$$os" = windows ]; then \
			archive=$$archive.zip; \
			(cd $$stage && zip -q -X ../../dist/$$archive $$bin); \
		else \
			archive=$$archive.tar.gz; \
			COPYFILE_DISABLE=1 tar -czf dist/$$archive -C $$stage $$bin; \
		fi; \
		archives="$$archives $$archive"; \
	done; \
	cd dist; \
	if command -v sha256sum >/dev/null 2>&1; then sum="sha256sum"; else sum="shasum -a 256"; fi; \
	$$sum $$archives > checksums.txt

# Release track rules for the workflow (see internal/release/track.go). Inputs
# come from the environment (TAG, NIGHTLY_DATE, KEEP) or stdin, never from shell
# interpolation of a tag name. Tags and npm view JSON are read from stdin. Run
# with `make -s` so only the result is printed. TAG, NIGHTLY_DATE and KEEP must
# come from the environment, never as `make VAR=...`, because make expands $(...)
# in command-line variables. Callers can rely only on a zero or non-zero exit
# status, because go run and make wrap the tool's exit codes.
release-classify:
	@go run ./cmd/releasetool classify "$$TAG"

release-next-nightly:
	@go run ./cmd/releasetool next-nightly "$$NIGHTLY_DATE"

release-prune-nightlies:
	@go run ./cmd/releasetool prune-nightlies "$${KEEP:-14}"

release-dist-tags:
	@go run ./cmd/releasetool dist-tags
