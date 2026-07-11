# local_md

A local filesystem browser for `http://localhost:3030` that renders files as
readable HTML instead of making you download them. Point it at a directory
(typically `/`) and everything under it becomes browsable, with Markdown,
office documents, notebooks, source code, spreadsheets, databases, and
archives all displayed in a form meant for reading.

```
./run.sh            # serve / (the whole filesystem)
./run.sh ~/notes    # serve a narrower root
```

The server listens only on loopback (`127.0.0.1:3030` and `[::1]:3030`).
Requires `pandoc` on the PATH; everything else is compiled in.

## Running as a service

`run.sh` runs the server in the foreground; for something that is always
there when you're logged in, install it as a macOS LaunchAgent instead:

```
./service.sh install     # build, install the LaunchAgent, and start
./service.sh redeploy    # after changing code: rebuild and restart
./service.sh status      # launchd state plus an HTTP probe
./service.sh logs        # tail ~/Library/Logs/localmd.log
./service.sh stop        # stop until 'start' or next login
./service.sh uninstall   # stop and remove the LaunchAgent
```

The service starts at login, restarts automatically if it crashes, and
logs to `~/Library/Logs/localmd.log`. The generated plist
(`~/Library/LaunchAgents/com.xoba.localmd.plist`) runs the compiled
`localmd` binary with a PATH that includes `/opt/homebrew/bin` so pandoc
is found. The serve root defaults to `/`; install with
`LOCALMD_ROOT=~/notes ./service.sh install` to serve a narrower root.

The server exposes two probes: `/_localmd/healthz` answers `ok`, and
`/_localmd/version` reports the git revision, commit time, and dirty
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
> .scala .scss .sh .sql .svelte .swift .tex .tf .toml .ts .tsx .vue .xml
> .yaml .yml .zig .zsh`

Plus exact filenames without useful extensions: `Makefile`, `makefile`,
`GNUmakefile`, `Dockerfile`, `CMakeLists.txt`, `.bashrc`, `.zshrc` — and
`.mf` jar manifests, colored as key/value properties.

`.plist` property lists also render as highlighted XML — binary ones are
converted first with macOS's built-in `plutil` (elsewhere, binary plists
fall back to download).

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

| Type | Notes |
|------|-------|
| `.csv`, `.tsv` | First row treated as the header. Tolerant parsing (ragged rows, lazy quotes); files over 2 MB or that fail to parse are served plain. Display capped at 2000 rows. |
| `.xlsx` | Browses like a directory: navigating to the file lists its sheets (with row counts), and each sheet renders as its own CSV-style table page. 10 MB cap. |
| `.sqlite`, `.sqlite3`, `.db` | Browses like a directory of tables and views, each listed with its row × column counts and on-disk size including indexes (via the `dbstat` virtual table). The listing page has a SQL query box — results render as a table right beneath it. Each table's page shows the total row count, the first 2000 rows, and its highlighted schema. `NULL` shown literally, binary blobs as `(N-byte blob)`. On-disk databases are opened in place, read-only, with no size limit — sqlite pages in only what a query touches. Databases inside archives are copied to a temp file first (the driver needs a real path) and capped at 100 MB. Write statements are rejected; non-SQLite `.db` files fall back to download. |

`run.sh` builds with the `sqlite_dbstat` and `sqlite_fts5` tags: dbstat
powers the per-table sizes, and FTS5 lets queries against databases with
full-text-search tables work. Without the tags everything degrades
gracefully (sizes are simply omitted).

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

Navigating to a `.jpg`, `.png`, `.gif`, `.webp`, `.bmp`, `.tif`, or
`.heic` shows
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
| Syntax-highlighted source | `.ada` `.adb` `.ads` `.awk` `.bash` `.bat` `.c` `.cc` `.clj` `.coffee` `.cpp` `.cs` `.css` `.dart` `.diff` `.el` `.erb` `.erl` `.ex` `.exs` `.f` `.f90` `.feature` `.fish` `.go` `.gradle` `.graphql` `.groovy` `.h` `.hcl` `.hpp` `.hs` `.ini` `.java` `.jl` `.js` `.json` `.jsx` `.kt` `.lisp` `.lua` `.mjs` `.nix` `.patch` `.php` `.pl` `.properties` `.proto` `.ps1` `.py` `.r` `.rb` `.rs` `.s` `.scala` `.scss` `.sh` `.sql` `.svelte` `.swift` `.tex` `.tf` `.toml` `.ts` `.tsx` `.vue` `.xml` `.yaml` `.yml` `.zig` `.zsh` |
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
| `.localmd.css` | Linked as a stylesheet into markdown rendered at or below its directory |
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
`/_localmd/drops/` until the OS cleans the temp directory.

## Directory listings

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
- **Git repositories** — a listing that is itself a git repository
  directory (a `.git` directory, or a bare repository: anything with a
  `HEAD` file plus `objects` and `refs` directories) gets a section at the
  bottom showing where HEAD points, commit/branch/tag counts, object
  store size, remotes, and the eight most recent commits. Gathered by
  shelling out to `git`, so it applies only to on-disk directories, not
  archive members.
- **Layout** — directory rows have separate name and blurb cells while file
  rows span both, so files reserve no empty blurb space and long filenames
  never widen the directory column. Long names truncate with an ellipsis
  (hover for the full name).
- Directories show modification times like files do; sizes are shown only
  for files.

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
[go-sqlite3](https://github.com/mattn/go-sqlite3) feed the shared table
template. MathJax is embedded in the binary and served under `/_localmd/`,
so math works offline. Pages are styled inline — no external assets — with
a GitHub-ish look; a `.localmd.css` file in any directory (or ancestor)
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


