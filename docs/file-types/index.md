# All file types

fb recognizes files mostly by extension, and falls back to showing the original when a
format is not one it knows. Case does not matter, and a trailing `~` from an editor
backup is ignored, so `README.md~` renders like `README.md`.

## Documents

| Format | Extensions |
|--------|------------|
| [Markdown](markdown.md) | `.md` |
| [reStructuredText](restructuredtext.md) | `.rst` |
| [Jupyter notebooks](notebooks.md) | `.ipynb` |
| [Word processors](word-processing.md) | `.docx` `.doc` `.odt` `.rtf` |
| [EPUB books](epub.md) | `.epub` |
| [PDF](pdf.md) | `.pdf` |
| [HTML and SVG](html-svg.md) | `.html` `.htm` `.svg` |
| [Plain text](plain-text.md) | `.txt`, and other text files |

## Code

| Format | Extensions and names |
|--------|----------------------|
| [Source code](source-code.md) | Every language Chroma knows by filename, over 250, such as `.c` `.go` `.lean` `.py` `.rs` `.swift` `.ts`; see [the full list](source-code.md#languages) |
| [Source code](source-code.md), by exact name | `Makefile` `Dockerfile` `CMakeLists.txt` `.zshrc`, and [more](source-code.md#languages) |
| [JSON](json.md) | `.json` |
| [Typst](typst.md) | `.typ` |
| [Property lists](property-lists.md) | `.plist` |
| [Go module files](go-modules.md) | `go.mod` `go.work` `go.sum` `go.work.sum` |

## Messages and calendars

| Format | Extensions |
|--------|------------|
| [Email](email.md) | `.eml` |
| [Calendars](calendar.md) | `.ics` |

## Data

| Format | Extensions |
|--------|------------|
| [CSV and TSV](csv-tsv.md) | `.csv` `.tsv` |
| [Parquet](parquet.md) | `.parquet` |
| [Excel workbooks](xlsx.md) | `.xlsx` |
| [SQLite databases](sqlite.md) | `.sqlite` `.sqlite3` `.db` |

## Containers

| Format | Extensions |
|--------|------------|
| [Archives](archives.md) | `.zip` `.jar` `.tar` `.tar.gz` `.tgz` `.tar.bz2` |

## Media

| Format | Extensions |
|--------|------------|
| [Images](images.md) | `.jpg` `.jpeg` `.png` `.gif` `.webp` `.bmp` `.tif` `.tiff` `.heic` `.heif` |
| [Video](video.md) | `.mov` `.mp4` `.m4v` `.webm` |
| [Audio](audio.md) | `.mp3` `.m4a` `.aac` `.flac` `.ogg` `.wav` |

## Size limits

Past its limit, a viewer falls back to showing or offering the original file.

| Format | Limit | Past the limit |
|--------|-------|----------------|
| Documents through Pandoc | 32 MiB | Original file |
| Source code, JSON, Typst, property lists, `go.mod` files, calendars | 2 MiB | Plain text |
| CSV and TSV | 2 MiB, showing 2,000 rows | Plain text |
| Parquet | 20 MiB, reading 100,000 rows and showing 2,000 | Original file |
| Excel | 10 MiB, showing 2,000 rows per sheet | Original file |
| SQLite on disk | No limit, showing 2,000 rows per table | |
| SQLite inside an archive | 100 MiB | Original file |
| TAR | 100 MiB stored, 1 GiB extracted | Original file |
| Files inside an archive | 128 MiB | An error message |
| Nested archives | 3 levels | An error message |
| Email | 32 MiB, 16 levels of nesting, 256 parts, 2 MiB of previews | A notice, with the original source available |

A [link to a line](../linking.md#links-to-lines) still works past the 2 MiB limit, as a
numbered view without colors.

## Files fb does not recognize

Anything else is served as it is: WebKit displays it if it can, and otherwise fb offers
to save a copy. Text files with unfamiliar extensions usually display as
[plain text](plain-text.md).

## Related pages

- [Linking to files and lines](../linking.md)
- [Back to fb Help](../index.md)
