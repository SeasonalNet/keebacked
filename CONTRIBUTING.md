# Contributing to keebacked

## Prerequisites

The validated development toolchain is:

- Go 1.27.3
- golangci-lint 2.12.2
- GNU Make
- Docker with BuildKit for image builds
- Linux for inotify and race-test coverage

The repository includes [`mise.toml`](mise.toml) for Go and golangci-lint provisioning.

## Initial setup

```bash
mise trust
mise install
go mod download
make quality
```

## Development workflow

1. Read [`AGENTS.md`](AGENTS.md) before changing behavior or architecture.
2. Keep KDBX handling opaque; do not add parsing or decryption.
3. Add regression tests for behavior, configuration, logging, lifecycle, or security changes.
4. Run `make quality` before committing.
5. Review `git diff --check` and the final diff before publishing changes.

The canonical quality gate runs:

```text
SemVer validation
format verification
go mod tidy verification
golangci-lint
go test
go test -race
go vet
metadata-injected binary build
```

Individual targets remain available:

```bash
make fmt
make lint
make test
make race
make vet
make build
```

For host-native testing, copy the container configuration and replace its paths with host paths:

```bash
cp config/keebacked.toml keebacked.local.toml
$EDITOR keebacked.local.toml
go run ./cmd/keebacked -config ./keebacked.local.toml -once
```

Run continuously:

```bash
go run ./cmd/keebacked -config ./keebacked.local.toml
```

## Versioning

`keebacked` follows Semantic Versioning. Git `vMAJOR.MINOR.PATCH` tags are the release version authority. A clean tagged build reports the tag; untagged builds report the nearest release tag, commit distance, revision, and dirty state.

```text
v1.2.3
```

Binary version output and image tags retain the `v` prefix:

```text
v1.2.3
keebacked:v1.2.3
```

For a release, update applicable [`CHANGELOG.md`](CHANGELOG.md) entries and create an annotated tag on the intended commit. Do not manually bump a version file. Pre-1.0 releases may make incompatible changes, but those changes must still be documented and versioned deliberately.

Check the resolved version and build metadata with:

```bash
make version
make metadata
```

## Binary build provenance

`make build` injects:

- Semantic version
- Git commit and commit timestamp
- Dirty-worktree state
- UTC build timestamp
- Invoking build user
- Build-host operating system and kernel

Go supplies the compiler version and target operating system/architecture at runtime. When custom values are unavailable, `keebacked` also consults Go's embedded VCS build settings.

Inspect the result:

```bash
make build
./keebacked -version
```

Builder and timestamp metadata reduce bit-for-bit reproducibility and may disclose a local username. Release automation should set deliberate values when appropriate:

```bash
make build \
  BUILT_BY=forgejo-actions \
  BUILD_HOST_OS=Linux \
  BUILD_HOST_KERNEL=container
```

Set `SOURCE_DATE_EPOCH` to derive a deterministic UTC build timestamp:

```bash
SOURCE_DATE_EPOCH=1785654000 make build BUILT_BY=reproducible
```

## Docker images

Build the default versioned image:

```bash
make image
```

The default result is:

```text
keebacked:<version-from-git-tag>
```

Publish under another repository name without changing source files:

```bash
make image IMAGE_REPOSITORY=registry.example.org/example/keebacked
```

Override the complete image reference when necessary:

```bash
make image IMAGE=registry.example.org/example/keebacked:test
```

The Dockerfile records standard OCI labels for title, description, version, revision, creation time, and source URL. `make image` supplies the same provenance values to the compiled binary.

Inspect the built artifact and labels:

```bash
docker run --rm "keebacked:$(make image-tag)" -version
docker image inspect "keebacked:$(make image-tag)" --format '{{json .Config.Labels}}'
```

`docker compose build` remains available for local use, but its metadata values come from `.env` and default to `unknown`. Use `make image` for correctly derived repository metadata, followed by:

```bash
docker compose up -d --no-build
```

## Security scanning

Forgejo runs [`.forgejo/workflows/security.yml`](.forgejo/workflows/security.yml) on pushes, pull requests targeting `main`, and manual dispatches. The workflow installs the pinned Gitleaks release and scans the repository using [`.gitleaks.toml`](.gitleaks.toml).

Run the equivalent scan locally when Gitleaks 8.30.1 is installed:

```bash
gitleaks dir . --config .gitleaks.toml --redact --verbose --no-banner
```

Treat allowlist changes as security-sensitive. Keep exceptions narrow, documented, and limited to values that cannot contain a secret.

## Dependency policy

Prefer the Go standard library. New dependencies must have a concrete operational benefit, a maintained upstream, and tests covering their integration. Run `go mod tidy` and review both `go.mod` and `go.sum` whenever dependencies change.

## Release checklist

1. Move applicable `CHANGELOG.md` entries out of `Unreleased`.
2. Run `make quality`.
3. Run `make image` and inspect the binary version and OCI labels.
4. Commit the release state.
5. Create an annotated `vX.Y.Z` Git tag on the release commit.
6. Publish the versioned image and push the tag through the normal repository workflow.
