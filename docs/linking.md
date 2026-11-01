# Linking to files and lines

fb resolves links in your documents the way a browser does, so ordinary Markdown links
can reach another document, a heading inside it, a line of a source file, or a cell of a
table.

## Links between files

Links are relative to the document containing them. A link can point to anything fb can
show, including folders and files inside archives:

```markdown
[Installation notes](setup.md)
[The parent folder](../)
[A file inside a ZIP](bundle.zip/src/main.go)
```

Absolute paths start at the root of your disk, as in `/Volumes/Archive/notes/setup.md`.

`file://` links do not work. fb removes them for safety when it renders a document, so
use a relative or absolute path instead.

## Links to headings

Every heading in a Markdown document gets an anchor based on GitHub's rule: lowercase,
punctuation removed, and spaces replaced with hyphens. A repeated heading gets `-1`,
`-2`, and so on.

```markdown
[Installing](setup.md#installing)
[The same page](#links-to-headings)
```

Markdown also supports wiki-style links. `[[setup.md]]` links to that file, and
`[[setup.md|Setup notes]]` sets the link's text. See [Markdown](file-types/markdown.md).

## Links to lines

Add `#L` and a line number to link to a line of a text file, or give two numbers to link
to a range:

```markdown
[The main function](main.go#L42)
[The whole loop](main.go#L10-L20)
```

These are the same forms GitHub uses, so one link works in both places. fb scrolls the
line into view and highlights it. Try it:
[line 5 of a Go file](../examples/source/go/catalog.go#L5), or
[lines 6 through 8](../examples/source/go/catalog.go#L6-L8).

To get the anchor for a line you are looking at, hover over its line number. The link
preview at the bottom of the window shows it.

### Which files can be linked to

**Every text file**, whether or not it normally shows line numbers:

| File | What a line link shows |
|------|------------------------|
| [Source code](file-types/source-code.md), [JSON](file-types/json.md), [Typst](file-types/typst.md), [property lists](file-types/property-lists.md) | The usual highlighted page |
| [`go.mod` and `go.work`](file-types/go-modules.md) | The usual page, with its Source section |
| [Calendars](file-types/calendar.md) | The usual page, with its Source section opened |
| [Markdown](file-types/markdown.md), [reStructuredText](file-types/restructuredtext.md), [HTML](file-types/html-svg.md), [CSV](file-types/csv-tsv.md), `go.sum`, and other files with a formatted view | The file's numbered source, with a **rendered** link back to the usual view |
| [Plain text](file-types/plain-text.md), logs, and other text files | The file with line numbers added |

So `README.md#L12` shows line 12 of the Markdown source, not the rendered document. In a
CSV file, `#L5` means line 5 of the file rather than row 5 of the table.

**Binary files** ignore a line link and open normally. This covers images, video, audio,
archives, PDFs, Word and EPUB files, spreadsheets, and databases. So does any file that
is not valid UTF-8 text around the requested line.

### Long files

A file larger than 2 MiB, or a file with no numbered view of its own and more than 10,000
lines, shows the 2,000 lines on each side of the target rather than the whole file. The
page is quick to display however large the file is, although fb has to read up to the line
first, which takes a moment deep in a very large log. **Earlier lines** and **Later lines**
move through the file, and a target past the end of the file shows its last lines.

These pages do not have syntax coloring. Source files within 2 MiB always open whole with
their usual coloring, however many lines they have, though ones with many thousands of lines
can be slow to open.

### When the numbers differ from your editor

fb's line numbers match the file on disk, with these exceptions:

- [JSON](file-types/json.md) that is all on one line is expanded so that it can be read.
- [Typst](file-types/typst.md) files are reformatted when `typstyle` is available.
- [`go.mod` and `go.work`](file-types/go-modules.md) show the Go tool's canonical layout.
- Binary [property lists](file-types/property-lists.md) have no lines of their own, so
  the numbers refer to the XML that fb converts them to.

### The short form

On a page that already shows line numbers, `#42` works the same as `#L42`. On other pages
only the `L` form is treated as a line, because a heading can have a numeric anchor:
`notes.md#2024` goes to a heading named 2024.

### Limitations

A rendered document cannot switch to its own numbered source with a link to itself. A
link inside `README.md` to `README.md#L12` stays on the rendered page, because WebKit
treats it as a jump within the page and fb is never asked for a new view. Links from any
other file work.

A heading given the custom anchor `{#L12}` cannot be linked to, because the line meaning
takes priority.

Line links are for documents rather than the command line, because the `open` command
cannot pass an anchor to fb. It treats a `#` in a path as part of the filename, and macOS
removes the anchor from a `file://` URL.

## Links to table cells

Tables take a `#cell=row,col` anchor, counting data rows and columns from 1. fb scrolls
the cell into view and flashes it: [row 2, column 3](../examples/formats/library.csv#cell=2,3).

## Options in a link

A query can select a particular view of a file:

| Add to a link | Effect | Applies to |
|---------------|--------|------------|
| `?raw=1` | The original file, unrendered | Any file |
| `?toc=1` | Adds a table of contents | [Markdown](file-types/markdown.md) |
| `?view=inbox` | The Inbox view | [Folders containing email](file-types/email.md) |

## Related pages

- [Markdown](file-types/markdown.md)
- [Navigating](navigating.md)
- [Back to fb Help](index.md)
