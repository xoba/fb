#!/usr/bin/env bash
set -euo pipefail

# Start the local_md server.
#
# Usage:
#   ./run.sh [SERVE_PATH]
#
# Serves / by default so the whole filesystem is browsable at
# http://localhost:3030/; pass a path to serve a narrower root instead:
#
#   ./run.sh ~/notes

cd "$(dirname "$0")"
# sqlite_dbstat enables per-table size reporting; sqlite_fts5 lets queries
# against databases containing FTS5 tables work. The first build after a tag
# change recompiles the SQLite amalgamation (about a minute); cached after.
exec go run -tags "sqlite_dbstat sqlite_fts5" . "${1:-/}"
