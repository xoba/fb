# Explore the examples

Open this folder in fb to try every explicitly documented file extension and
special filename. The original, self-contained samples are organized into
[documents](documents/), [source](source/), [formats](formats/), and
[directory-demo](directory-demo/).

From the repository checkout:

```sh
open -a fb examples
```

## Email and Inbox

- [mime-tour.eml](mime-tour.eml) is a fictional message with plain-text and
  HTML alternatives, an embedded CID image, a CSV attachment with a Unicode
  filename, and a forwarded message with its own headers and body. Expand
  the MIME tree and open individual parts to explore it.
- [mime-tour-reply.eml](mime-tour-reply.eml) is a plain-text reply using
  ISO-8859-1 and quoted-printable encoding.
- Select **Inbox** in this folder's listing to compare the two messages by
  sender, subject, and date, or search them. **Files** returns to the listing.

Both messages were created from scratch as local fixtures, use only
`example.com` addresses, and were never sent.

## Other viewers

| Try | Examples |
|-----|----------|
| Documents and notebooks | [Markdown](documents/sample.md), [notebook](documents/sample.ipynb), [reStructuredText](documents/sample.rst), [Word](documents/sample.docx), [legacy Word](documents/sample.doc), [OpenDocument](documents/sample.odt), [RTF](documents/sample.rtf), [EPUB](documents/sample.epub) |
| Source highlighting | [source/](source/) covers every listed extension and exact filename, including Typst, shell configuration, Makefiles, and CMake |
| Structured Go files | [go.mod](source/go.mod), [go.work](source/go.work), [go.sum](source/go.sum), [go.work.sum](source/go.work.sum) |
| Calendar cards | [sample.ics](documents/sample.ics) |
| Tables and spreadsheets | [CSV](formats/library.csv), [TSV](formats/library.tsv), [Parquet](formats/library.parquet), [XLSX](formats/library.xlsx) |
| SQLite databases | [DB](formats/library.db), [SQLite](formats/library.sqlite), [SQLite3](formats/library.sqlite3) |
| Archives | [ZIP](formats/tiny-archive.zip), [JAR](formats/tiny-archive.jar), [TAR](formats/tiny-archive.tar), [TAR.GZ](formats/tiny-archive.tar.gz), [TGZ](formats/tiny-archive.tgz), [TAR.BZ2](formats/tiny-archive.tar.bz2) |
| Images | [JPG](formats/color-grid.jpg), [JPEG](formats/color-grid.jpeg), [PNG](formats/color-grid.png), [GIF](formats/color-grid.gif), [WebP](formats/color-grid.webp), [BMP](formats/color-grid.bmp), [TIF](formats/color-grid.tif), [TIFF](formats/color-grid.tiff), [HEIC](formats/color-grid.heic), [HEIF](formats/color-grid.heif) |
| Video | [MP4](formats/test-pattern.mp4), [MOV](formats/test-pattern.mov), [M4V](formats/test-pattern.m4v), [WebM](formats/test-pattern.webm) |
| Audio | [WAV](formats/quiet-tone.wav), [M4A](formats/quiet-tone.m4a), [MP3](formats/quiet-tone.mp3), [AAC](formats/quiet-tone.aac), [FLAC](formats/quiet-tone.flac), [Ogg](formats/quiet-tone.ogg) |
| Property lists | [Info.plist](source/Info.plist) |
| Native PDF and SVG display | [PDF](formats/sample.pdf), [SVG](formats/sample.svg) |
| Directory features | [directory-demo/](directory-demo/) demonstrates inline README previews, `README.txt`, `.fb.css`, and `index.html`; [blurb.txt](blurb.txt) supplies this folder's description |

The media fixtures use generated color patterns and short, quiet tones.
The archive and database fixtures contain tiny fictional catalogs. These are
format examples; media playback still depends on the WebKit version and its
available codecs (the Ogg file remains available through **raw** if it cannot
play).

The Go checksum examples contain real public dependency checksums; the
workspace points to the small module in `source/`. Source snippets are
for browsing and are not one combined buildable project.
