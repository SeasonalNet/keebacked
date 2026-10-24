#!/bin/sh
# Test tag resolution in an isolated repository without changing project refs.
set -eu
source_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
fixture=$(mktemp -d)
trap 'rm -rf "$fixture"' EXIT HUP INT TERM
mkdir -p "$fixture/repo/tools"
cp "$source_root/tools/version.sh" "$fixture/repo/tools/version.sh"
git -C "$fixture/repo" init -q
git -C "$fixture/repo" config user.name 'Version test'
git -C "$fixture/repo" config user.email 'version-test@example.invalid'
git -C "$fixture/repo" config commit.gpgSign false
git -C "$fixture/repo" config tag.gpgSign false
git -C "$fixture/repo" add tools/version.sh
git -C "$fixture/repo" commit -qm initial
revision=$(git -C "$fixture/repo" rev-parse --short=12 HEAD)
expected="0.0.0-dev.1+g$revision"
actual=$(sh "$fixture/repo/tools/version.sh")
[ "$actual" = "$expected" ] || { printf 'expected %s, got %s\n' "$expected" "$actual" >&2; exit 1; }
git -C "$fixture/repo" tag v1.2.3
actual=$(sh "$fixture/repo/tools/version.sh")
[ "$actual" = '1.2.3' ] || { printf 'tagged version: expected 1.2.3, got %s\n' "$actual" >&2; exit 1; }
actual=$(sh "$fixture/repo/tools/version.sh" --release v1.2.3)
[ "$actual" = '1.2.3' ] || { printf 'release version: expected 1.2.3, got %s\n' "$actual" >&2; exit 1; }
git -C "$fixture/repo" tag v1.2.3-rc.1
actual=$(sh "$fixture/repo/tools/version.sh" --stable v1.2.3-rc.1)
[ "$actual" = false ] || { printf 'prerelease was marked stable\n' >&2; exit 1; }
printf 'version tests: development, clean release tag, and prerelease classification passed\n'
