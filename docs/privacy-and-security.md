# Privacy and security

fb is a local viewer. It has no account and does not start a web server or send your files
anywhere. It is signed and notarized by its developer, and it runs with your own file
permissions, so macOS may ask for permission the first time you open certain folders.

## What fb changes on disk

Browsing is read-only. Opening files, moving between folders, and running SQL queries do
not edit, move, rename, or delete your originals.

fb does write a few things:

- **Save a Copy** writes to the destination you choose, and can replace a file there.
- **Preferences** record your zoom level and your last location, and macOS keeps your
  **Open Recent** list.
- **Temporary files** hold conversions and working copies. These include JPEG versions of
  HEIC and TIFF images, and copies of databases found inside archives.

## When fb uses the network

fb does not need a connection for ordinary viewing, and it has no telemetry. The network is
used in these cases:

| What | When | Goes to |
|------|------|---------|
| Map tiles | Viewing a photo with GPS coordinates | OpenStreetMap |
| Remote images and stylesheets | A document you open refers to them | Wherever the document says |
| Links you click | Websites and email links open outside fb | Your browser or mail app |

Requests for map tiles reveal your IP address and the general location of the photo to
OpenStreetMap. Email is stricter than other documents: its remote images are removed, so
that opening a message cannot tell its sender you read it.

## How fb handles untrusted files

Files you open might have been written by anyone, so fb limits what they can do:

- **Scripts do not run.** Scripts, styles, event handlers, and non-web links are removed
  from rendered documents. Formatting, images, anchors, and tables remain.
- **HTML and SVG are sandboxed.** They display, but their scripts never run.
- **Email is sanitized further.** Scripts, forms, message styling, and remote images are
  removed.
- **SQL is read-only.** Query boxes accept a single `SELECT`, `WITH`, or `EXPLAIN`
  statement, and databases open read-only.
- **Git is locked down.** Repository settings that could run a program are turned off.

Two exceptions are worth knowing about. A `.fb.css` file can restyle Markdown in its folder
and load outside resources, though it cannot run a script. And Git, `sips`, `textutil`, and
`plutil` come from your Mac rather than from the app.

## Where fb's parts come from

The app includes its own copy of Pandoc for converting documents, and it never uses one
installed elsewhere on your Mac. Choose **Help > Third-Party Notices** for details of
the software fb is built from.

## Related pages

- [Troubleshooting](troubleshooting.md)
- [Back to fb Help](index.md)
