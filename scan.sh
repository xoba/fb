#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

# Pre-publish hygiene scan: secrets and personal information, in the
# working tree and across all git history. Run before pushing anywhere
# public; fails loudly on any finding.
#
# Usage:
#   ./scan.sh
#
# gitleaks is required (brew install gitleaks); trufflehog joins in when
# installed. A grep sweep then catches personal information that secret
# scanners don't consider leaks: email addresses (example.com/org/net
# fixtures excepted), home directory paths, and private key material.
# Findings judged acceptable live in .scanallow (one extended regex per
# line, comments allowed) — anything matching there is ignored, so the
# scan stays green without forgetting what it saw.
#
# Note: commit author names and emails publish with history too, and no
# content scan sees them. git log --format='%an <%ae>' | sort -u

fail=0

echo "== gitleaks (full history)"
if ! gitleaks git --no-banner --redact . >/dev/null 2>&1; then
    gitleaks git --no-banner --redact . || true
    fail=1
else
    echo "clean"
fi

if command -v trufflehog >/dev/null; then
    echo "== trufflehog (full history)"
    findings=$(trufflehog git "file://$PWD" --no-update --json 2>/dev/null | wc -l | tr -d ' ')
    if [[ "$findings" != 0 ]]; then
        echo "$findings findings — rerun trufflehog git file://$PWD for detail"
        fail=1
    else
        echo "clean"
    fi
fi

echo "== personal information sweep (tracked files and all history)"
allow() {
    if [[ -f .scanallow ]]; then
        grep -v -E -f <(grep -v '^#' .scanallow | grep -v '^$') || true
    else
        cat
    fi
}
sweep() {
    local label="$1" pattern="$2" except="${3:-^$}"
    local hits
    hits=$( {
        git grep -I -h -o -E "$pattern" -- ':(exclude)assets' 2>/dev/null || true
        git rev-list --all | while read -r c; do
            git grep -I -h -o -E "$pattern" "$c" -- ':(exclude)assets' 2>/dev/null || true
        done
    } | sort -u | { grep -v -E "$except" || true; } | allow)
    if [[ -n "$hits" ]]; then
        echo "$label:"
        sed 's/^/  /' <<<"$hits"
        fail=1
    fi
}

sweep "email addresses" '[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}' '@example\.(com|org|net)$'
sweep "home directory paths" '/(Users|home)/[A-Za-z0-9_.-]+'
sweep "private key material" 'BEGIN [A-Z ]*PRIVATE KEY'

if [[ $fail != 0 ]]; then
    echo "SCAN FAILED — resolve (or allowlist in .scanallow) before publishing"
    exit 1
fi
echo "scan clean"
