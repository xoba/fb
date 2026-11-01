# JSON

fb shows JSON files highlighted and with line numbers, and expands minified files so that you
can read them.

**Extensions:** `.json`

## What you see

The file with syntax coloring and line numbers. How it is laid out depends on the file:

| The file | What you see |
|----------|--------------|
| All on one line | Expanded with two-space indentation |
| On several lines | Exactly as written, so line numbers match your editor |
| Not valid JSON, or several values in a row | Exactly as written |

Leaving formatted files alone means a link such as `settings.json#L42` points at line 42 of
the file on disk. In a minified file, line numbers refer to the expanded view, since the
file has only one line of its own.

## Options

| Option | Effect |
|--------|--------|
| `#L42` | Scrolls to that line and highlights it. See [Linking](../linking.md#links-to-lines) |
| `?raw=1` | The original file, as does the **raw** link |

## Limits and fallbacks

Files over 2 MiB display as plain text instead of being highlighted or expanded. A link to a
line in one of these shows a numbered window around that line, without colors.

## Try it

[catalog.json](../../examples/source/catalog.json)

[All file types](index.md) · [fb Help](../index.md)
