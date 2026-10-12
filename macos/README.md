# fb for macOS

The native app targets Apple Silicon and macOS 26 or later. SwiftUI hosts the
existing web interface in WebKit. The Go rendering engine is linked into the
app as a C archive; the app does not launch an HTTP server or use localhost.
Pandoc 3.12.1 is bundled as a private converter executable. End users need
neither Homebrew nor a separate Pandoc installation.

**Browsing is read-only:** opening files, navigating folders, and running SQL
queries do not edit, move, rename, or delete your originals. Dropping a file or
folder into the app changes what you are viewing and opens the original in
place.

Existing server users should first follow the
[stop/uninstall migration steps](../README.md#migrating-from-the-server).
For a fresh installation, download the
[signed and notarized release](https://github.com/xoba/fb/releases/latest)
directly, or run `brew install --cask xoba/tap/fb`.

## Build locally

The developer machine needs Xcode, XcodeGen 2.44 or later, Python 3, and the Go
version specified by the repository's `go.mod`. From the repository root:

```sh
macos/Scripts/build.sh
open macos/.build/xcode/Build/Products/Debug/fb.app
```

The script downloads the pinned official Pandoc ZIP into the ignored `.build`
directory and verifies its SHA-256, signature, architecture, version, and library
dependencies. It builds the Go archive, generates `FB.xcodeproj`, and builds the
Swift app. `project.yml` is the source of truth; do not commit the generated
project. The initial download requires a network connection; ordinary use of
local content does not.

The build also verifies the bundled fb, Go dependency, and MathJax license
notices. After changing Go modules, the Go toolchain, or embedded MathJax,
regenerate and review them with `python3 macos/Scripts/prepare-go-notices.py`.

Local builds select the first available Apple Development identity, falling
back to an ad-hoc signature. Override with `FB_SIGN_IDENTITY='…'`, or use
`FB_SIGN_IDENTITY=-` for an explicit ad-hoc build. These signatures are for local
development and do not establish a notarized public download. The app enables
the hardened runtime and is not sandboxed in this direct-download version.

```sh
CONFIGURATION=Release macos/Scripts/build.sh
```

Additional arguments are forwarded to `xcodebuild`. The generated project can
be opened in Xcode after the first script build; the Go archive must be rebuilt
with the script when Go sources change.

## Redeploy locally

"Redeploy" means build the latest source and install the resulting app to
`~/Applications/fb.app`:

```sh
macos/Scripts/deploy.sh
```

This runs an incremental Release build, stages and verifies the complete bundle,
then replaces the installed copy with rollback on failure. Only a running app
at that installed path is asked to quit; it reopens after installation. Running
Debug copies are left alone. The source revision appears in About fb; builds
from an uncommitted working tree carry a `-dirty` suffix. Commit before deploying
when the installed app should identify a clean revision. The build script also
accepts an explicit `FB_BUILD_REVISION` override.

Local redeployment uses the available development signature. It does not perform
Developer ID distribution signing or notarization, contact those services, or
publish the app.

To install an already notarized release built from the current clean checkout,
preserving its distribution signature and ticket:

```sh
macos/Scripts/deploy.sh --notarized-app /absolute/path/to/notarized/fb.app
```

This verifies the recorded Git revision, Developer ID signatures, stapled ticket,
and Gatekeeper acceptance before installing it to the same location.

An existing Dock shortcut from an older development build can retain a stale
bookmark and show a placeholder after fb quits. Remove that shortcut from the
Dock, open `~/Applications/fb.app`, and choose Options → Keep in Dock on its
new icon. Removing the shortcut does not remove the app or documents.

## Opening files and folders

From a terminal, pass files or directories to the app through macOS:

```sh
open -b com.xoba.fb.app -- README.md
open -b com.xoba.fb.app -- .
```

Multiple paths and paths containing spaces are supported. See
[Open from the terminal](../README.md#open-from-the-terminal) for an optional
`fb` shell shortcut and how to select a particular installed app. The cask does
not ship a CLI executable, and the app has no public URL scheme.

Use File → Open (⌘O), Finder's Open With, or drop files and folders directly
onto the document view. Dropping items is navigation: the first opens in the
current window, and additional items open in separate windows. The app uses
the original filesystem URLs without uploading, importing, moving, or editing
the items. Browsing and SQL queries remain read-only. Rendering and conversion
may create temporary files, and the app saves preferences such as zoom and the
last opened location.

The toolbar and menus provide Back/Forward, Reload, Reveal in Finder, Find
(⌘F), zoom, and Save a Copy (⇧⌘S). Find temporarily replaces the path in the
same toolbar row; Escape or Done restores the path. External links open in
the default browser.
Save a Copy writes only to a destination explicitly selected in the save panel
and can replace an existing file there.
The most recent filesystem location is remembered across launches.

View → Zoom In and Zoom Out adjust document zoom between 50% and 300%.
Your last chosen level is saved for new windows and future launches; existing
windows keep their current zoom. Actual Size (⌘0) resets the current window and
the saved default to 100%.

This direct-download build uses the current user's filesystem permissions and
normal macOS privacy prompts. It is not a sandboxed Mac App Store build. Git
annotations are optional and omitted when Git/developer tools are unavailable;
ordinary document viewing does not require them.

## Verification

```sh
go test ./...
go test -race -tags fbmac ./...
go vet -tags fbmac ./...
macos/Scripts/smoke-test.sh
```

The last command needs a logged-in graphical Mac session and Python 3. It
creates disposable fixtures under `.build/smoke`, compiles the actual Swift/Go
bridge, and exercises WebKit loading, Pandoc, math, relative resources, native
file/folder pasteboard URLs, SQL form submission and script requests, archive
members, email MIME navigation and decoded attachments, embedded email images,
inbox listings, HTML script sandboxing, audio metadata, and PDF navigation. A temporary
window is required because WebKit defers media loading for unattached views.

## Signing and packaging

The public channels are a notarized GitHub Release download and
`brew install --cask xoba/tap/fb`. Version 0.1.2 (build 4) is published as
[`mac-v0.1.2`](https://github.com/xoba/fb/releases/tag/mac-v0.1.2). See
[Distribution.md](Distribution.md) for the concrete release workflow and
legacy-server migration. Local redeployment is separate from publication.

The local packaging script requires an installed **Developer ID Application**
certificate with its matching private key. It does not create certificates or
change the Apple developer account. App Store Connect credentials are not
required for this signing path. With an existing identity:

```sh
DEVELOPER_ID_IDENTITY='Developer ID Application: Your Name (TEAMID)' \
  macos/Scripts/package.sh
```

This builds Release, signs Pandoc before signing the containing app, verifies
the signatures, and writes a ZIP and checksum under `.build/distribution`.
The file is explicitly named `signed` until notarization succeeds.

To submit to Apple's notarization service using credentials you have already
stored in Keychain, supply the profile name:

```sh
DEVELOPER_ID_IDENTITY='Developer ID Application: Your Name (TEAMID)' \
NOTARY_PROFILE='your-existing-profile' macos/Scripts/package.sh
```

The script submits the ZIP, waits for acceptance, staples and validates the
ticket, checks Gatekeeper, and emits a separate `notarized` ZIP. It never
submits without `NOTARY_PROFILE` and does not publish or upload a release.

### Cloud Developer ID signing

If the Mac has no local Developer ID private key, Xcode's archive/export workflow
can use Apple's cloud-managed Developer ID certificate. This is separate from
the `codesign`-based packaging script above. Sign into Xcode > Settings >
Accounts with the Account Holder, or an account granted cloud Developer ID
access. The script can also authenticate with an existing team API key that has
the required cloud Developer ID permission; successful iOS signing alone does
not prove that permission.

Commit the intended release source and run a local build to prepare the matching
Go archive, Pandoc, and generated project. From the repository root, create a
development-signed archive. The clean-checkout guard prevents archiving
uncommitted source, and the explicit revision keeps About fb tied to the commit:

```sh
test -z "$(git status --porcelain)" &&
xcodebuild archive -project macos/FB.xcodeproj -scheme FB \
  -configuration Release -destination 'generic/platform=macOS' \
  -derivedDataPath macos/.build/xcode -archivePath macos/.build/FB.xcarchive \
  CODE_SIGN_STYLE=Manual CODE_SIGN_IDENTITY='Apple Development' \
  DEVELOPMENT_TEAM=3288VRMQE7 \
  FB_BUILD_REVISION="$(git rev-parse --short HEAD)"
```

Then export using the account already signed into Xcode:

```sh
macos/Scripts/export-cloud.sh --xcode-account macos/.build/FB.xcarchive
```

This mode does not require, read, or pass API key settings. As an alternative,
use an existing team API key that is authorized for cloud Developer ID signing:

```sh
ASC_RELEASE_KEY_PATH=/absolute/path/to/AuthKey_6B7NTN43DS.p8 \
  macos/Scripts/export-cloud.sh --api-key macos/.build/FB.xcarchive
```

Key ID, issuer, and team can be overridden; see `--help`. This invokes
`xcodebuild -exportArchive` with `method=developer-id`, `destination=export`, and
automatic distribution signing. Running it contacts Apple and can create
managed certificates/profiles. It does not publish the app or upload it to App
Store Connect. Xcode 27 permits automatic export from a manually signed archive;
the installed `xcodebuild -help` documents these options.

The script verifies the app **and the bundled Pandoc executable** have the
expected team's Developer ID signatures, hardened runtime, and secure
timestamps. It produces a signed ZIP, with notarization still a separate step.
An Apple permission error is preserved in the export log without retries that
change the account or credentials. See Apple's [cloud-managed certificates
guide](https://developer.apple.com/help/account/certificates/cloud-managed-certificates/)
and [macOS distribution-signing
workflow](https://developer.apple.com/documentation/xcode/creating-distribution-signed-code-for-the-mac/).

### Notarize an existing signed export

Use `notarize-existing.sh` on the Developer ID-signed app or ZIP produced by the
cloud export. It also accepts an existing locally signed release. No local
Developer ID private key is needed for this step. Check the signatures and
architecture without submitting anything:

```sh
macos/Scripts/notarize-existing.sh --check-only /absolute/path/to/export/fb.app
```

Submit using an existing Keychain profile:

```sh
NOTARY_PROFILE='your-existing-profile' \
  macos/Scripts/notarize-existing.sh /absolute/path/to/export/fb.app \
  macos/.build/distribution/fb-0.1.2-arm64
```

Alternatively, use an existing team API key with file permissions `600`:

```sh
ASC_RELEASE_KEY_PATH=/absolute/path/to/AuthKey_6B7NTN43DS.p8 \
  macos/Scripts/notarize-existing.sh /absolute/path/to/export/fb.app \
  macos/.build/distribution/fb-0.1.2-arm64
```

Choose one authentication method, and use a fresh output directory. Key ID,
issuer, team, and timeout overrides are described by `--help`. The script
preserves the input, submits a staged copy to Apple, and keeps the submission
result and available developer log. Only after acceptance, stapling, ticket
validation, and Gatekeeper checks does it create
`fb-<version>-arm64-notarized.zip` and its SHA-256 file in the output directory.
It does not build, sign, install, tag, or publish the app.

Prepare the Pandoc source distribution before publishing:

```sh
python3 macos/Scripts/prepare-pandoc-source.py
```

This verifies and packages the pinned source archives and Cabal revisions under
`.build/distribution`. Publish `pandoc-3.12.1-macos-arm64-source.tar.gz` and its
checksum alongside the app. The source includes Pandoc, GHC/runtime, linked
components, conservative alternatives where the stripped binary omits version
information, original notices, and a reconstruction helper. See
[`Dependencies/Pandoc-SOURCE.txt`](Dependencies/Pandoc-SOURCE.txt) for the source
inventory and upstream build-plan limitations. The app embeds the component
notices and provenance; packaging copies them next to its ZIP.

## Updating Pandoc

Update `Dependencies/pandoc.json` deliberately, including the asset digest from
the official GitHub release API. Update the corresponding notices and source
materials at the same time. Then rebuild and exercise the Markdown/document
viewers. Never substitute a Homebrew executable: it can link libraries outside
the bundle.

The initially verified 3.12.1 arm64 binary is about 192 MB uncompressed and links
only macOS system libraries. Its official download is about 42 MB. The binary
includes Pandoc's data files; it does not include TeX or user-provided filters.
