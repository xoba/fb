# Go module files

fb presents Go's module files as structured pages rather than text, using the Go tool's own
parser.

**Files:** `go.mod` `go.work` `go.sum` `go.work.sum`

## What you see

**`go.mod` and `go.work`** show the module path, Go version, and toolchain at the top. Direct
and indirect dependencies follow in separate tables, with each module linked to its
[pkg.go.dev](https://pkg.go.dev) page at that exact version. After those come replacements,
exclusions, retractions, and tools. Replacements pointing at folders, and `use` entries, link
into fb itself.

A **Source** section at the end shows the file with line numbers in the canonical layout that
`go mod edit -fmt` produces. It keeps the file's comments, which the tables do not show.

**`go.sum` and `go.work.sum`** show one checksum per table row, with modules linked the same
way.

These pages open only with file names exactly as above. A file named `other.mod` is not
treated as a module file.

## Options

| Option | Effect |
|--------|--------|
| `#L12` | In `go.mod` and `go.work`: scrolls to that line of the Source section. In the `.sum` files: shows the numbered file, with a **rendered** link back |
| `?raw=1` | The original file, as does the **raw** link |

Because the Source section uses the canonical layout, its line numbers can differ from an
unformatted file. See [Linking](../linking.md#when-the-numbers-differ-from-your-editor).

## Limits and fallbacks

Files over 2 MiB show as plain text. A `go.mod` or `go.work` file that does not parse shows as
written, with line numbers to help you find the problem.

## Try it

- [go.mod](../../examples/source/go.mod)
- [go.work](../../examples/source/go.work)
- [go.sum](../../examples/source/go.sum)
- [go.work.sum](../../examples/source/go.work.sum)

[All file types](index.md) · [fb Help](../index.md)
