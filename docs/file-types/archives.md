# ZIP, JAR, and TAR archives

fb opens archives as if they were folders, without extracting anything to your disk.

**Extensions:** `.zip` `.jar` `.tar` `.tar.gz` `.tgz` `.tar.bz2`

## What you see

An ordinary [directory listing](../directory-listings.md) of the archive's contents. Every feature
works inside: source gets highlighted, Markdown renders, tables display, and READMEs and blurbs
appear. Breadcrumbs cross the boundary, as in `/ notes bundle.zip src main.go`, and the listing's
**raw** link gets you the archive itself.

Archives inside archives also open, down to three levels.

ZIP and JAR files are read on demand, so even large ones open cheaply. TAR files have no index, so
the whole archive is unpacked into memory first.

Git annotations do not appear inside archives.

## Options

| Option | Effect |
|--------|--------|
| `?raw=1` | On the archive: the original file. On a file inside: that file's bytes |
| `#L12` | On a text file inside: works as it does anywhere else. See [Linking](../linking.md#links-to-lines) |

A link can point straight at a file inside an archive:
[`tiny-archive.zip/data/colors.csv`](../../examples/formats/tiny-archive.zip/data/colors.csv)

## Limits and fallbacks

| Limit | Value |
|-------|-------|
| TAR file size, as stored | 100 MiB |
| TAR contents, unpacked | 1 GiB |
| A single file inside an archive | 128 MiB |
| Nesting | 3 levels |

TAR files past these limits fall back to the original file.

## Try it

- [tiny-archive.zip](../../examples/formats/tiny-archive.zip)
- [tiny-archive.jar](../../examples/formats/tiny-archive.jar)
- [tiny-archive.tar](../../examples/formats/tiny-archive.tar)
- [tiny-archive.tar.gz](../../examples/formats/tiny-archive.tar.gz)
- [tiny-archive.tgz](../../examples/formats/tiny-archive.tgz)
- [tiny-archive.tar.bz2](../../examples/formats/tiny-archive.tar.bz2)

[All file types](index.md) · [fb Help](../index.md)
