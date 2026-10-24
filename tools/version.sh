#!/bin/sh
# Resolve release identity exclusively from Git tags and commit provenance.
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
git_cmd() { git -C "$root" "$@"; }
number='(0|[1-9][0-9]*)'
identifier='(0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)'
pattern="^v$number\\.$number\\.$number(-$identifier(\\.$identifier)*)?(\\+[0-9A-Za-z-]+(\\.[0-9A-Za-z-]+)*)?$"
valid_tag() { printf '%s\n' "$1" | grep -Eq "$pattern"; }
fail() { printf 'version: %s\n' "$*" >&2; exit 1; }
mode=${1:---version}
has_git=false
if [ "$(git_cmd rev-parse --show-toplevel 2>/dev/null || true)" = "$root" ]; then has_git=true; fi
case "$mode" in
  --revision) if "$has_git"; then git_cmd rev-parse HEAD; else printf 'unknown\n'; fi; exit ;;
  --timestamp) if "$has_git"; then git_cmd show -s --format=%cI HEAD; else printf 'unknown\n'; fi; exit ;;
  --image-tag)
    tag=${2:-}; valid_tag "$tag" || fail "expected v-prefixed SemVer, got: $tag"
    image_tag=$(printf '%s' "$tag" | tr '+' '_')
    [ "${#image_tag}" -le 128 ] || fail 'tag exceeds OCI tag length limit'
    printf '%s\n' "$image_tag"; exit ;;
  --stable)
    tag=${2:-}; valid_tag "$tag" || fail "expected v-prefixed SemVer, got: $tag"
    case "${tag%%+*}" in *-*) printf 'false\n';; *) printf 'true\n';; esac; exit ;;
  --release)
    tag=${2:-}; valid_tag "$tag" || fail "expected v-prefixed SemVer, got: $tag"
    "$has_git" || fail 'release validation requires a Git checkout'
    [ "$(git_cmd rev-parse --is-shallow-repository)" = false ] || fail 'release validation requires full history and tags'
    [ "$(git_cmd rev-parse --verify "refs/tags/$tag^{commit}")" = "$(git_cmd rev-parse HEAD)" ] || fail 'release tag does not identify HEAD'
    git_cmd diff --quiet HEAD -- || fail 'release checkout has tracked modifications'
    printf '%s\n' "${tag#v}"; exit ;;
  --version) ;;
  *) fail "unknown option: $mode" ;;
esac
if ! "$has_git"; then printf '0.0.0+unknown\n'; exit; fi
revision=$(git_cmd rev-parse --short=12 HEAD)
set --
for candidate in $(git_cmd tag --merged HEAD --list 'v[0-9]*'); do
  if valid_tag "$candidate"; then set -- "$@" "--match=$candidate"; fi
done
nearest=
if [ "$#" -gt 0 ]; then nearest=$(git_cmd describe --tags --abbrev=0 "$@" HEAD); fi
dirty=
if ! git_cmd diff --quiet HEAD --; then dirty=.dirty; fi
if [ -n "$nearest" ] && [ "$(git_cmd rev-parse "refs/tags/$nearest^{commit}")" = "$(git_cmd rev-parse HEAD)" ] && [ -z "$dirty" ]; then
  printf '%s\n' "${nearest#v}"
else
  base=0.0.0
  if [ -n "$nearest" ]; then
    base=${nearest#v}; base=${base%%+*}; base=${base%%-*}
    distance=$(git_cmd rev-list --count "$nearest..HEAD")
  else distance=$(git_cmd rev-list --count HEAD); fi
  printf '%s-dev.%s+g%s%s\n' "$base" "$distance" "$revision" "$dirty"
fi
