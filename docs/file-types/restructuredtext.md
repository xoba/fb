# reStructuredText

reStructuredText is the markup used by Python's documentation tools. fb renders it through
Pandoc, so documents read much like [Markdown](markdown.md).

**Extensions:** `.rst`

## What you see

A formatted page with headings, lists, tables, and links. Rendered output gets the same
sanitizing as Markdown: scripts and styles are removed, while formatting and images remain.

Pandoc reads standard reStructuredText. Directives that depend on Sphinx or a project's own
extensions may display as plain text or be left out.

## Options

| Option | Effect |
|--------|--------|
| `?raw=1` | The original file, as does the **raw** link |
| `#L12` | Shows the numbered source at that line. See [Linking](../linking.md#links-to-lines) |

## Limits and fallbacks

Files over 32 MiB are not rendered. If Pandoc cannot parse a file, fb falls back to the
original file.

## Try it

[sample.rst](../../examples/documents/sample.rst)

[All file types](index.md) · [fb Help](../index.md)
