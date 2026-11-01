# HTML and SVG

fb renders HTML pages and SVG images in place, with their scripts turned off.

**Extensions:** `.html` `.htm` `.svg`

## What you see

The page or drawing as WebKit renders it, along with any images and stylesheets it refers
to, including remote ones.

Files are displayed under a sandboxing security policy, so anything they try to do with a
script does not happen. A page that relies on JavaScript may look incomplete. This is
deliberate: a page opened in fb could otherwise read any file you can read.

An `index.html` in a folder displays when you open it directly, but the folder's listing
still appears when you open the folder.

A file with no extension that looks like markup displays as plain text, so it cannot run
as a page.

## Options

| Option | Effect |
|--------|--------|
| `?raw=1` | Requests the original, which WebKit also renders |
| `#L12` | Shows the numbered source at that line, with a **rendered** link back. See [Linking](../linking.md#links-to-lines) |

A line link is the way to read a page's source in fb, because the original file renders.

## Limits and fallbacks

fb sets no size limit on these files.

## Try it

- [index.html](../../examples/directory-demo/index.html)
- [sample.svg](../../examples/formats/sample.svg)
- [The source of index.html](../../examples/directory-demo/index.html#L1)

[All file types](index.md) · [fb Help](../index.md)
