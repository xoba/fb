# Markdown

fb renders Markdown through Pandoc with a generous set of extensions, so most documents
written for GitHub or for note-taking apps display as you expect.

**Extensions:** `.md`

## What you see

A formatted page with headings, lists, tables, images, code blocks, and links. fb
supports:

- **Footnotes**, which appear at the end of each section.
- **Heading anchors** that follow GitHub's rule, so `## Getting Started` becomes
  `#getting-started`. See [Linking](../linking.md#links-to-headings).
- **Bare URLs** that become links without any extra markup.
- **Wiki links:** `[[name.md]]` links like `[name.md](name.md)`, and `[[name.md|Title]]`
  sets the link text.
- **Emoji** written as `:smile:`.
- **Math** with TeX, through a copy of MathJax inside the app, so no connection is needed.
- **Images** at their natural size, or at a size you choose. Both of these make an image
  300 pixels wide, shrinking it further if the window is narrow:

  ```markdown
  <img src="picture.png" alt="Preview" width="300">
  ![Preview](picture.png){width=300px}
  ```

### Math copied from chat apps

Copy buttons in chat apps often mangle math, stripping the backslashes from `\[…\]` and
`\(…\)`, turning subscript underscores into `*`, and collapsing `\\` row breaks. fb detects
this damage and repairs it before rendering. Ordinary Markdown is not affected.

### What is removed

Pandoc passes HTML in a document straight through, so fb sanitizes the result. Scripts,
styles, event handlers, inline `style` attributes, and links with non-web schemes such as
`file:` are removed, while formatting, images, anchors, and tables remain. Only
non-negative whole numbers are kept in an image's `width` and `height`, so percentages do
not work.

YAML front matter is not interpreted, because its values could add content to the page. It
displays as ordinary text.

## Options

| Option | Effect |
|--------|--------|
| `?toc=1` | Adds a table of contents, down to the third heading level |
| `?raw=1` | The original file, as does the **raw** link |
| `#L12` | Shows the numbered source at that line. See [Linking](../linking.md#links-to-lines) |

### Custom styles

A `.fb.css` file in a folder, or any folder above it, is linked into Markdown rendered at
or below that folder, and the nearest file wins. It can restyle the page and load outside
resources, but cannot run a script. Only use it in folders you trust.

See it in the [sample folder](../../examples/directory-demo/README.md).

## Limits and fallbacks

Files over 32 MiB show as their original text rather than being rendered. A `README.md`
also renders at the bottom of its folder's [listing](../directory-listings.md).

## Try it

[sample.md](../../examples/documents/sample.md)

[All file types](index.md) · [fb Help](../index.md)
