# Source code

fb shows source files as highlighted, GitHub-styled pages with line numbers you can click
and link to.

**Extensions:** `.ada` `.adb` `.ads` `.asm` `.awk` `.bash` `.bat` `.c` `.cc` `.clj` `.coffee`
`.cpp` `.cs` `.css` `.dart` `.diff` `.el` `.erb` `.erl` `.ex` `.exs` `.f` `.f90` `.feature`
`.fish` `.go` `.gradle` `.graphql` `.groovy` `.h` `.hcl` `.hpp` `.hs` `.ini` `.java` `.jl`
`.js` `.jsx` `.kt` `.lisp` `.lua` `.mf` `.mjs` `.nix` `.patch` `.php` `.pl` `.properties`
`.proto` `.ps1` `.py` `.r` `.rb` `.rs` `.s` `.scala` `.scss` `.sh` `.sql` `.svelte` `.swift`
`.tex` `.tf` `.toml` `.ts` `.tsx` `.vue` `.xml` `.yaml` `.yml` `.zig` `.zsh`

**Exact filenames:** `Makefile` `makefile` `GNUmakefile` `Dockerfile` `CMakeLists.txt`
`.bashrc` `.zshrc`

## What you see

The file with syntax coloring and a line number beside every line. Click a line number to
highlight that line and add it to your history. Copying code leaves the line numbers out.

Highlighting is done by [Chroma](https://github.com/alecthomas/chroma) inside the app. If it
only partly understands a file, the rest appears as uncolored text rather than being marked
as errors.

Some formats have particular handling:

- **`.asm`** shows comments, labels, directives, instructions, registers, and numeric
  operands, with the original spacing kept. Mixed RISC-V and accelerator code can use both
  `#` comments and immediates such as `#0.5` and `#-127`.
- **`.s`** uses the GNU assembler's syntax, which suits the many dialects that share this
  extension best.
- **`.mf`** Java manifests are colored as key and value pairs.

Related formats have pages of their own: [JSON](json.md), [Typst](typst.md),
[property lists](property-lists.md), and [Go module files](go-modules.md). Some formats are
deliberately not highlighted: [HTML and SVG](html-svg.md) render, `.txt` is
[plain text](plain-text.md), and `.md` is [Markdown](markdown.md).

## Options

| Option | Effect |
|--------|--------|
| `#L42` | Scrolls to that line and highlights it |
| `#L10-L20` | Scrolls to that range and highlights it |
| `?raw=1` | The original file, as does the **raw** link |

See [Links to lines](../linking.md#links-to-lines) for all of the details.

## Limits and fallbacks

Files over 2 MiB show as plain text. A link to a line in one of these shows a numbered
window around that line, without colors.

Files approaching that limit can be slow to open if they have many thousands of lines,
because of how much page there is to lay out.

## Try it

- [Go](../../examples/source/go/catalog.go), and
  [line 5 of the same file](../../examples/source/go/catalog.go#L5)
- [Python](../../examples/source/catalog.py)
- [Rust](../../examples/source/catalog.rs)
- [Swift](../../examples/source/Catalog.swift)
- [Every sample](../../examples/source/)

[All file types](index.md) · [fb Help](../index.md)
