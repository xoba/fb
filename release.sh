#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

# Release fb: gate, tag, push both remotes, redeploy the dev service, and
# publish the new version to the Homebrew tap.
#
# Usage:
#   ./release.sh                     propose the next patch version, confirm, go
#   ./release.sh patch|minor|major   bump from the latest tag, no prompt
#   ./release.sh v1.2.3              release exactly this version
#   ./release.sh -n [...]            dry run: print the plan, change nothing
#
# If HEAD already carries a version tag, no new tag is created: the script
# republishes that version (idempotent re-run after a partial failure). A
# tagged HEAD plus a request for a new version is refused — nothing new to
# release.
#
# Gates before anything happens: clean tree on main, a clean tap checkout,
# gofmt, go vet, go test, and scan.sh (secrets and personal-info hygiene).

TAP="$HOME/homebrew-tap"
FORMULA="$TAP/Formula/fb.rb"

dry=0
if [[ "${1:-}" == "-n" || "${1:-}" == "--dry-run" ]]; then
    dry=1
    shift
fi
spec="${1:-}"

run() {
    if [[ $dry == 1 ]]; then echo "would: $*"; else "$@"; fi
}

# ---- gates
[[ "$(git branch --show-current)" == "main" ]] || { echo "not on main" >&2; exit 1; }
[[ -z "$(git status --porcelain)" ]] || { echo "working tree not clean" >&2; exit 1; }
[[ -f "$FORMULA" ]] || { echo "tap formula not found at $FORMULA" >&2; exit 1; }
[[ -z "$(git -C "$TAP" status --porcelain)" ]] || { echo "tap checkout not clean" >&2; exit 1; }

echo "== gates: gofmt, vet, test, scan"
unformatted=$(gofmt -l .)
[[ -z "$unformatted" ]] || { echo "gofmt needed: $unformatted" >&2; exit 1; }
go vet .
go test .
./scan.sh >/dev/null && echo "scan clean"

# ---- version
latest=$(git tag --list 'v*' --sort=-v:refname | head -1)
latest=${latest:-v0.0.0}
head_tag=$(git tag --points-at HEAD --list 'v*' | sort -V | tail -1)

bump() {
    local v=${1#v} ma mi pa
    IFS=. read -r ma mi pa <<<"$v"
    case "$2" in
    major) echo "v$((ma + 1)).0.0" ;;
    minor) echo "v$ma.$((mi + 1)).0" ;;
    patch) echo "v$ma.$mi.$((pa + 1))" ;;
    esac
}

if [[ -n "$head_tag" ]]; then
    version=$head_tag
    if [[ -n "$spec" && "$spec" != "$version" ]]; then
        echo "refusing: HEAD is already released as $version — commit something before releasing anew" >&2
        exit 1
    fi
    echo "HEAD is already tagged $version — republishing it (no new tag)"
elif [[ "$spec" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    version=$spec
elif [[ "$spec" =~ ^(major|minor|patch)$ ]]; then
    version=$(bump "$latest" "$spec")
elif [[ -z "$spec" ]]; then
    version=$(bump "$latest" patch)
    if [[ $dry == 0 ]]; then
        printf "latest is %s; release %s? [y/N] " "$latest" "$version"
        read -r ok
        [[ "$ok" == y* || "$ok" == Y* ]] || { echo "aborted" >&2; exit 1; }
    fi
else
    echo "unrecognized version spec: $spec (want vX.Y.Z, patch, minor, or major)" >&2
    exit 2
fi

if [[ -z "$head_tag" ]]; then
    newest=$(printf '%s\n%s\n' "$latest" "$version" | sort -V | tail -1)
    if [[ "$newest" != "$version" || "$version" == "$latest" ]]; then
        echo "$version is not newer than the latest tag $latest" >&2
        exit 1
    fi
fi

echo "== releasing $version"
[[ -n "$head_tag" ]] || run git tag "$version"
run git push origin main
run git push github main
run git push origin "$version"
run git push github "$version"

echo "== redeploying dev service"
run ./service.sh redeploy

echo "== publishing $version to the tap"
url="https://github.com/xoba/fb/archive/refs/tags/$version.tar.gz"
if [[ $dry == 1 ]]; then
    echo "would: fetch $url for its sha256, update $FORMULA, commit and push the tap"
else
    sha=""
    for _ in 1 2 3 4 5; do
        sha=$(curl -fsSL "$url" | shasum -a 256 | cut -d' ' -f1) && [[ -n "$sha" ]] && break
        echo "tarball not ready yet; retrying"
        sleep 2
    done
    [[ -n "$sha" ]] || { echo "cannot fetch $url for its sha256" >&2; exit 1; }
    sed -i '' \
        -e "s|archive/refs/tags/v[0-9.]*\.tar\.gz|archive/refs/tags/$version.tar.gz|" \
        -e "s|^  sha256 \".*\"|  sha256 \"$sha\"|" "$FORMULA"
    git -C "$TAP" add -A
    if git -C "$TAP" diff --cached --quiet; then
        echo "tap already at $version"
    else
        git -C "$TAP" commit -m "fb $version"
    fi
    git -C "$TAP" push
fi

echo "== verifying via brew (the real user path)"
run git -C "$(brew --repository xoba/tap)" pull
run brew upgrade fb
echo "released $version"
