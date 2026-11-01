# Plain text

Plain text displays as it is, in a monospaced font. Prose reads better without line numbers
or highlighting, so `.txt` files do not get them unless you ask.

**Extensions:** `.txt`, and most other text files fb does not otherwise recognize, such as
`.log`

## What you see

The text of the file, unchanged. Text files with an extension fb does not know display this
way if WebKit recognizes them as text. Otherwise fb offers to save a copy.

Files with no extension that look like markup display as plain text rather than as a page.

Editor backups count as the file they back up, so `notes.md~` renders as Markdown and
`main.go~` gets Go highlighting.

## Options

| Option | Effect |
|--------|--------|
| `#L12` | Adds line numbers and highlights that line. See [Linking](../linking.md#links-to-lines) |
| `#L10-L20` | Adds line numbers and highlights those lines |

Files with more than 10,000 lines show a window of lines around the target, which works
for logs of any size.

## Limits and fallbacks

fb sets no size limit on plain text.

## Try it

- [README.txt](../../examples/directory-demo/plain-text/README.txt)
- [The same file with line numbers](../../examples/directory-demo/plain-text/README.txt#L1)

[All file types](index.md) · [fb Help](../index.md)
