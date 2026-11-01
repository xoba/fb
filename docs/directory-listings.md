# Directory listings

Opening a folder shows a listing of its contents, with a summary at the top and any
README rendered at the bottom. Try the [sample folder](../examples/directory-demo/).

## The summary

A muted line above the listing totals what it shows, for that level only and not counting
dot-files: `5 dirs · 23 files · 1.4 GB`. Workbooks and databases get the same treatment:
`3 sheets · 1240 rows`, or `9 tables · 2 views · 1693 rows · 460 KB`. For a large
database, the counts of tables appear immediately and the totals follow once they have
been calculated.

## Sorting

The selector at the top sorts by time, name, or size, with the newest first by default.
Clicking the active choice reverses its direction, and an arrow shows which way it runs.

Folders always appear above files. Sorting by name keeps a reading order: folders, then
Markdown, then everything else.

## Blurbs

A folder containing a `blurb.txt` shows that text at the top of its own listing, and on
its row in the parent folder's listing. The file must be plain text of 512 bytes or fewer.

## READMEs

A `README.md` renders at the bottom of the listing, so a folder can introduce itself.
A `README.txt` is shown as preformatted text if there is no `README.md`.

## Dot-files

Files and folders whose names start with `.` collapse into a "N dot-files" section at the
bottom of the listing. One click expands it.

## Email folders

A folder containing `.eml` files also offers an Inbox view with senders, subjects, dates,
and search. See [Email](file-types/email.md).

## Special filenames

| Filename | What it does |
|----------|--------------|
| `blurb.txt` | Shown atop its folder's listing and on the folder's row in its parent |
| `README.md`, `README.txt` | Rendered at the bottom of the listing |
| `.fb.css` | A stylesheet for Markdown at or below its folder. See [Markdown](file-types/markdown.md#custom-styles) |
| `index.html` | Viewable in place, rather than replacing the folder's listing |

Editor backups count as the file they back up: `README.md~` renders like `README.md`, and
`main.go~` gets Go highlighting.

## Long names and layout

Long names are cut short with an ellipsis, and hovering shows the full name. Folders show
modification times as files do, but sizes appear only for files.

## Related pages

- [Git awareness](git.md)
- [ZIP, JAR, and TAR archives](file-types/archives.md), which browse like folders
- [Back to fb Help](index.md)
