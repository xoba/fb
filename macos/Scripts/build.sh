#!/bin/bash
set -euo pipefail

mac_dir="$(cd "$(dirname "$0")/.." && pwd)"
repo_dir="$(cd "$mac_dir/.." && pwd)"
configuration="${CONFIGURATION:-Debug}"
build_revision="${FB_BUILD_REVISION:-}"
if [[ -z "$build_revision" ]]; then
  build_revision="$(git -C "$repo_dir" rev-parse --short HEAD 2>/dev/null || echo local)"
  if [[ -n "$(git -C "$repo_dir" status --porcelain --untracked-files=normal 2>/dev/null)" ]]; then
    build_revision="$build_revision-dirty"
  fi
fi

for tool in go python3 xcodegen xcodebuild; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "Missing build tool: $tool" >&2
    exit 1
  fi
done

python3 "$mac_dir/Scripts/prepare-go-notices.py" --check
"$mac_dir/Scripts/prepare-pandoc.sh"
mkdir -p "$mac_dir/.build/engine"
cd "$repo_dir"
CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 MACOSX_DEPLOYMENT_TARGET=26.0 \
  go build -tags fbmac -buildmode=c-archive -o "$mac_dir/.build/engine/libfb.a" .

# A development signature is useful locally. It does not replace Developer ID
# signing and notarization for downloads on other Macs.
identity="${FB_SIGN_IDENTITY:-}"
if [[ -z "$identity" ]]; then
  identity="$(/usr/bin/security find-identity -v -p codesigning 2>/dev/null | \
    /usr/bin/awk '/"Apple Development:/ { print $2; exit }')"
  identity="${identity:--}"
fi
echo "Building $configuration; signing identity: $identity" >&2
cd "$mac_dir"
xcodegen generate
xcodebuild -project FB.xcodeproj -scheme FB -configuration "$configuration" \
  -destination 'platform=macOS,arch=arm64' -derivedDataPath .build/xcode \
  CODE_SIGN_IDENTITY="$identity" FB_BUILD_REVISION="$build_revision" build "$@"

app_path="$mac_dir/.build/xcode/Build/Products/$configuration/fb.app"
/usr/bin/codesign --verify --deep --strict "$app_path"
if [[ ! -x "$app_path/Contents/Resources/bin/fb" ]]; then
  echo "The fb command is missing from the bundle." >&2
  exit 1
fi
echo "Built: $app_path"
