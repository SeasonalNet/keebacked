# keebacked

`keebacked` watches an encrypted KeePass KDBX file and creates verified, atomic, versioned copies on one or more locally mounted backup targets.

It does **not** parse, decrypt, unlock, or modify KDBX data. The source file is treated as opaque bytes. The current implementation is Linux-only because it uses inotify directly.

Release versions are derived from Git `v*` tags. Untagged builds report the nearest tag, commit distance, and revision. Release history is recorded in [`CHANGELOG.md`](CHANGELOG.md).

## Design

- Watches the source directory directly with Linux inotify.
- Debounces save bursts and replacement-based writes.
- Reconciles periodically because filesystem events are only hints.
- Takes one stable temporary snapshot before touching any target.
- Verifies every copied file with SHA-256 before publication.
- Writes a timestamped history copy and an atomic `latest` copy.
- Retains the newest configured number of history files.
- Requires a `.keebacked-target` marker on every destination.
- Handles targets independently.
- Uses readable colored logs on interactive terminals and JSON elsewhere.
- Exposes no network API and performs no KDBX processing.

## Target layout

Each target contains:

```text
.keebacked-target
history/
  vault-20260802T041234.123456789Z-7d17f31a752d.kdbx
latest/
  vault.kdbx
  vault.kdbx.sha256
```

The marker may contain a short target name used in logs:

```bash
printf '%s\n' 'primary' > /path/to/target/.keebacked-target
chmod 600 /path/to/target/.keebacked-target
```

Create the marker **while the intended filesystem is mounted**. If the mount later disappears, the marker disappears with it and `keebacked` refuses to write into the underlying mountpoint.

## Configuration

`keebacked` reads TOML from `/etc/keebacked/keebacked.toml` by default. Use `-config` to select another file. Unknown keys are rejected so misspelled settings cannot silently fall back to defaults.

The tracked container configuration is [`config/keebacked.toml`](config/keebacked.toml):

```toml
source = "/source/vault.kdbx"
targets = ["/targets/primary", "/targets/offline"]
marker = ".keebacked-target"
debounce = "3s"
reconcile = "15m"
keep = 100
stability_interval = "250ms"
stability_attempts = 8

[logging]
format = "auto"
color = "auto"
```

| Key | Default | Meaning |
|---|---:|---|
| `source` | `/source/vault.kdbx` | Source file path |
| `targets` | `[/targets/primary, /targets/offline]` | Target root directories |
| `marker` | `.keebacked-target` | Required target marker filename |
| `debounce` | `3s` | Delay after filesystem activity |
| `reconcile` | `15m` | Periodic reconciliation interval |
| `keep` | `100` | History files retained per target; `0` keeps all |
| `stability_interval` | `250ms` | Delay used to confirm source stability |
| `stability_attempts` | `8` | Maximum snapshot attempts |
| `logging.format` | `auto` | `auto`, `text`, or `json` output |
| `logging.color` | `auto` | `auto`, `always`, or `never` ANSI styling |

`source` and every target must be absolute paths. Duration values use Go duration syntax such as `250ms`, `3s`, or `15m`.

### Logging

`logging.format = "auto"` selects human-readable text when the log output file descriptor is attached to a TTY and structured JSON otherwise. Normal systemd, Compose, redirected, and piped output therefore remains JSON. Explicit `text` and `json` values override detection.

`logging.color = "auto"` adds restrained ANSI styling to the level token only when text output is attached to a TTY, `TERM` is not `dumb`, and the `NO_COLOR` environment variable is not set to a non-empty value. `always` forces ANSI styling for text output, while `never` disables it. JSON output never contains ANSI escapes.

## Native execution

Build the binary and inspect its provenance:

```bash
make build
./keebacked -version
```

Run against an explicit host configuration:

```bash
cp config/keebacked.toml keebacked.local.toml
$EDITOR keebacked.local.toml
./keebacked -config ./keebacked.local.toml -once
```

## Docker Compose

The Compose file contains no machine-specific host paths. Copy [`.env.example`](.env.example), set the source directory and target roots for the current host, and ensure the configured source filename matches the file mounted at `/source`.

```bash
cp .env.example .env
$EDITOR .env

mkdir -p /path/to/primary-target /path/to/offline-target
printf '%s\n' primary > /path/to/primary-target/.keebacked-target
printf '%s\n' offline > /path/to/offline-target/.keebacked-target
chmod 700 /path/to/primary-target /path/to/offline-target
chmod 600 \
  /path/to/primary-target/.keebacked-target \
  /path/to/offline-target/.keebacked-target

make image
docker run --rm "keebacked:$(make image-tag)" -version
docker compose up -d --no-build
```

`docker exec` operates on an existing container, not an image. Use `docker run --rm <image> -version` to inspect an image before deployment.

`make image` resolves its version from Git tags and injects release and build provenance into the binary and OCI image labels. Override `IMAGE_REPOSITORY` when publishing elsewhere:

```bash
make image IMAGE_REPOSITORY=registry.example.org/example/keebacked
```

## CLI

```text
-config     path to the TOML configuration file
-once       perform one reconciliation and exit
-version    print release and build provenance
```

## Contributing

Development workflow, quality gates, versioning, and release-build instructions are documented in [`CONTRIBUTING.md`](CONTRIBUTING.md).

## License

This software is distributed under the AGPLv3 software license. See [`LICENSE`](LICENSE) for details.
