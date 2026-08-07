#!/usr/bin/env bash
set -euo pipefail

# Start the fb server.
#
# Usage:
#   ./run.sh [SERVE_PATH]
#
# Serves / by default so the whole filesystem is browsable at
# http://localhost:3030/; pass a path to serve a narrower root instead:
#
#   ./run.sh ~/notes

cd "$(dirname "$0")"
exec go run . "${1:-/}"
