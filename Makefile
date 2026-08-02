GO ?= go
DOCKER ?= docker
BINARY ?= keebacked
VERSION_FILE ?= VERSION
RELEASE_VERSION ?= $(strip $(shell cat $(VERSION_FILE) 2>/dev/null))
VERSION_TAG ?= v$(RELEASE_VERSION)
IMAGE_REPOSITORY ?= keebacked
IMAGE_TAG ?= $(VERSION_TAG)
IMAGE ?= $(IMAGE_REPOSITORY):$(IMAGE_TAG)
SOURCE_URL ?= https://git.seasonalnet.org/SeasonalNet/keebacked

COMMIT ?= $(shell git rev-parse --short=12 HEAD 2>/dev/null || printf 'unknown')
COMMIT_TIME ?= $(shell git show -s --format=%cI HEAD 2>/dev/null || printf 'unknown')
MODIFIED ?= $(shell if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then if test -n "$$(git status --porcelain --untracked-files=normal 2>/dev/null)"; then printf 'true'; else printf 'false'; fi; else printf 'unknown'; fi)
BUILD_DATE ?= $(shell if test -n "$$SOURCE_DATE_EPOCH"; then date -u -d "@$$SOURCE_DATE_EPOCH" '+%Y-%m-%dT%H:%M:%SZ'; else date -u '+%Y-%m-%dT%H:%M:%SZ'; fi)
BUILT_BY ?= $(shell id -un 2>/dev/null || printf 'unknown')
BUILD_HOST_OS ?= $(shell uname -s 2>/dev/null || printf 'unknown')
BUILD_HOST_KERNEL ?= $(shell uname -r 2>/dev/null || printf 'unknown')

BUILDINFO_PACKAGE := git.seasonalnet.org/SeasonalNet/keebacked/internal/buildinfo
LDFLAGS := -s -w \
	-X $(BUILDINFO_PACKAGE).version=$(VERSION_TAG) \
	-X $(BUILDINFO_PACKAGE).commit=$(COMMIT) \
	-X $(BUILDINFO_PACKAGE).commitTime=$(COMMIT_TIME) \
	-X $(BUILDINFO_PACKAGE).buildDate=$(BUILD_DATE) \
	-X $(BUILDINFO_PACKAGE).builtBy=$(BUILT_BY) \
	-X $(BUILDINFO_PACKAGE).buildHostOS=$(BUILD_HOST_OS) \
	-X $(BUILDINFO_PACKAGE).buildHostKernel=$(BUILD_HOST_KERNEL) \
	-X $(BUILDINFO_PACKAGE).modified=$(MODIFIED)

GOFILES := $(shell find . -type f -name '*.go' -not -path './.git/*')

.PHONY: all build clean fmt fmt-check image lint metadata race test tidy-check quality version version-check vet

all: quality

version-check:
	@test -f $(VERSION_FILE) || { echo "missing $(VERSION_FILE)" >&2; exit 1; }
	@test "$$(awk 'END { print NR }' $(VERSION_FILE))" -eq 1 || { echo "$(VERSION_FILE) must contain exactly one line" >&2; exit 1; }
	@grep -Eq '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$$' $(VERSION_FILE) || { echo "$(VERSION_FILE) must contain one SemVer value without a leading v" >&2; exit 1; }

version: version-check
	@printf '%s\n' '$(VERSION_TAG)'

metadata: version-check
	@printf '%-18s %s\n' \
		version '$(VERSION_TAG)' \
		commit '$(COMMIT)' \
		commit_time '$(COMMIT_TIME)' \
		modified '$(MODIFIED)' \
		built_at '$(BUILD_DATE)' \
		built_by '$(BUILT_BY)' \
		build_host_os '$(BUILD_HOST_OS)' \
		build_host_kernel '$(BUILD_HOST_KERNEL)' \
		image '$(IMAGE)'

build: version-check
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/keebacked

image: version-check
	$(DOCKER) build \
		--build-arg VERSION='$(VERSION_TAG)' \
		--build-arg COMMIT='$(COMMIT)' \
		--build-arg COMMIT_TIME='$(COMMIT_TIME)' \
		--build-arg MODIFIED='$(MODIFIED)' \
		--build-arg BUILD_DATE='$(BUILD_DATE)' \
		--build-arg BUILT_BY='$(BUILT_BY)' \
		--build-arg BUILD_HOST_OS='$(BUILD_HOST_OS)' \
		--build-arg BUILD_HOST_KERNEL='$(BUILD_HOST_KERNEL)' \
		--build-arg SOURCE_URL='$(SOURCE_URL)' \
		--tag '$(IMAGE)' \
		.

clean:
	rm -f $(BINARY) coverage.out
	rm -rf dist

fmt:
	golangci-lint fmt

fmt-check:
	@test -z "$$(gofmt -l $(GOFILES))" || { gofmt -l $(GOFILES); exit 1; }

lint:
	golangci-lint run ./...

test:
	$(GO) test ./...

race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

tidy-check:
	$(GO) mod tidy
	git diff --exit-code -- go.mod go.sum

quality: version-check fmt-check tidy-check lint test race vet build
