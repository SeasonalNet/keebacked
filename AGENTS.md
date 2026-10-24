# AGENTS.md

## Mission

`keebacked` is a narrowly scoped daemon that treats a KeePass KDBX database as an opaque critical file and creates verified, atomic, versioned copies on independently mounted targets.

## Non-negotiable invariants

- Never decrypt, parse, unlock, inspect, or modify KDBX contents.
- Never log source contents, credentials, key material, or full file data.
- The configured source is authoritative. Targets are one-way backups only.
- Never write to a target unless its marker file is present.
- Never publish a destination file before its complete copy has been synced and hash-verified.
- Target failures are independent; one unavailable target must not block another healthy target.
- File-system notifications are hints. Periodic reconciliation remains mandatory.
- Do not add a TCP listener, REST API, WebSocket API, gRPC API, database, or message broker without an explicit architecture decision.
- Any future IPC must default to a local Unix-domain socket with filesystem access controls.
- Run without root privileges and without Linux capabilities.
- Runtime configuration is TOML. Unknown keys must fail validation rather than being ignored.
- `VERSION` is the authoritative SemVer source. Keep release documentation and default image references synchronized with it.

## Development rules

- Target Go 1.27.3, pinned in `go.mod`, `mise.toml`, Docker, and CI.
- Keep dependencies minimal and justified. Prefer the Go standard library.
- Preserve deterministic behavior and explicit error handling.
- Add tests for every change to copy publication, verification, retention, mount/marker validation, event handling, configuration, or logging policy.
- Run `make quality` before committing. Human development and release workflow is documented in `CONTRIBUTING.md`.
- Do not weaken lint rules globally to accommodate one finding. Use a narrow, documented suppression only when the finding is demonstrably inapplicable.
- Keep the repository focused. General-purpose backup features require an explicit scope decision.

## Repository map

- `cmd/keebacked`: process entry point and build metadata.
- `internal/buildinfo`: release version and build provenance reporting.
- `internal/config`: strict TOML configuration loading, normalization, and validation.
- `internal/backup`: stable snapshot creation, verified publication, and retention.
- `internal/daemon`: debounce, periodic reconciliation, and serialized execution.
- `internal/logging`: TTY-aware JSON/text selection and optional ANSI styling.
- `internal/watch`: minimal Linux inotify wrapper.

## Security review focus

Review changes for path traversal, symlink handling, target confusion, writes to unmounted host directories, partial publication, permission widening, sensitive logging, unbounded resource use, and races between source replacement and snapshot creation.
