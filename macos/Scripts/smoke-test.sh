#!/bin/bash
set -euo pipefail
mac_dir="$(cd "$(dirname "$0")/.." && pwd)"
repo_dir="$(cd "$mac_dir/.." && pwd)"
test_dir="$mac_dir/.build/smoke"
mkdir -p "$test_dir" "$mac_dir/.build/engine"
"$mac_dir/Scripts/prepare-pandoc.sh"
cd "$repo_dir"
CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 MACOSX_DEPLOYMENT_TARGET=26.0 \
  go build -tags fbmac -buildmode=c-archive -o "$mac_dir/.build/engine/libfb.a" .
python3 "$mac_dir/Tests/make-fixtures.py" "$test_dir/fixtures"
xcrun swiftc -swift-version 6 -parse-as-library -target arm64-apple-macosx26.0 \
  -import-objc-header "$mac_dir/App/FB-Bridging-Header.h" -I "$mac_dir/.build/engine" \
  "$mac_dir/App/FBEngine.swift" "$mac_dir/App/FBSchemeHandler.swift" \
  "$mac_dir/App/BrowserModel.swift" \
  "$mac_dir/Tests/SmokeCheck.swift" \
  -L "$mac_dir/.build/engine" -lfb -lresolv -lpthread \
  -framework AppKit -framework WebKit -framework PDFKit -framework Security -framework CoreFoundation \
  -o "$test_dir/SmokeCheck"
pandoc_relative="$(/usr/bin/plutil -extract executable raw -o - "$mac_dir/Dependencies/pandoc.json")"
pandoc_path="$mac_dir/.build/dependencies/$pandoc_relative"
if [[ ! -x "$pandoc_path" ]]; then
  echo "Pinned Pandoc is missing: $pandoc_path" >&2
  exit 1
fi
"$test_dir/SmokeCheck" "$test_dir/fixtures" "$pandoc_path"
