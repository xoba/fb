# Jupyter notebooks

fb shows notebooks as reading documents, without running them or needing Jupyter.

**Extensions:** `.ipynb`

## What you see

The notebook's Markdown cells, syntax-highlighted code cells, and the saved output of each
cell, as one page. Images stored in the notebook are included in the page.

fb never runs any code: you see whatever outputs were saved in the file.

## Options

| Option | Effect |
|--------|--------|
| `?raw=1` | The original file, as does the **raw** link |
| `#L12` | Shows the numbered JSON at that line. See [Linking](../linking.md#links-to-lines) |

## Limits and fallbacks

Files over 32 MiB are not rendered. If Pandoc cannot parse a file, fb falls back to the
original file.

## Try it

[sample.ipynb](../../examples/documents/sample.ipynb)

[All file types](index.md) · [fb Help](../index.md)
