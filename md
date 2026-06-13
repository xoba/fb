#!/usr/bin/env bash
set -euo pipefail

# Open a local file through the local_md browser.
#
# Usage:
#   md FILE
#
# The local_md server must already be running, typically with / as its served
# root:
#
#   go run ~/local_md/main.go /
#
# This script resolves FILE to an absolute path, URL-encodes that path, and
# asks macOS to open the matching local_md URL. For example, from ~/arca:
#
#   md todo.md
#
# opens:
#
#   http://localhost:3030/Users/mra/arca/todo.md
#
# The server renders Markdown files as HTML on the fly. Non-Markdown files are
# still opened through the same browser endpoint and served as-is.

usage() {
  echo "Usage: md FILE" >&2
}

if [[ $# -ne 1 ]]; then
  usage
  exit 2
fi

input="$1"

if [[ ! -f "$input" ]]; then
  echo "md: file not found: $input" >&2
  exit 1
fi

if ! command -v python3 >/dev/null 2>&1; then
  echo "md: python3 is required to URL-encode file paths" >&2
  exit 1
fi

opener="${MD_OPEN:-open}"
if ! command -v "$opener" >/dev/null 2>&1; then
  echo "md: opener not found: $opener" >&2
  exit 1
fi

# Resolve the target file without relying on GNU realpath/readlink behavior,
# which differs across platforms and may not be present on a stock macOS setup.
dir="$(cd -P "$(dirname "$input")" && pwd)"
base="$(basename "$input")"

if [[ "$dir" == "/" ]]; then
  full_path="/$base"
else
  full_path="$dir/$base"
fi

# Preserve path separators but escape spaces, punctuation, Unicode, and other
# bytes that are not safe in a URL path.
url_path="$(python3 - "$full_path" <<'PY'
import sys
import urllib.parse

print(urllib.parse.quote(sys.argv[1], safe="/"))
PY
)"

# Delegate to macOS. MD_OPEN is intentionally supported for tests, e.g.
# MD_OPEN=/bin/echo ./md README.md
exec "$opener" "http://localhost:3030${url_path}"
