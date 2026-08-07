# fb

A local file browser for `http://localhost:3030` that renders files as
readable HTML instead of making you download them. Point it at a directory
(your home directory by default) and everything under it becomes browsable,
with Markdown, office documents, notebooks, source code, spreadsheets,
databases, and archives all displayed in a form meant for reading.

The server listens only on loopback, on port 3030 by default — see
[Configuration](#configuration) for changing that, and note that a taken
port moves the server to the next free one. Requires `pandoc` on the
PATH; everything else is compiled in.

## Installing

```
brew install xoba/tap/fb    # installs pandoc alongside
brew services start fb      # serve your home directory, starting at login
open http://localhost:3030/
```

Or with Go: `go install xoba.com/fb@latest` (then `brew install pandoc`
if it's not already present).

Run `fb` with no argument to serve your home directory, or pass a path
to serve a different root — `fb /` makes the whole filesystem browsable.
`fb -open ~/some/project` also opens your browser there, on whatever
port was actually bound.

### Configuration

An optional config file sets what flags otherwise would — and it's the
way to configure the brew service, which can't take flags. It lives at
`~/.config/fb/config` (`$XDG_CONFIG_HOME/fb/config` if that's set):

```
# ~/.config/fb/config
port = 8080
root = ~/notes
```

Precedence, most powerful first: the `-port` flag, then `$FB_PORT`, then
the file's `port`, then 3030; a path argument, then the file's `root`,
then your home directory. After editing the file:

```
brew services restart fb
```

### Port fallback

If the chosen port is already taken — by another fb, or anything else —
fb serves on the next free port instead of failing: 3031, then 3032, and
so on. So if `localhost:3030` shows something unexpected (or nothing),
try the next few ports, or check the log, where fb names the port it
bound: `brew services info fb` shows the log location for the brew
service (`$(brew --prefix)/var/log/fb.log`), and the from-source service
logs to `~/Library/Logs/fb.log`.

## Developing

```
./run.sh            # go run, serving / (the whole filesystem)
./run.sh ~/notes    # serve a narrower root
```

## Running as a service from source

`run.sh` runs the server in the foreground; for something that is always
there when you're logged in, install it as a macOS LaunchAgent instead:

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
`/_fb/version` reports the git revision, commit time, and dirty
flag that `go build` stamps into the binary (all `unknown` under
`go run`). `redeploy` and `install` poll the version endpoint until the
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

The exception is Markdown, which renders for every client — serving `.md`
files as HTML is the program's original purpose.

Editor backup files get the same treatment as their base file: a trailing
`~` is ignored for type detection, so `README.md~` renders like
`README.md` and `main.go~` gets Go highlighting.

## File types with special formatting

### Documents (rendered through pandoc)

| Type | Notes |
|------|-------|
| `.md` | Full pipeline: footnotes, GitHub-style heading anchors, autolinked URLs, emoji, and TeX math via embedded MathJax (no network needed). Add `?toc=1` for a table of contents. Renders for all clients, not just navigations. |
| `.ipynb` | Jupyter notebooks — markdown cells, syntax-highlighted code cells, and cell outputs. |
| `.rst` | reStructuredText documents. |
| `.docx`, `.odt`, `.rtf` | Word processor documents as clean HTML. |
| `.doc` | Legacy binary Word documents, converted by macOS's built-in `textutil` and then styled through pandoc. Falls back to download where textutil is unavailable. |
| `.epub` | Books, including their title page. |

Non-markdown formats pass `--embed-resources`, so images stored inside the
document arrive as data URIs in a single self-contained page. If pandoc
cannot parse a file, it falls back to a plain download.

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
those better), `.txt` (prose reads better plain), `.md` (pandoc's job), and
`go.mod` (chroma mis-identifies it). Files over 2 MB are served plain.

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
| `.csv`, `.tsv` | First row treated as the header. Tolerant parsing (ragged rows, lazy quotes); files over 2 MB or that fail to parse are served plain. Display capped at 2000 rows. |
| `.xlsx` | Browses like a directory: navigating to the file lists its sheets (with row counts), and each sheet renders as its own CSV-style table page. Blank rows above a sheet's data are skipped, so the first populated row becomes the header. 10 MB cap. |
| `.sqlite`, `.sqlite3`, `.db` | Browses like a directory of tables and views, each listed with its row × column counts and on-disk size including indexes (via the `dbstat` virtual table). The listing page has a SQL query box — results render as a table right beneath it. Each table's page shows the total row count, the first 2000 rows, and its highlighted schema. `NULL` shown literally, binary blobs as `(N-byte blob)`. On-disk databases are opened in place, read-only, with no size limit — sqlite pages in only what a query touches. Databases inside archives are copied to a temp file first (the driver needs a real path) and capped at 100 MB. Write statements are rejected; non-SQLite `.db` files fall back to download. |

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

### Everything else

Served verbatim with sensible content types — PDFs, video, and
audio display natively in the browser. `index.html` files are viewable
in place (the Go standard library's redirect-back-to-directory behavior is
bypassed).

## All specially handled types at a glance

| Treatment | Types |
|-----------|-------|
| Rendered as a document (pandoc) | `.md` `.rst` `.ipynb` `.doc` `.docx` `.odt` `.rtf` `.epub` |
| Syntax-highlighted source | `.ada` `.adb` `.ads` `.awk` `.bash` `.bat` `.c` `.cc` `.clj` `.coffee` `.cpp` `.cs` `.css` `.dart` `.diff` `.el` `.erb` `.erl` `.ex` `.exs` `.f` `.f90` `.feature` `.fish` `.go` `.gradle` `.graphql` `.groovy` `.h` `.hcl` `.hpp` `.hs` `.ini` `.java` `.jl` `.js` `.json` `.jsx` `.kt` `.lisp` `.lua` `.mf` `.mjs` `.nix` `.patch` `.php` `.pl` `.properties` `.proto` `.ps1` `.py` `.r` `.rb` `.rs` `.s` `.scala` `.scss` `.sh` `.sql` `.svelte` `.swift` `.tex` `.tf` `.toml` `.ts` `.tsx` `.typ` `.vue` `.xml` `.yaml` `.yml` `.zig` `.zsh` |
| Syntax-highlighted by exact filename | `Makefile` `makefile` `GNUmakefile` `Dockerfile` `CMakeLists.txt` `.bashrc` `.zshrc` |
| Highlighted as XML (binary converted via plutil) | `.plist` |
| Displayed as tables | `.csv` `.tsv` (and every sheet/table inside the containers below) |
| Browsed like directories | `.zip` `.jar` `.tar` `.tar.gz` `.tgz` `.tar.bz2` `.xlsx` `.sqlite` `.sqlite3` `.db` |
| Image pages with EXIF readout | `.jpg` `.jpeg` `.png` `.gif` `.webp` `.bmp` `.tif` `.tiff` `.heic` `.heif` |
| Video player pages | `.mov` `.mp4` `.m4v` `.webm` |

And filenames with special roles inside directory listings:

| Filename | Role |
|----------|------|
| `blurb.txt` | Shown atop its directory's listing and on the directory's row in the parent listing |
| `README.md`, `README.txt` | Rendered inline at the bottom of the listing |
| `.fb.css` | Linked as a stylesheet into markdown rendered at or below its directory |
| `index.html` | Viewable in place rather than redirecting back to the directory |

## Drag and drop

Drop any file from Finder onto any rendered page and it opens as if you
had clicked it: the file is uploaded to a per-session scratch area
(browsers deliberately hide the original path from pages, so the server
works from a copy) and the browser navigates to it, rendering through the
ordinary pipeline — a dropped notebook renders, a dropped zip browses, a
dropped database gets the query box. A floating monitor shows upload
progress; files over 100 MB (the largest viewer cap) are rejected with a
notice before any upload starts. Past drops remain browsable at
`/_fb/drops/` until the OS cleans the temp directory.

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

One Go binary (`main.go`) built around a single abstraction: a `fileServer`
that serves any `fs.FS`. The real filesystem is just `os.DirFS(root)`;
opening an archive produces another `fs.FS` (zip natively, tar via an
in-memory map), which gets wrapped in a nested `fileServer` mounted at the
archive's URL path — that is the entire archive-browsing feature. Markdown
is piped to pandoc over stdin, so rendering works identically no matter
which filesystem a file lives in.

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
is linked into rendered markdown beneath it, nearest file winning.

Everything is served with aggressive no-cache headers (the point is seeing
your *current* files), except the embedded MathJax assets, which only change
with the binary.

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


