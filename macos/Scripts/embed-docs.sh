#!/bin/bash
set -euo pipefail

# Copies the documentation and the sample files it links to into the app. They
# keep their positions relative to each other, so a link such as
# ../../examples/formats/library.csv works the same in the bundle, in a checkout,
# and on GitHub. The examples folder also holds large local-only files that
# .gitignore excludes, so the list comes from git rather than the filesystem.

mac_dir="$(cd "$(dirname "$0")/.." && pwd)"
repo_dir="$(cd "$mac_dir/.." && pwd)"

: "${TARGET_BUILD_DIR:?This script runs as an Xcode build phase}"
: "${UNLOCALIZED_RESOURCES_FOLDER_PATH:?This script runs as an Xcode build phase}"
resources="$TARGET_BUILD_DIR/$UNLOCALIZED_RESOURCES_FOLDER_PATH"

rm -rf "$resources/docs" "$resources/examples"
mkdir -p "$resources"

cd "$repo_dir"
count=0
while IFS= read -r -d '' file; do
  [[ -f "$file" ]] || continue
  mkdir -p "$resources/$(dirname "$file")"
  /usr/bin/install -m 644 "$file" "$resources/$file"
  count=$((count + 1))
done < <(git ls-files -z --cached --others --exclude-standard -- \
  docs examples/README.md examples/blurb.txt \
  examples/mime-tour.eml examples/mime-tour-reply.eml \
  examples/documents examples/formats examples/source examples/directory-demo)

if [[ ! -f "$resources/docs/index.md" ]]; then
  echo "Documentation is missing from the bundle: $resources/docs/index.md" >&2
  exit 1
fi
echo "Embedded $count documentation and sample files" >&2
