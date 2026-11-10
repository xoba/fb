#!/bin/bash
# Prepare a reviewable cask from a real release artifact. Never publish it.
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: macos/Scripts/generate-cask.sh NOTARIZED.zip GITHUB_RELEASE_URL

Verify a local Developer ID-signed, stapled, notarized arm64 fb ZIP and generate
Casks/fb.rb under macos/.build/distribution/casks/<version>/. The version comes
from the app, and the checksum is calculated from the exact supplied archive.

GITHUB_RELEASE_URL must be an explicit versioned download URL under
https://github.com/xoba/fb/releases/download/ with the same archive filename.
It can be the intended URL before publication; this script does not claim the
remote asset exists or upload anything. Review/publish the release separately.

FB_DEVELOPMENT_TEAM defaults to 3288VRMQE7 for signature verification.
No build, certificate changes, Git tags, release uploads, or tap edits occur.
USAGE
}
if [[ "${1:-}" == --help || "${1:-}" == -h ]]; then
  usage
  exit 0
fi
if [[ "$#" != 2 ]]; then
  usage >&2
  exit 1
fi

mac_dir="$(cd "$(dirname "$0")/.." && pwd)"
artifact="$1"
release_url="$2"
team_id="${FB_DEVELOPMENT_TEAM:-3288VRMQE7}"
if [[ ! -f "$artifact" || "$artifact" != *.zip ]]; then
  echo "A real local ZIP archive is required: $artifact" >&2
  exit 1
fi
if [[ ! "$team_id" =~ ^[A-Z0-9]{10}$ ]]; then
  echo "Invalid developer team ID: $team_id" >&2
  exit 1
fi
url_pattern='^https://github\.com/xoba/fb/releases/download/([A-Za-z0-9][A-Za-z0-9._-]*)/([A-Za-z0-9][A-Za-z0-9._-]*\.zip)$'
if [[ ! "$release_url" =~ $url_pattern ]]; then
  echo 'Provide the exact versioned https://github.com/xoba/fb/releases/download/TAG/ASSET.zip URL.' >&2
  exit 1
fi
if [[ "${BASH_REMATCH[2]}" != "$(basename "$artifact")" ]]; then
  echo 'Release URL asset filename must match the supplied local archive.' >&2
  exit 1
fi

distribution_dir="$mac_dir/.build/distribution"
mkdir -p "$distribution_dir"
inspection_dir="$(mktemp -d "$distribution_dir/.cask-inspection.XXXXXX")"
trap 'rm -rf "$inspection_dir"' EXIT

# Only inspect the expected app payload. Reject traversal entries before ditto
# extracts; resource-fork metadata produced by package.sh is also permitted.
while IFS= read -r entry; do
  case "$entry" in
    /*|../*|*/../*|*/..|*\\*) echo "Unsafe ZIP entry: $entry" >&2; exit 1 ;;
    fb.app/|fb.app/*|__MACOSX/|__MACOSX/*) ;;
    *) echo "Unexpected ZIP payload (expected fb.app): $entry" >&2; exit 1 ;;
  esac
done < <(/usr/bin/unzip -Z1 "$artifact")
/usr/bin/ditto -x -k "$artifact" "$inspection_dir"
app_path="$inspection_dir/fb.app"
plist="$app_path/Contents/Info.plist"
if [[ ! -f "$plist" || -L "$app_path" ]]; then
  echo 'Archive does not contain the expected fb.app bundle.' >&2
  exit 1
fi
bundle_id="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$plist")"
version="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$plist")"
minimum_macos="$(/usr/libexec/PlistBuddy -c 'Print :LSMinimumSystemVersion' "$plist")"
if [[ "$bundle_id" != com.xoba.fb.app || ! "$version" =~ ^[0-9]+\.[0-9]+(\.[0-9]+)?$ ]]; then
  echo 'Archive must contain com.xoba.fb.app with a numeric release version.' >&2
  exit 1
fi
if [[ "$minimum_macos" != 26 && "$minimum_macos" != 26.0 ]]; then
  echo "App requires macOS $minimum_macos; update this generator's platform declaration before releasing." >&2
  exit 1
fi
expected_filename="fb-$version-arm64-notarized.zip"
if [[ "$(basename "$artifact")" != "$expected_filename" ]]; then
  echo "Expected the release filename $expected_filename for bundle version $version." >&2
  exit 1
fi

/usr/bin/codesign --verify --deep --strict "$app_path"
requirement="anchor apple generic and certificate leaf[subject.OU] = \"$team_id\" and certificate 1[field.1.2.840.113635.100.6.2.6] exists and certificate leaf[field.1.2.840.113635.100.6.1.13] exists"
for code_path in "$app_path" "$app_path/Contents/Helpers/pandoc"; do
  /usr/bin/codesign --verify --strict -R "=$requirement" "$code_path"
  details="$(/usr/bin/codesign -d --verbose=4 "$code_path" 2>&1)"
  if [[ "$details" != *'(runtime)'* || "$details" != *'Timestamp='* ]]; then
    echo "Code lacks hardened runtime or secure timestamp: $code_path" >&2
    exit 1
  fi
done
if [[ "$(/usr/bin/lipo -archs "$app_path/Contents/MacOS/fb")" != arm64 ]]; then
  echo 'Expected the arm64-only release app; update architecture declarations before releasing another variant.' >&2
  exit 1
fi
xcrun stapler validate "$app_path"
/usr/sbin/spctl --assess --type execute --verbose=2 "$app_path"

sha256="$(/usr/bin/shasum -a 256 "$artifact" | /usr/bin/awk '{print $1}')"
# Homebrew uses version interpolation to recognize a versioned download URL.
version_reference='#{version}'
cask_url="${release_url//$version/$version_reference}"
cask_dir="$distribution_dir/casks/$version/Casks"
mkdir -p "$cask_dir"
cask_path="$cask_dir/fb.rb"
cat > "$inspection_dir/fb.rb" <<CASK
cask "fb" do
  version "$version"
  sha256 "$sha256"

  url "$cask_url"
  name "fb"
  desc "Local file browser and document viewer"
  homepage "https://github.com/xoba/fb"

  livecheck do
    skip "Updated through the signed macOS release workflow"
  end

  depends_on arch: :arm64
  depends_on macos: :tahoe

  app "fb.app"
  binary "#{appdir}/fb.app/Contents/Resources/bin/fb"
end
CASK
/bin/mv "$inspection_dir/fb.rb" "$cask_path"
echo "Prepared cask: $cask_path"
echo "Archive SHA-256: $sha256"
echo "Release URL (not checked or published): $release_url"
echo 'Review this file, publish the verified release assets, then copy Casks/fb.rb into xoba/homebrew-tap.'
echo 'Future install command: brew install --cask xoba/tap/fb'
