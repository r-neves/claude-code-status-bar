#!/usr/bin/env bash
# Build claude-code-status-bar and install it into ~/.claude.
#
# By default this only installs the binary and prints the settings.json snippet
# you need. Pass --configure to have the script patch settings.json for you; it
# backs the file up first and refuses to run without jq.
set -euo pipefail

BINARY="claude-code-status-bar"
PREFIX="${PREFIX:-$HOME/.claude}"
SETTINGS="$PREFIX/settings.json"

configure=0
skip_build=0
for arg in "$@"; do
  case "$arg" in
    --configure)  configure=1 ;;
    --skip-build) skip_build=1 ;;
    -h|--help)
      sed -n '2,7p' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *)
      echo "unknown flag: $arg" >&2
      exit 2
      ;;
  esac
done

if [[ "$skip_build" -eq 0 ]]; then
  command -v go >/dev/null || { echo "go toolchain not found" >&2; exit 1; }
  echo "building $BINARY..."
  version="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
  go build -trimpath -ldflags "-s -w -X main.version=$version" -o "$BINARY" .

  mkdir -p "$PREFIX"
  install -m 0755 "$BINARY" "$PREFIX/$BINARY"
  echo "installed $PREFIX/$BINARY"
fi

snippet="  \"statusLine\": { \"type\": \"command\", \"command\": \"$PREFIX/$BINARY\", \"padding\": 0 }"

if [[ "$configure" -eq 0 ]]; then
  cat <<EOF

Add this to $SETTINGS:

$snippet

Then restart Claude Code. Re-run with --configure to have this script do it.
EOF
  exit 0
fi

if ! command -v jq >/dev/null; then
  echo "jq is required for --configure; add the snippet manually:" >&2
  echo "$snippet" >&2
  exit 1
fi

if [[ ! -f "$SETTINGS" ]]; then
  echo '{}' > "$SETTINGS"
fi

backup="$SETTINGS.bak-$(date +%s)"
cp "$SETTINGS" "$backup"
echo "backed up settings to $backup"

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
jq --arg cmd "$PREFIX/$BINARY" \
   '.statusLine = {type: "command", command: $cmd, padding: 0}' \
   "$SETTINGS" > "$tmp"
mv "$tmp" "$SETTINGS"
trap - EXIT

echo "configured statusLine in $SETTINGS"
echo "restart Claude Code to pick it up"
