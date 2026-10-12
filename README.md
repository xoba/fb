## TL;DR

The macOS file browser you always wished you had: read Markdown, documents,
notebooks, and code; explore spreadsheets and SQLite databases; and browse
inside ZIP/TAR archives.

Requires Apple Silicon and macOS 26 or later. Existing users of the old server
should [stop and uninstall it first](#migrating-from-the-server).

```sh
brew install --cask xoba/tap/fb
```

The app is signed and notarized, with everything bundled. You can also
[download it directly](https://github.com/xoba/fb/releases/latest).

## About fb

A local file browser that presents Markdown, office documents, notebooks,
source code, email messages, spreadsheets, databases, and archives in a form
meant for reading.
The macOS app embeds the Go renderer and Pandoc, reads original files in place,
and does not run a web server.

**Browsing is read-only.** Opening files, navigating folders, and running SQL
queries do not edit, move, rename, or delete your originals. Dragging a file or
folder into fb simply opens it at its existing location; it changes what you
are viewing, without importing or moving the item.

fb saves app preferences, such as zoom and the last opened location, and may
create temporary files for rendering and conversion. **Save a Copy** is an
explicit write to the destination you choose and can replace an existing file
there.

**The standalone macOS app is the primary interface.** The older command-line
HTTP server is deprecated and retained for compatibility. Its Go rendering
code remains shared with the app; deprecating the server does not remove the
viewers. Download the [signed and notarized macOS release](https://github.com/xoba/fb/releases/latest)
or install it through Homebrew Cask.

- [Installing](#installing) · [Open from the terminal](#open-from-the-terminal)
- [Legacy server configuration](#configuration) · [Port fallback](#port-fallback)
- [The one rule: navigations render, everything else is raw](#the-one-rule-navigations-render-everything-else-is-raw)
- [File types with special formatting](#file-types-with-special-formatting) · [at a glance](#all-specially-handled-types-at-a-glance)
- [Drag and drop](#drag-and-drop)
- [Directory listings](#directory-listings)
- [Git awareness](#git-awareness)
- [How it works, generally](#how-it-works-generally)

## Installing

### Migrating from the server

If you installed the old Homebrew formula, stop its service and uninstall the
formula **before** installing the app cask:

```sh
brew services stop fb
brew uninstall --formula xoba/tap/fb
brew install --cask xoba/tap/fb
```

The cask does not stop or uninstall the old server automatically. This is a
switch from a formula to a cask; `brew upgrade fb` does not perform it. Removing
the old server does not remove your documents, and the app reads them in place.

If you installed the server's login service from a source checkout, run
`./service.sh uninstall` there before installing the app. Quit any server you
started manually in a terminal as well. The app needs no running server.

For a fresh installation, skip the server removal steps and use the cask
installation below.

### macOS app

The native app in [`macos/`](macos/README.md) runs on Apple Silicon with
macOS 26 or later. It embeds the same Go rendering engine and a pinned Pandoc
inside a SwiftUI/WebKit app. It does not start a web server or require Homebrew
on the user's Mac. Finder drops and Open With read original files and folders
in place; multiple dropped items open separate windows.

Choose View → Zoom In, Zoom Out, or Actual Size (⌘0) to adjust document zoom.
fb remembers your last chosen zoom level for new windows and future launches.
Existing windows keep their current zoom until you change it there.

Install the signed, notarized app:

```sh
brew install --cask xoba/tap/fb
```

Or download the app ZIP from [GitHub Releases](https://github.com/xoba/fb/releases/latest)
and move `fb.app` to your Applications folder. Homebrew is optional; the download
contains everything needed for ordinary document viewing.

To build, install, and open it from source:

```sh
macos/Scripts/deploy.sh
open ~/Applications/fb.app
```

Building requires Xcode, XcodeGen, Go, Python 3, and internet access to fetch the pinned
Pandoc archive once. The installed app needs none of those build tools.
Local builds use an available Apple Development identity; public distribution
requires Developer ID signing and notarization. See the
[Mac build and distribution instructions](macos/README.md).

The cask installs the same signed and notarized ZIP available from GitHub
Releases. Update it with `brew upgrade --cask xoba/tap/fb`. Releases also include
checksums, dependency notices, and Pandoc source materials. See the
[distribution workflow](macos/Distribution.md) for release and migration steps.

### Open from the terminal

macOS's `open` command sends files and folders directly to the installed app:

```sh
open -b com.xoba.fb.app -- README.md
open -b com.xoba.fb.app -- .
open -b com.xoba.fb.app -- "My notes.md" "/path/to/folder"
```

Relative paths work, and multiple items open in separate windows. Quote paths
containing spaces. Pass paths directly, without `--args`, so macOS delivers
file-open events to the app.

For a shorter `fb` command in macOS's default zsh shell, add this function to
`~/.zshrc`, then open a new terminal or paste the function into your current
shell:

```sh
fb() {
  if [ "$#" -eq 0 ]; then
    set -- .
  fi
  /usr/bin/open -b com.xoba.fb.app -- "$@"
}
```

Then use `fb README.md`, `fb "My notes.md" /path/to/folder`, or just `fb` to open
the current directory. This is an optional shell shortcut; the cask does not
currently install an `fb` executable. The deprecated formula's `fb` executable
starts the HTTP server, so follow the migration steps above first.

No public URL scheme is registered. The `fb://` addresses used inside the app
are private to its web view; use filesystem paths with `open` instead. If you
have several development copies installed, select a specific app explicitly:

```sh
open -a "$HOME/Applications/fb.app" -- .
```

### Command-line browser and service (deprecated)

These instructions are for existing server users. New macOS installations
should use the native app above.

```
brew install xoba/tap/fb    # installs pandoc alongside
brew services start fb      # serve your home directory, starting at login
open http://localhost:3030/
```

The formula lives in the [xoba/homebrew-tap](https://github.com/xoba/homebrew-tap)
repository — the install command taps it automatically. New releases
arrive with `brew upgrade fb`, and a running service picks them up on
its own: fb watches its binary on disk and, within a minute of an
upgrade replacing it, drains in-flight requests and exits so launchd
relaunches the new build (brew itself never restarts services). Manual
terminal runs never self-restart, and `selfrestart = off` in the config
file opts a service out. `fb -version` prints the installed version.

Or with Go: `go install xoba.com/fb@latest` (then `brew install pandoc`
if it's not already present).

Run `fb` with no argument to serve your home directory, or pass a path
to serve a different root — `fb /` makes the whole filesystem browsable.
`fb -open ~/some/project` also opens your browser there, on whatever
port was actually bound.

The legacy server listens only on loopback, on port 3030 by default. It requires
`pandoc` on PATH. **Loopback is its only access control: it has no accounts,
tokens, or passwords.** Other local users on a shared machine can read the
served files; use the native app instead or serve a narrow root. The native
app opens no listening port and uses the current user's filesystem permissions.

### Configuration

For the legacy server, an optional config file sets what flags otherwise would — and it's the
way to configure the brew service, which can't take flags. It lives at
`~/.config/fb/config` (`$XDG_CONFIG_HOME/fb/config` if that's set):

```
# ~/.config/fb/config
port = 8080
root = ~/notes
selfrestart = off   # optional: services otherwise restart on upgrade
```

Precedence, most powerful first: the `-port` flag, then `$FB_PORT`, then
the file's `port`, then 3030; a path argument, then the file's `root`,
then your home directory. After editing the file:

```
brew services restart fb
```

### Port fallback

For the legacy server, if the chosen port is already taken — by another fb, or anything else —
fb serves on the next free port instead of failing: 3031, then 3032, and
so on. So if `localhost:3030` shows something unexpected (or nothing),
try the next few ports, or check the log, where fb names the port it
bound: `brew services info fb` shows the log location for the brew
service (`$(brew --prefix)/var/log/fb.log`), and the from-source service
logs to `~/Library/Logs/fb.log`.

## Developing

```
macos/Scripts/deploy.sh  # build and install the latest native app
macos/Scripts/smoke-test.sh
go test ./...
go test -race -tags fbmac ./...
```

Native distribution follows [the macOS release workflow](macos/Distribution.md).
The old server scripts remain available for explicit compatibility work:

```sh
./run.sh ~/notes
./release.sh --legacy-server -n patch  # preview an explicit legacy release
```

`release.sh` requires `--legacy-server`; it is not the app release command.
It gates on a clean tree and tests/scans, tags and pushes a legacy version,
and updates the Homebrew formula. It never starts or redeploys a local server.

## Running as a service from source (deprecated)

For existing server users, `run.sh` runs in the foreground and `service.sh`
manages the old login LaunchAgent. The native app needs neither:

```
./service.sh install     # build, install the LaunchAgent, and start
./service.sh redeploy    # after changing code: rebuild and restart
./service.sh status      # launchd state plus an HTTP probe
./service.sh logs        # tail ~/Library/Logs/fb.log
./service.sh stop        # stop until 'start' or next login
./service.sh uninstall   # stop and remove the LaunchAgent
```

The service starts at login, restarts automatically if it crashes, and
logs to `~/Library/Logs/fb.log`. The generated plist
(`~/Library/LaunchAgents/com.xoba.fb.plist`) runs the compiled
`fb` binary with a PATH that includes `/opt/homebrew/bin` so pandoc
is found. The serve root defaults to `/`; install with
`FB_ROOT=~/notes ./service.sh install` to serve a narrower root.

The server exposes two probes: `/_fb/healthz` answers `ok`, and
`/_fb/version` reports the release version (stamped at build time; brew
and `go install` builds carry their tag), the git revision, commit
time, and dirty flag that `go build` stamps into the binary (`unknown`
under `go run`), and the process uptime — a fresh uptime beside a new
version is how a self-restart after an upgrade shows itself. `redeploy` and `install` poll the version endpoint until the
served revision matches git HEAD, so a redeploy either confirms the new
build is actually the one serving or fails loudly.

## The one rule: navigations render, everything else is raw

Every fancy view applies only when a **browser navigates** to a file —
detected via the `Sec-Fetch-Dest: document` header (or an `Accept` header
preferring `text/html`). All other requests get the original bytes untouched:

- `curl`, scripts, and pipelines see the real file.
- `<link>` stylesheets, `<script>`, `fetch()` and other subresource loads
  work normally — a page's CSS is never accidentally replaced by a syntax
  highlighting view of that CSS.
- `?raw=1` forces the original bytes even in a browser; rendered pages have
  a `raw` link in the top-right corner for exactly this.

The exception is Markdown, which renders for every client unless `?raw=1`
explicitly requests its source.

Freshness also applies going backwards: a page restored from the
browser's back/forward cache reloads itself, since that cache resurrects
the page as frozen at navigation time — stale git annotations and all —
ignoring the server's no-store headers. Media player pages are exempt,
so Back onto a video or audio page keeps its playback position.

Browsing is read-only: fb does not edit, move, rename, or delete the originals
it displays.
Format-conversion caches and temporary database copies live in scratch space;
even git is invoked with `--no-optional-locks` so browsing leaves repositories
untouched. The native app saves preferences such as zoom and the last opened
location. Save a Copy writes only to your chosen destination and can replace
an existing file there.
The legacy browser also uses scratch copies for file uploads.

Editor backup files get the same treatment as their base file: a trailing
`~` is ignored for type detection, so `README.md~` renders like
`README.md` and `main.go~` gets Go highlighting.

## File types with special formatting

### Documents (rendered through pandoc)

| Type | Notes |
|------|-------|
| `.md` | Full pipeline: footnotes, GitHub-style heading anchors, autolinked URLs, wiki links (`[[name.md]]` links like `[name.md](name.md)`, `[[name.md\|Title]]` sets the link text), emoji, and TeX math via embedded MathJax (no network needed). Math mangled by chat UIs' copy-as-markdown (ChatGPT's copy button strips the backslashes from `\[…\]` and `\(…\)` delimiters, turns subscript underscores into `*`, and collapses `\\` row breaks) is detected and repaired before rendering; ordinary markdown is untouched. Add `?toc=1` for a table of contents. Renders for all clients, not just navigations. |
| `.ipynb` | Jupyter notebooks — markdown cells, syntax-highlighted code cells, and cell outputs. |
| `.rst` | reStructuredText documents. |
| `.docx`, `.odt`, `.rtf` | Word processor documents as clean HTML. |
| `.doc` | Legacy binary Word documents, converted by macOS's built-in `textutil` and then styled through pandoc. Falls back to download where textutil is unavailable. |
| `.epub` | Books, including their title page. |

Non-markdown formats pass `--embed-resources`, so images stored inside the
document arrive as data URIs in a single self-contained page. If pandoc
cannot parse a file, it falls back to a plain download. Files over 32 MB
are served plainly rather than rendered.

Rendered output is sanitized before serving: pandoc passes raw HTML from a
document straight through, and these pages run on fb's origin, so scripts,
styles, event handlers, and non-web link schemes are stripped while
formatting, images, anchors, and tables survive. Markdown YAML front
matter is not interpreted (metadata could inject into the page head); it
shows as ordinary text.

### Source code (highlighted by chroma)

About sixty extensions render as GitHub-styled pages with clickable,
linkable line numbers (`#L42`):

> `.ada .adb .ads .awk .bash .bat .c .cc .clj .coffee .cpp .cs .css .dart
> .diff .el .erb .erl .ex .exs .f .f90 .feature .fish .go .gradle .graphql
> .groovy .h .hcl .hpp .hs .ini .java .jl .js .json .jsx .kt .lisp .lua
> .mjs .nix .patch .php .pl .properties .proto .ps1 .py .r .rb .rs .s
> .scala .scss .sh .sql .svelte .swift .tex .tf .toml .ts .tsx .typ .vue
> .xml .yaml .yml .zig .zsh`

Plus exact filenames without useful extensions: `Makefile`, `makefile`,
`GNUmakefile`, `Dockerfile`, `CMakeLists.txt`, `.bashrc`, `.zshrc` — and
`.mf` jar manifests, colored as key/value properties.

`.plist` property lists also render as highlighted XML — binary ones are
converted first with macOS's built-in `plutil` (elsewhere, binary plists
fall back to download).

`.json` files are pretty-printed (two-space indent) before highlighting;
files that don't parse as JSON show as-is. Oversized ones display as plain
text in the browser instead of downloading.

`.typ` files are reformatted with [typstyle](https://typstyle-rs.github.io/typstyle/)
before highlighting, when it's installed; without it (or for sources typstyle
can't parse) the file shows as written. Note the displayed line numbers are
the formatter's, not the file's.

Deliberately *not* highlighted: `.html` and `.svg` (the browser renders
those better — inline, but sandboxed, so any scripts they carry don't
run), `.txt` (prose reads better plain), and `.md` (pandoc's job). Files
over 2 MB are served plain.

### Go module files

`go.mod` and `go.work` render as structured pages, parsed by
[golang.org/x/mod/modfile](https://pkg.go.dev/golang.org/x/mod/modfile)
(the go command's own parser): the module path, go version and toolchain
up top, then direct and indirect dependencies in separate tables with
every module linked to its pkg.go.dev page at that exact version, then
replacements, exclusions, retractions and tools. Directory replacements
and `use` entries link into the browser itself. The canonically formatted
source (the `go mod edit -fmt` layout, where the comments live) follows,
with line numbers.

`go.sum` and `go.work.sum` tabulate one checksum line per row, modules
linked the same way. A module file that doesn't parse shows as plain
numbered text.

### Email messages

`.eml` files open like a directory with a README: a tree of MIME parts comes
first, followed by the readable message. The tree preserves the message's
structure, including multipart containers, plain-text and HTML alternatives,
inline images, attachments, and forwarded messages. Click a part to inspect
it, then use Back to return to the message. Headers and the original message
source are available in disclosures; **Save original .eml** returns the
complete original file.

Attachments open with the same viewers as files elsewhere in fb: CSV files
become queryable tables, PDFs display in the browser, images get the image
viewer, documents render normally, and archives open as browsable folders.
**Save this part** also lets you save the decoded attachment as a separate
file. Plain-text and HTML message bodies retain their email previews.

Folders containing `.eml` files also offer an **Inbox** view with sender,
subject, and date columns and search. Each row opens the same MIME browser;
the normal directory listing remains available. This browses local mail files
without connecting to an email account or changing message state.
Navigating to another folder returns to the normal file browser; inbox mode
applies only to the folder where you select it.

The reader decodes MIME transfer encodings, encoded header values, and
declared character sets. Message-body HTML is sanitized: scripts, forms,
message styling, and remote images are removed. Embedded CID images can display without a
network request. The message and its attachments remain unchanged; saving a
copy is optional. Messages inside ZIP/TAR archives work the same way.

Message parsing is limited to 32 MiB, 16 levels of MIME nesting, and 256 parts.
Body and source previews are capped at 2 MiB. Inbox pages contain up to 500
files grouped by filename; sorting by date and searching apply to the current
page. The inbox reads only headers, capped at 64 KiB per message.
Unsupported or malformed content reports a warning or falls back to source;
the original bytes remain accessible.

### Calendar files

`.ics` files — meeting invitations, cancellations and replies, exported
calendars, subscribed feeds — render as event cards rather than
downloading (the browser otherwise hands `text/calendar` straight to
Calendar). Each card shows when, in your local time with the event's own
zone alongside when it differs (a Windows-style zone name the system
doesn't know, like `Eastern Standard Time`, shows the time as written);
the recurrence rule in words ("every 2 weeks on Tuesday and Thursday, 10
times", exceptions listed); where, with URLs linked; the organizer and
every attendee with their response; the description; links and
reminders. Status shows as a badge (tentative, cancelled, private), as
does what the file is (invitation, cancellation, reply). Every property
of the component follows in a collapsed table, and the whole source
after the cards, so nothing in the file is out of reach.

Files with more than ten events start collapsed, in date order, as an
agenda. To-dos, journal entries and free/busy blocks get cards of their
own kind. The parser is a small tolerant reading of RFC 5545 written for
this: folding, quoted and multi-valued parameters, escapes, `BEGIN`/`END`
nesting — and whatever it can't read, it skips rather than fails on. A
file with no calendar in it at all shows as plain numbered text.

### Tabular data

All three viewers share one look: bordered cells, a frozen header, a frozen
row-number column on the left, and a frozen column-number row on top, so
coordinates stay visible while scrolling big tables. Coordinate cells are
tinted pale blue to distinguish them from real data.

Cells longer than 200 characters are cut short. When the full content is
plain text (valid UTF-8, no control characters beyond ordinary whitespace)
the cut ends in a "… more" link to a page showing the complete cell, with
breadcrumbs and a back link to the table; binary-ish content just gets an
ellipsis. This works in every table view, including SQL query results.
The back link returns to the cell's spot in the table — it carries the
coordinates in the URL fragment, and the table scrolls the cell into view
and flashes it — rather than resetting to the top-left corner.

A cell whose text is a single well-formed `http://` or `https://` URL is
hyperlinked (opening in a new tab, so the table keeps its place); a cut
URL cell links its visible prefix to the full URL.

| Type | Notes |
|------|-------|
| `.csv`, `.tsv` | First row treated as the header. Tolerant parsing (ragged rows, lazy quotes); files over 2 MB or that fail to parse are served plain. Display capped at 2000 rows. The page has a SQL query box — the file is queryable as a table named `t`. |
| `.parquet` | Renders and queries like csv (table `t`), decoded in pure Go. Column order follows the schema; NULLs show empty. 20 MB cap, first 100k rows decoded (the summary says when that bites). Files that fail to decode fall back to download. |
| `.xlsx` | Browses like a directory: navigating to the file lists its sheets (with row counts), and each sheet renders as its own CSV-style table page. Blank rows above a sheet's data are skipped, so the first populated row becomes the header. 10 MB cap. The workbook listing has a SQL query box with one table per sheet — joins across sheets work. |
| `.sqlite`, `.sqlite3`, `.db` | Browses like a directory of tables and views, each listed with its row × column counts and on-disk size including indexes (via the `dbstat` virtual table). The listing page has a SQL query box (its placeholder names the first real table) — results render as a table right beneath it. Each table's page shows the total row count, the first 2000 rows, and its highlighted schema. `NULL` shown literally, binary blobs as `(N-byte blob)`. On-disk databases are opened in place, read-only, with no size limit — sqlite pages in only what a query touches. Databases inside archives are copied to a temp file first (the driver needs a real path) and capped at 100 MB. The query box accepts only a single read-only statement (`SELECT`, `WITH`, or `EXPLAIN`) — enforced before the query reaches sqlite, on top of `mode=ro` and `PRAGMA query_only` — and non-SQLite `.db` files fall back to download. |

The query boxes on csv/tsv/parquet pages and xlsx listings run real
SQLite: the file is loaded into an in-memory database — a csv, tsv, or
parquet file as a table
named `t`, a workbook as one table per sheet, with sheet names sanitized
to identifiers (`Q1 Sales` is queried as `Q1_Sales`). Columns whose
cells all parse as numbers get numeric typing, so `sum()` and `order by`
behave numerically. Everywhere the box appears, the same rule applies:
one read-only statement (SELECT, WITH, EXPLAIN) per query.

The SQLite driver is pure Go ([modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite)),
with dbstat (powering the per-table sizes) and FTS5 (so queries against
databases with full-text-search tables work) built in — no cgo, no C
toolchain, no build tags.

A bonus of the directory model: fetching a sheet or table URL with
`?raw=1` (or from a non-browser client) exports *that member* as CSV —
`curl localhost:3030/finances.xlsx/revenue` emits real CSV.

### Archives (browsed like directories)

| Type | Notes |
|------|-------|
| `.zip`, `.jar` | Read lazily via the central directory — cheap even for large archives. |
| `.tar`, `.tar.gz`, `.tgz`, `.tar.bz2` | Tars have no index, so the whole archive is extracted into memory. Browsable when the file itself is at most 100 MB (as stored, compressed or not); larger ones fall back to download. A 1 GB extraction ceiling guards against decompression bombs. |

Navigating to an archive shows a normal directory listing of its contents,
and *every* feature recurses inside: members get syntax highlighting,
markdown rendering, table views, readme and blurb display — even archives
nested inside archives (to depth 3). Breadcrumbs cross the archive boundary
(`/ notes bundle.zip src main.go`), and the listing's `raw` link downloads
the archive itself.

### Images

Navigating to a `.jpg`/`.jpeg`, `.png`, `.gif`, `.webp`, `.bmp`,
`.tif`/`.tiff`, or `.heic`/`.heif` shows
the image scaled to fit the browser window — up or down, aspect ratio
preserved, regardless of its native dimensions — with a technical readout
beneath it: file size, dimensions and
megapixels, MIME type (from the actual content), modification time — and
the full EXIF block when present, with photography fields formatted
naturally (`f/7.1`, `85 mm`, `1/200 s`). GPS-tagged photos additionally
get a map below the EXIF with a dot at the shot's location, composed of
static OpenStreetMap tiles (plain images — the one feature that needs
the network), with links out to OpenStreetMap and Apple Maps.
Subresource and non-browser fetches get the raw bytes as usual, so
`<img>` tags elsewhere keep working. HEIC and TIFF images — which most
browsers cannot display — are converted to JPEG with macOS's `sips`
(EXIF and GPS survive the conversion), cached per file, and shown with
the full readout and map.

### Video

Navigating to a `.mov`, `.mp4`, `.m4v`, or `.webm` shows an inline
player scaled to fit the browser window (like images, independent of
the movie's native dimensions) with file size, format, and modification
time beneath it; duration and dimensions fill in from the browser's own
decoder once the metadata loads.

### Audio

Navigating to a `.mp3`, `.m4a`, `.aac`, `.flac`, `.ogg`, or `.wav`
shows an inline player with file size, format, and modification time
beneath it — plus whatever tags the file carries (title, artist, album,
year, genre, track, composer) and its embedded cover art, all read in
pure Go. Duration fills in from the browser's own decoder, like video.
(Safari doesn't play `.ogg`; the raw link always downloads.)

### Everything else

Served verbatim with sensible content types — PDFs display natively in
the browser (video and audio get the player pages above instead).
`index.html` files are viewable
in place (the Go standard library's redirect-back-to-directory behavior is
bypassed). HTML and SVG are served under a sandboxing Content Security
Policy, so they render but their scripts never run on fb's origin, and
extensionless files that sniff as markup display as plain text.

## All specially handled types at a glance

| Treatment | Types |
|-----------|-------|
| Rendered as a document (pandoc) | `.md` `.rst` `.ipynb` `.doc` `.docx` `.odt` `.rtf` `.epub` |
| Syntax-highlighted source | `.ada` `.adb` `.ads` `.awk` `.bash` `.bat` `.c` `.cc` `.clj` `.coffee` `.cpp` `.cs` `.css` `.dart` `.diff` `.el` `.erb` `.erl` `.ex` `.exs` `.f` `.f90` `.feature` `.fish` `.go` `.gradle` `.graphql` `.groovy` `.h` `.hcl` `.hpp` `.hs` `.ini` `.java` `.jl` `.js` `.json` `.jsx` `.kt` `.lisp` `.lua` `.mf` `.mjs` `.nix` `.patch` `.php` `.pl` `.properties` `.proto` `.ps1` `.py` `.r` `.rb` `.rs` `.s` `.scala` `.scss` `.sh` `.sql` `.svelte` `.swift` `.tex` `.tf` `.toml` `.ts` `.tsx` `.typ` `.vue` `.xml` `.yaml` `.yml` `.zig` `.zsh` |
| Syntax-highlighted by exact filename | `Makefile` `makefile` `GNUmakefile` `Dockerfile` `CMakeLists.txt` `.bashrc` `.zshrc` |
| Highlighted as XML (binary converted via plutil) | `.plist` |
| Structured module pages | `go.mod` `go.work` `go.sum` `go.work.sum` |
| Email reader and hierarchical MIME browser | `.eml` |
| Calendar event cards | `.ics` |
| Displayed as tables | `.csv` `.tsv` `.parquet` (and every sheet/table inside the containers below) |
| Browsed like directories | `.zip` `.jar` `.tar` `.tar.gz` `.tgz` `.tar.bz2` `.xlsx` `.sqlite` `.sqlite3` `.db` |
| Image pages with EXIF readout | `.jpg` `.jpeg` `.png` `.gif` `.webp` `.bmp` `.tif` `.tiff` `.heic` `.heif` |
| Video player pages | `.mov` `.mp4` `.m4v` `.webm` |
| Audio player pages with tag readout | `.mp3` `.m4a` `.aac` `.flac` `.ogg` `.wav` |

And filenames with special roles inside directory listings:

| Filename | Role |
|----------|------|
| `blurb.txt` | Shown atop its directory's listing and on the directory's row in the parent listing |
| `README.md`, `README.txt` | Rendered inline at the bottom of the listing |
| `.fb.css` | Linked as a stylesheet into markdown rendered at or below its directory |
| `index.html` | Viewable in place rather than redirecting back to the directory |

Try the [example gallery](examples/README.md) for sample files covering every
listed extension and special filename, including a multipart email and a
second message for exploring Inbox view.

## Drag and drop

In the macOS app, dragging in a file or folder is a navigation action: it opens
that item for viewing at its existing location. Finder/Desktop drops use
original filesystem URLs, without uploading, importing, moving, or editing the
items. The first item opens in the current window; additional items open in
separate windows. Rendering may use temporary conversion files. The following
scratch-copy behavior applies only to the deprecated web-browser version.

Drop any file from Finder onto any rendered page and it opens as if you
had clicked it: the file is uploaded to a per-session scratch area
(browsers deliberately hide the original path from pages, so the server
works from a copy) and the browser navigates to it, rendering through the
ordinary pipeline — a dropped notebook renders, a dropped zip browses, a
dropped database gets the query box. A floating monitor shows upload
progress; files over 100 MB (the largest viewer cap) are rejected with a
notice before any upload starts. Past drops remain browsable at
`/_fb/drops/` until they age out — the oldest are evicted once drops
total over 1 GB — or the OS cleans the temp directory.

Drop a *folder* and nothing is uploaded — the folder already lives on the
machine this server browses, so the page simply navigates to its listing:
drag-and-drop as `cd`. Safari hands over the folder's `file://` URL
directly; Chrome and Firefox reveal only the folder's name and contents,
so the server searches for it in order of where drags actually come from:
the Desktop first (walked directly, so the common case resolves without
waiting on Spotlight), then Spotlight over the whole serve root, then a
breadth-first walk that probes the home directory and the serve root in
lockstep. Every candidate is verified against the child names seen in the
browser, with the folder's modification time and then the same
Desktop-then-home prior breaking ties. If nothing matches, or several
folders match equally, the drop fails with a notice instead of guessing.
Folders outside the serve root are rejected by name.

## Directory listings

- **Summary** — a muted headline above the listing totals what it shows:
  `5 dirs · 23 files · 1.4 GB` (that level only, dot-files excluded).
  Container listings get the same treatment: `3 sheets · 1240 rows` for a
  workbook, `9 tables · 2 views · 1693 rows · 460 KB` for a database. For
  large on-disk databases the member counts render instantly and the
  row/byte totals are appended once the async per-table stats finish.
- **Sorting** — `sort: time · name · size` selector at the top; newest
  first by default. Clicking the active key reverses its direction (the
  arrow shows which way); directories always group above files. Name sort
  preserves the original reading order: directories, then markdown, then
  everything else.
- **Blurbs** — a directory containing a `blurb.txt` (plain text, ≤ 512
  bytes) shows its contents at the top of its own listing, and on its row
  in the parent's listing, in muted text between the name and the date.
- **Readmes** — a `README.md` renders inline at the bottom of the listing
  (via pandoc); `README.txt` is shown preformatted as a fallback.
- **Email** — folders containing `.eml` files offer an Inbox view with sender,
  subject, date, and search; individual messages have a MIME part tree with
  the readable message below it. See [Email messages](#email-messages).
- **Dot-files** — entries starting with `.` collapse into a "N dot-files"
  disclosure section at the bottom; one click expands them.
- **Git** — listings inside a git worktree annotate entries with their
  status; see [Git awareness](#git-awareness) below.
- **Layout** — directory rows have separate name and blurb cells while file
  rows span both, so files reserve no empty blurb space and long filenames
  never widen the directory column. Long names truncate with an ellipsis
  (hover for the full name).
- Directories show modification times like files do; sizes are shown only
  for files.

## Git awareness

Browsing inside a git working tree, listings annotate what git knows,
without ever touching the repository.

**Per-entry badges.** Entries carry small colored letters after their
names: amber `M` modified, green letter staged (`A` new file, `M`
modified, and so on), gray `?` untracked, red `!` merge conflict, muted
`i` gitignored. Hovering any badge explains the state in words. A
subdirectory containing changes gets a dot whose hover text counts them
(`2 modified, 1 untracked within`). A file deleted from HEAD but gone
from disk keeps a ghost row — struck-through, unlinked — where it used
to be, as does a vanished tracked directory, so deletions stay visible
until committed.

**The `git:` line.** The worktree's top-level listing adds a muted line
under the summary — branch, sync against the upstream as of the last
fetch (`ahead 2 of origin/main`), and dirty counts, with anything
actionable in amber; `clean · in sync` otherwise. Listings deeper in the
worktree get the same line with just the counts, scoped to their own
subtree, and only when something is dirty there.

**Repository directories.** A listing that is itself a repository — a
`.git` directory, or a bare repository (anything with a `HEAD` file plus
`objects` and `refs` directories) — gets a section at the bottom showing
where HEAD points, commit/branch/tag counts, object store size, remotes,
and the eight most recent commits.

**Mechanics.** One `git status` scoped to the listed directory per page
view, run with `--no-optional-locks` so browsing never mutates the
repository, and a 3-second timeout after which the listing simply
renders unannotated. Repository sections shell out to `git` as well, so
both features apply only to on-disk directories, not archive members;
without `git` on the PATH, listings render plainly and nothing breaks.

## How it works, generally

The Go rendering engine is built around a single abstraction: a `fileServer`
that serves any `fs.FS`. The real filesystem is `os.OpenRoot(root).FS()` —
a root-confined filesystem, so a symlink inside the tree cannot point
browsing at files outside it; opening an archive produces another `fs.FS`
(zip natively, tar via an in-memory map), which gets wrapped in a nested
`fileServer` mounted at the archive's URL path — that is the entire
archive-browsing feature. Markdown is piped to pandoc over stdin, so
rendering works identically no matter which filesystem a file lives in.

The macOS app links this engine into its process and routes WebKit's `fb:`
requests through the in-memory bridge in `desktop_engine.go`. It never calls
the legacy CLI's listener startup. Only the deprecated server exposes HTTP.

Request dispatch lives in one `route` function: stat the path; directories
get listings; then, by extension, pandoc documents, table views, syntax
highlighting, and archive descent — all gated on the navigation check — and
anything unmatched is served raw. If the path doesn't exist directly, the
router walks its prefixes to find an archive to descend into.

Rendering choices: pandoc converts documents (the only external program);
[chroma](https://github.com/alecthomas/chroma) highlights code in pure Go;
`encoding/csv`, [excelize](https://github.com/xuri/excelize), and
[modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) feed the shared
table template. MathJax is embedded in the binary and served under `/_fb/`,
so math works offline. Pages are styled inline — no external assets — with
a GitHub-ish look; a `.fb.css` file in any directory (or ancestor)
is linked into rendered markdown beneath it, nearest file winning. That
trusts the directory's CSS: it can restyle the page and load external
URLs (though never run script), so don't browse hostile trees expecting
their `.fb.css` to be inert.

Everything is served with aggressive no-cache headers (the point is seeing
your *current* files), except the embedded MathJax assets, which only change
with the binary.

Security posture: the server is unauthenticated by design, so loopback is
the whole boundary — on a shared machine, every local user can read what
you serve (see the warning at the top of this README). Against *web*
attackers it binds loopback only and rejects requests whose `Host` isn't
localhost or a loopback IP (DNS-rebinding defense); POST endpoints
require a same-origin fetch marker (CSRF defense); every response carries
`nosniff`, a resource policy, framing and referrer restrictions; pandoc
output is sanitized; raw HTML/SVG is sandboxed; databases open read-only
with the query box restricted to single read-only statements; and git
probing runs with a scrubbed environment and hostile repo config
(`core.fsmonitor`) pinned off. See `security_review.md` for the full
assessment.

## original readme before work began

i want to make a localhost browser which is always on, which enables
me to browse my filesystem but automatically converting md files
to html on the fly, so i can view them better.

let's write it in the go language. but let it call out to pandoc
for the *.md files. we can follow the style of ~/desktop-stuff/html
script.

let it use func ServeFileFS(w ResponseWriter, r *Request, fsys
fs.FS, name string) from net/http package.  it will open up an fs.FS
in the directory it is started in. it will just pass all files
as-is, as it normally operates, but for *.md files, it will convert
to html on the fly and serve that instead.

let's use http://localhost:3030 as our endpoint.


