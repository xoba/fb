# EPUB books

fb shows an EPUB book as one long page, starting with its title page.

**Extensions:** `.epub`

## What you see

The whole book in a single scrolling page, including its title page and with its images
included in the page. Use **Edit > Find…** to search it. There is no pagination or library
as in a dedicated reader.

## Options

| Option | Effect |
|--------|--------|
| `?raw=1` | The original file, which fb offers to save |

EPUB files are binary, so they ignore [links to lines](../linking.md#links-to-lines).

## Limits and fallbacks

Files over 32 MiB are not rendered. If Pandoc cannot parse a file, fb offers to save the
original.

## Try it

[sample.epub](../../examples/documents/sample.epub)

[All file types](index.md) · [fb Help](../index.md)
