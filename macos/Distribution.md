# Homebrew Cask distribution

The direct-download package is the signed, notarized ZIP containing the
standalone app. The cask in `xoba/homebrew-tap/Casks/fb.rb` installs that app. It
has no Homebrew Go, Pandoc, or server dependency. Version 0.1.4 (build 6) is
published at [mac-v0.1.4](https://github.com/xoba/fb/releases/tag/mac-v0.1.4), with
checksums, dependency notices, and the Pandoc source distribution. Its uploaded
bytes were verified, and the cask passed Homebrew style, online audit, and an
installation test. This workflow describes how to prepare subsequent releases.

## Prepare a reviewable release

1. Build and verify a release from the intended committed source. Use the local
   Developer ID workflow in `Scripts/package.sh`, or export through an authorized
   Xcode cloud-signing account. The app and its bundled helper must have the
   expected Developer ID signatures, hardened runtime, and secure timestamps.
   The [archive example](README.md#cloud-developer-id-signing) requires a clean
   checkout and sets `FB_BUILD_REVISION` from Git so About File Browser identifies the source.
2. Notarize the exact release and staple the ticket to the app. Create the final
   ZIP **after** stapling; `package.sh` does this when `NOTARY_PROFILE` is supplied.
   For a cloud-signed export or an existing signed app/ZIP, use
   `notarize-existing.sh` with an existing Keychain profile:

   ```sh
   NOTARY_PROFILE='your-existing-profile' \
     macos/Scripts/notarize-existing.sh /absolute/path/to/export/fb.app \
     macos/.build/distribution/fb-0.1.4-arm64
   ```

   Alternatively, replace `NOTARY_PROFILE` with
   `ASC_RELEASE_KEY_PATH=/absolute/path/to/AuthKey_6B7NTN43DS.p8` for an existing
   team API key with file permissions `600`. Choose one authentication method
   and a fresh output directory. The script preserves the input, verifies both
   Developer ID signatures, submits to Apple, and produces the final stapled ZIP
   and checksum only after acceptance and validation. Use `--check-only` for
   local signature/architecture checks without submission. See the
   [notarization instructions](README.md#notarize-an-existing-signed-export).
3. Choose the GitHub release tag and exact asset URL. A separate Mac tag namespace,
   such as `mac-v0.1.4`, avoids confusing the earlier server release tags.
4. Generate the cask from the actual notarized artifact:

   ```sh
   macos/Scripts/generate-cask.sh \
     macos/.build/distribution/fb-0.1.4-arm64/fb-0.1.4-arm64-notarized.zip \
     https://github.com/xoba/fb/releases/download/mac-v0.1.4/fb-0.1.4-arm64-notarized.zip
   ```

The script reads the version and macOS minimum from the ZIP's app, checks both
Developer ID signatures, confirms Apple Silicon architecture, validates the
stapled ticket, checks Gatekeeper, and calculates the ZIP's SHA-256. Only after
those checks pass does it write
`.build/distribution/casks/<version>/Casks/fb.rb`. The URL is the explicit intended
release URL; the script does not assert it is already hosted. No placeholder
checksum, unsigned artifact, or guessed release version is emitted.

The generator currently supports the app's macOS 26 / arm64 baseline. If that
changes, update the generator's validation and cask requirements together.
`livecheck` is skipped because this repository also contains legacy server tags;
cask updates come from this verified release workflow. Normal `brew upgrade
--cask xoba/tap/fb` still follows the version committed to the tap.

## Publish after review

Publish the exact notarized ZIP, checksum, notices, and required source material
to the chosen `xoba/fb` GitHub release. Verify the uploaded bytes match the local
checksum, then copy the reviewed cask into the tap's `Casks` directory. Run
Homebrew's cask style/audit checks and test the install before committing the tap
update. No script here publishes a GitHub release, creates a tag, or edits/pushes
the tap automatically.

For users migrating from the old Homebrew formula, stop and remove the server
before installing the cask:

```sh
brew services stop fb
brew uninstall --formula xoba/tap/fb
brew install --cask xoba/tap/fb
```

For a server login service installed from source, run `./service.sh uninstall`
in its checkout before installing the app. Quit any manually started server.
These steps leave documents in place; the app needs no server. The cask does
not stop services or uninstall the formula automatically.

For a fresh installation, only the cask step is needed:

```sh
brew install --cask xoba/tap/fb
```

Homebrew normally installs apps in `/Applications`. To use the local development
deployment location, pass `--appdir="$HOME/Applications"`. If a manually installed
copy already exists there, resolve that installation explicitly before handing
the location to Homebrew; do not force-overwrite it blindly.

The legacy formula and app cask can have the same token because `--cask` selects
the cask. A plain `brew install fb` can still select the formula while it exists.
Use explicit `--cask` for installs/upgrades and `--formula` when removing the
legacy command-line package. Retiring its launch agent/service is a separate
migration step; the cask should not silently run service-changing shell hooks.

## Distribution requirements

Developer ID signing and notarization must succeed before a cask is generated.
An Apple Development signature used for local installation is insufficient for
this distribution workflow. The generator does not create certificates or fix
Apple account permissions.

The app bundles Pandoc under GPL-2.0-or-later. Preserve its license/notices and
provide the complete corresponding source for the distributed build and linked
components as required by that license; see
[`Dependencies/NOTICE.md`](Dependencies/NOTICE.md). Assemble the pinned source
asset with `python3 macos/Scripts/prepare-pandoc-source.py` and publish the
resulting `.build/distribution/pandoc-3.12.1-macos-arm64-source.tar.gz` and its
checksum alongside the app. Its manifest records identified component versions
and conservative source alternatives; `Dependencies/Pandoc-SOURCE.txt` documents
the upstream build-plan limitations. Do not publish until those release
materials are ready.

## References

- [Homebrew Cask Cookbook](https://docs.brew.sh/Cask-Cookbook): checksum, URL, app
  artifact, architecture, and minimum macOS declarations.
- [Maintaining a tap](https://docs.brew.sh/How-to-Create-and-Maintain-a-Tap#casks):
  casks and formulae may coexist in a tap; casks live under `Casks`.
- [Homebrew manual](https://docs.brew.sh/Manpage#install-options-formulacask-):
  `--cask` selects casks, `--appdir` controls the app installation directory.
- [Apple notarization workflow](https://developer.apple.com/documentation/security/customizing-the-notarization-workflow):
  signing, notarization, ticket stapling, and distribution packaging.
