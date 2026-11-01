# fb for macOS 0.1.8

fb is a standalone local file browser for Apple Silicon Macs running macOS 26
or later. It presents documents, source code, spreadsheets, databases, and
archives in a readable interface.

Version 0.1.8 (build 10) adds built-in documentation and links to specific lines.

- Read the full documentation from the Help menu, including a page for every
  file type with sample files to try. The pages are ordinary Markdown displayed
  by fb. Type in the Help menu's search field to jump to a topic.
- Link to a line of a file with `#L42`, or to a range with `#L10-L20`. fb scrolls
  to the lines and highlights them. Files that normally appear without line
  numbers, such as Markdown, open in a numbered view with a link back to the
  rendered page.
- Open very large text files at a linked line quickly. fb shows 4,000 lines
  around the target, with links to earlier and later lines.
- Keep Back and Forward up to date after links within a page, and keep the anchor
  when a link opens in a new window.
- Keep the author's layout in formatted JSON, so line numbers match the file. Only
  minified JSON is reformatted.

**Migrating from the old Homebrew server?** Stop its service and uninstall the
formula before installing the app:

```sh
brew services stop fb
brew uninstall --formula xoba/tap/fb
brew install --cask xoba/tap/fb
```

The cask does not stop or remove the old server automatically. For a server
login service installed from a source checkout, run `./service.sh uninstall`
there first. Quit any manually started server. Your documents remain in place;
the standalone app needs no server.

For a fresh installation, run only:

```sh
brew install --cask xoba/tap/fb
```

If you already installed the app cask, update it with:

```sh
brew update
brew upgrade --cask xoba/tap/fb
```

Or download `fb-0.1.8-arm64-notarized.zip` below. `SHA256SUMS` covers the app,
dependency notices, and Pandoc source archive.

- Open files and folders from Finder, the Desktop, Open With, or File → Open.
  Dropped items are opened at their original locations; multiple items open
  separate windows.
- Open a file or directory from the terminal with
  `open -b com.xoba.fb.app -- README.md` or `open -b com.xoba.fb.app -- .`.
  See the [terminal instructions](https://github.com/xoba/fb#open-from-the-terminal)
  for an optional `fb` shell shortcut.
- Read Markdown with math, office documents, notebooks, EPUB books, highlighted
  source code, calendars, and supported PDF, image, audio, and video files.
- Browse spreadsheets, SQLite databases, and archives. Run read-only SQL
  queries against supported tabular formats.
- Browse email inboxes and MIME parts, viewing attachments with the same file
  viewers used elsewhere in the app. Save decoded attachments separately when
  needed.
- Use native Back/Forward, Reload, Find, Print, zoom, Reveal in Finder, and
  Save a Copy. The most recent location is remembered across launches.
- The Go rendering engine and Pandoc 3.12.1 are bundled in the app. No local
  web server, listening port, Homebrew installation, or separate Pandoc setup
  is required.

Place `fb.app` in your Applications folder and open it. Browsing uses your
account's filesystem access and normal macOS privacy permissions. Local
documents and math work offline; linked remote resources and map tiles need
an internet connection. Conversion caches may use temporary files. Git
annotations are optional and require Git to be available.

The app is a viewer: browsing and SQL queries leave original documents
unchanged. Save a Copy writes to the destination you select. Some formats
have display or size limits; the interface reports limits where applicable.

Pandoc and other bundled components retain their respective licenses. See the
dependency notices and corresponding source materials supplied with the
distribution.
