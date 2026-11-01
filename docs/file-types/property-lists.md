# Property lists

Property lists hold settings for Mac and iOS apps. fb shows them as highlighted XML, whichever
form the file is stored in.

**Extensions:** `.plist`

## What you see

The list as syntax-colored XML with line numbers. Binary property lists are converted with
macOS's built-in `plutil` first, so you can read them as well.

XML property lists also pass through `plutil`, which can change their formatting. Line numbers
belong to the converted text in either case, so a
[link to a line](../linking.md#links-to-lines) can differ from your editor.

## Options

| Option | Effect |
|--------|--------|
| `#L42` | Scrolls to that line and highlights it |
| `?raw=1` | The original file |

## Limits and fallbacks

Files over 2 MiB are not converted or highlighted. A binary property list that cannot be
converted falls back to the original file, and other files that `plutil` rejects show as
written.

## Try it

[Info.plist](../../examples/source/Info.plist)

[All file types](index.md) · [fb Help](../index.md)
