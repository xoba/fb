# Typst

[Typst](https://typst.app) is a markup-based typesetting language. fb shows its source files
highlighted, reformatted when it can be.

**Extensions:** `.typ`

## What you see

The source with syntax coloring and line numbers, not the typeset document.

If the separate [typstyle](https://typstyle-rs.github.io/typstyle/) formatter is available,
fb runs the file through it before displaying it. Otherwise, or if typstyle cannot parse the
file, it shows as written.

fb looks for `typstyle` on its own search path. When fb is opened from Finder, this path
usually leaves out the folders Homebrew and Cargo install into, so the file then shows as
written even if you have it installed.

**Line numbers belong to what is displayed.** After reformatting they can differ from your
editor, including in a [link to a line](../linking.md#links-to-lines).

## Options

| Option | Effect |
|--------|--------|
| `#L42` | Scrolls to that line and highlights it |
| `?raw=1` | The original file, as does the **raw** link |

## Limits and fallbacks

Files over 2 MiB display as plain text. A link to a line in one of these shows a numbered
window around that line, without colors.

## Try it

[catalog.typ](../../examples/source/catalog.typ)

[All file types](index.md) · [fb Help](../index.md)
