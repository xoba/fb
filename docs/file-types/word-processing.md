# Word processor documents

fb shows word processor documents as clean, readable pages, so you can read them without
opening an office app.

**Extensions:** `.docx` `.doc` `.odt` `.rtf`

## What you see

The document's text, headings, lists, tables, and images, converted to a web page. Images
stored in the document are included in the page itself.

This is a reading view rather than a faithful reproduction. Exact fonts, page layouts,
headers, and footers may not carry over. To see the original, use **File > Save a Copy** and
open the copy in its own app.

| Extension | Format | Converted by |
|-----------|--------|--------------|
| `.docx` | Microsoft Word | Pandoc |
| `.doc` | Older binary Microsoft Word | macOS's `textutil`, then Pandoc |
| `.odt` | OpenDocument Text | Pandoc |
| `.rtf` | Rich Text Format | Pandoc |

## Options

| Option | Effect |
|--------|--------|
| `?raw=1` | The original file, which fb offers to save |
| `#L12` | For `.rtf` only, which is text: shows the numbered source at that line |

The other formats are binary, so they ignore [links to lines](../linking.md#links-to-lines).

## Limits and fallbacks

Files over 32 MiB are not rendered. If a file cannot be converted, fb offers to save the
original.

## Try it

- [sample.docx](../../examples/documents/sample.docx)
- [sample.doc](../../examples/documents/sample.doc)
- [sample.odt](../../examples/documents/sample.odt)
- [sample.rtf](../../examples/documents/sample.rtf)

[All file types](index.md) · [fb Help](../index.md)
