# Source code

fb shows source files as highlighted, GitHub-styled pages with line numbers you can click
and link to.

**Extensions and names:** every language [Chroma](https://github.com/alecthomas/chroma)
recognizes by filename, over 250 of them, from `.abap` and `.ada` to `.lean`, `.py`, `.rs`,
`.swift`, and `.zig`, plus exact names such as `Makefile`, `Dockerfile`, `CMakeLists.txt`, and
`.zshrc`. Case does not matter. See [Languages](#languages) for the full list.

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
- **`.m`** is Objective-C and **`.fs`** is F#. Chroma also knows MATLAB, Mathematica, and
  Forth by these extensions, and fb picks the meaning most common on a Mac.
- **Exact names win over extensions**, so `CMakeLists.txt` and `meson_options.txt` are
  highlighted even though other `.txt` files are plain text.

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

A file whose contents are not text, meaning not UTF-8 or containing NUL bytes, opens as the
original file even when its extension names a language: a compiled gettext `.mo` catalog is
not shown as Modelica source.

Files approaching that limit can be slow to open if they have many thousands of lines,
because of how much page there is to lay out.

## Try it

- [Go](../../examples/source/go/catalog.go), and
  [line 5 of the same file](../../examples/source/go/catalog.go#L5)
- [Python](../../examples/source/catalog.py)
- [Rust](../../examples/source/catalog.rs)
- [Swift](../../examples/source/Catalog.swift)
- [Lean](../../examples/source/languages/Catalog.lean)
- [Every sample](../../examples/source/), with the less common languages in
  [languages](../../examples/source/languages/)

## Languages

Each language Chroma reaches by filename, with the extensions and names that lead to it
and a sample to try. A pattern shared by several languages is listed under the one fb uses.

<!-- languages:begin (generated: FB_UPDATE_DOCS=1 go test -run TestSourceLanguageTable) -->

| Language | Extensions and names | Example |
|----------|----------------------|---------|
| ABAP | `.abap` | [catalog.abap](../../examples/source/languages/catalog.abap) |
| ABNF | `.abnf` | [catalog.abnf](../../examples/source/languages/catalog.abnf) |
| ActionScript | `.as` | [Catalog.as](../../examples/source/languages/Catalog.as) |
| Ada | `.ada` `.adb` `.ads` | [browse.ada](../../examples/source/browse.ada) |
| Agda | `.agda` | [catalog.agda](../../examples/source/languages/catalog.agda) |
| AL | `.al` `.dal` | [catalog.al](../../examples/source/languages/catalog.al) |
| Alloy | `.als` | [catalog.als](../../examples/source/languages/catalog.als) |
| AMPL | `.mod` `.run` `v.mod` | [catalog.mod](../../examples/source/languages/catalog.mod) |
| ApacheConf | `.htaccess` `apache.conf` `apache2.conf` | [apache.conf](../../examples/source/languages/apache.conf) |
| APL | `.apl` | [catalog.apl](../../examples/source/languages/catalog.apl) |
| AppleScript | `.applescript` | [catalog.applescript](../../examples/source/languages/catalog.applescript) |
| ArangoDB AQL | `.aql` | [catalog.aql](../../examples/source/languages/catalog.aql) |
| Arduino | `.ino` | [catalog.ino](../../examples/source/languages/catalog.ino) |
| Arturo | `.art` | [catalog.art](../../examples/source/languages/catalog.art) |
| Assembly | `.asm` | [catalog.asm](../../examples/source/languages/catalog.asm) |
| ATL | `.atl` | [catalog.atl](../../examples/source/languages/catalog.atl) |
| AutoHotkey | `.ahk` `.ahkl` | [catalog.ahk](../../examples/source/languages/catalog.ahk) |
| AutoIt | `.au3` | [catalog.au3](../../examples/source/languages/catalog.au3) |
| Awk | `.awk` | [count.awk](../../examples/source/count.awk) |
| Ballerina | `.bal` | [catalog.bal](../../examples/source/languages/catalog.bal) |
| Bash | `.bash` `.bash_*` `.bashrc` `.ebuild` `.eclass` `.env` `.env.*` `.exheres-0` `.exlib` `.ksh` `.sh` `.zsh` `.zshrc` `APKBUILD` `PKGBUILD` `bash_*` `bashrc` `zshrc` | [.bashrc](../../examples/source/.bashrc) |
| Bash Session | `.sh-session` | [catalog.sh-session](../../examples/source/languages/catalog.sh-session) |
| Batchfile | `.bat` `.cmd` | [browse.bat](../../examples/source/browse.bat) |
| Beef | `.bf` | [catalog.bf](../../examples/source/languages/catalog.bf) |
| BibTeX | `.bib` | [catalog.bib](../../examples/source/languages/catalog.bib) |
| Bicep | `.bicep` | [catalog.bicep](../../examples/source/languages/catalog.bicep) |
| BlitzBasic | `.bb` `.decls` | [catalog.bb](../../examples/source/languages/catalog.bb) |
| BNF | `.bnf` | [catalog.bnf](../../examples/source/languages/catalog.bnf) |
| BQN | `.bqn` | [catalog.bqn](../../examples/source/languages/catalog.bqn) |
| Brainfuck | `.b` | [catalog.b](../../examples/source/languages/catalog.b) |
| C | `.c` `.h` `.idc` `.x[bp]m` | [catalog.c](../../examples/source/catalog.c) |
| C# | `.cs` | [Catalog.cs](../../examples/source/Catalog.cs) |
| C++ | `.C` `.H` `.c++` `.cc` `.cp` `.cpp` `.cxx` `.h++` `.hh` `.hpp` `.hxx` `.tpp` | [catalog.cc](../../examples/source/catalog.cc) |
| C3 | `.c3` `.c3i` `.c3t` | [catalog.c3](../../examples/source/languages/catalog.c3) |
| Caddyfile | `Caddyfile*` | [Caddyfile](../../examples/source/languages/Caddyfile) |
| Cap'n Proto | `.capnp` | [catalog.capnp](../../examples/source/languages/catalog.capnp) |
| Cassandra CQL | `.cql` | [catalog.cql](../../examples/source/languages/catalog.cql) |
| Ceylon | `.ceylon` | [catalog.ceylon](../../examples/source/languages/catalog.ceylon) |
| CFEngine3 | `.cf` | [catalog.cf](../../examples/source/languages/catalog.cf) |
| ChaiScript | `.chai` | [catalog.chai](../../examples/source/languages/catalog.chai) |
| Chapel | `.chpl` | [catalog.chpl](../../examples/source/languages/catalog.chpl) |
| Cheetah | `.go.tmpl` `.spt` `.tmpl` | [catalog.tmpl](../../examples/source/languages/catalog.tmpl) |
| Clojure | `.clj` `.edn` | [catalog.clj](../../examples/source/catalog.clj) |
| CMake | `.cmake` `CMakeLists.txt` | [CMakeLists.txt](../../examples/source/CMakeLists.txt) |
| COBOL | `.cob` `.cpy` | [catalog.cob](../../examples/source/languages/catalog.cob) |
| CoffeeScript | `.coffee` | [catalog.coffee](../../examples/source/catalog.coffee) |
| Common Lisp | `.cl` `.lisp` | [catalog.lisp](../../examples/source/catalog.lisp) |
| Coq | `.v` | [catalog.v](../../examples/source/languages/catalog.v) |
| Core | `.core` | [catalog.core](../../examples/source/languages/catalog.core) |
| Crystal | `.cr` | [catalog.cr](../../examples/source/languages/catalog.cr) |
| CSS | `.css` | [catalog.css](../../examples/source/catalog.css) |
| CUE | `.cue` | [catalog.cue](../../examples/source/languages/catalog.cue) |
| Cython | `.pxd` `.pxi` `.pyx` | [catalog.pyx](../../examples/source/languages/catalog.pyx) |
| D | `.d` `.di` | [catalog.d](../../examples/source/languages/catalog.d) |
| Dart | `.dart` | [catalog.dart](../../examples/source/catalog.dart) |
| Dax | `.dax` | [catalog.dax](../../examples/source/languages/catalog.dax) |
| Desktop file | `.desktop` | [catalog.desktop](../../examples/source/languages/catalog.desktop) |
| Devicetree | `.dts` `.dtsi` | [catalog.dts](../../examples/source/languages/catalog.dts) |
| Diff | `.diff` `.patch` | [catalog.diff](../../examples/source/catalog.diff) |
| dns | `.zone` | [catalog.zone](../../examples/source/languages/catalog.zone) |
| Docker | `.Containerfile` `.Dockerfile` `.docker` `Containerfile` `Containerfile.*` `Dockerfile` `Dockerfile.*` | [Dockerfile](../../examples/source/Dockerfile) |
| DTD | `.dtd` | [catalog.dtd](../../examples/source/languages/catalog.dtd) |
| Dylan | `.dyl` `.dylan` `.intr` | [catalog.dylan](../../examples/source/languages/catalog.dylan) |
| EBNF | `.ebnf` | [catalog.ebnf](../../examples/source/languages/catalog.ebnf) |
| Elixir | `.eex` `.ex` `.exs` | [catalog.ex](../../examples/source/catalog.ex) |
| Elm | `.elm` | [Catalog.elm](../../examples/source/languages/Catalog.elm) |
| EmacsLisp | `.el` | [catalog.el](../../examples/source/catalog.el) |
| ERB | `.erb` `.html.erb` `.rhtml` `.xml.erb` | [catalog.erb](../../examples/source/catalog.erb) |
| Erlang | `.erl` `.es` `.escript` `.hrl` | [catalog.erl](../../examples/source/catalog.erl) |
| Factor | `.factor` | [catalog.factor](../../examples/source/languages/catalog.factor) |
| Fennel | `.fennel` | [catalog.fennel](../../examples/source/languages/catalog.fennel) |
| Fish | `.fish` `.load` | [browse.fish](../../examples/source/browse.fish) |
| Forth | `.frt` `.fth` | [catalog.fth](../../examples/source/languages/catalog.fth) |
| Fortran | `.f03` `.f90` `.f95` | [catalog.f90](../../examples/source/catalog.f90) |
| FortranFixed | `.f` | [catalog.f](../../examples/source/catalog.f) |
| FSharp | `.fs` `.fsi` | [catalog.fs](../../examples/source/languages/catalog.fs) |
| GAS | `.s` | [catalog.s](../../examples/source/catalog.s) |
| GDScript3 | `.gd` | [catalog.gd](../../examples/source/languages/catalog.gd) |
| Gemfile.lock | `.gemfile.lock` `Gemfile.lock` | [Gemfile.lock](../../examples/source/languages/Gemfile.lock) |
| Gemtext | `.gemini` `.gmi` `.gmni` | [catalog.gmi](../../examples/source/languages/catalog.gmi) |
| Genshi | `.kid` | [catalog.kid](../../examples/source/languages/catalog.kid) |
| Gettext | `.po` `.pot` | [catalog.po](../../examples/source/languages/catalog.po) |
| Gherkin | `.feature` | [catalog.feature](../../examples/source/catalog.feature) |
| Gleam | `.gleam` | [catalog.gleam](../../examples/source/languages/catalog.gleam) |
| GLSL | `.frag` `.geo` `.vert` | [catalog.frag](../../examples/source/languages/catalog.frag) |
| Gnuplot | `.plot` `.plt` | [catalog.plt](../../examples/source/languages/catalog.plt) |
| Go | `.go` | [catalog.go](../../examples/source/go/catalog.go) |
| Go Template | `.gotmpl` | [catalog.gotmpl](../../examples/source/languages/catalog.gotmpl) |
| GraphQL | `.graphql` `.graphqls` | [catalog.graphql](../../examples/source/catalog.graphql) |
| Groff | `.1p` `.3pm` `.[1-9]` `.man` | [catalog.1](../../examples/source/languages/catalog.1) |
| Groovy | `.gradle` `.groovy` | [catalog.gradle](../../examples/source/catalog.gradle) |
| Handlebars | `.handlebars` `.hbs` | [catalog.hbs](../../examples/source/languages/catalog.hbs) |
| Hare | `.ha` | [catalog.ha](../../examples/source/languages/catalog.ha) |
| Haskell | `.hs` | [catalog.hs](../../examples/source/catalog.hs) |
| Haxe | `.hx` `.hxsl` | [Catalog.hx](../../examples/source/languages/Catalog.hx) |
| HCL | `.hcl` | [catalog.hcl](../../examples/source/catalog.hcl) |
| HLB | `.hlb` | [catalog.hlb](../../examples/source/languages/catalog.hlb) |
| HLSL | `.cginc` `.fx` `.fxh` `.hlsl` `.hlsli` | [catalog.hlsl](../../examples/source/languages/catalog.hlsl) |
| HolyC | `.HH` `.hc` `.hc.z` | [catalog.hc](../../examples/source/languages/catalog.hc) |
| HTML | `.xhtml` `.xslt` | [catalog.xhtml](../../examples/source/languages/catalog.xhtml) |
| Hy | `.hy` | [catalog.hy](../../examples/source/languages/catalog.hy) |
| Idris | `.idr` | [catalog.idr](../../examples/source/languages/catalog.idr) |
| Igor | `.ipf` | [catalog.ipf](../../examples/source/languages/catalog.ipf) |
| INI | `.build` `.cfg` `.container` `.editorconfig` `.gitconfig` `.image` `.inf` `.ini` `.kube` `.pod` `.pylintrc` `.volume` `pylintrc` | [catalog.ini](../../examples/source/catalog.ini) |
| Io | `.io` | [catalog.io](../../examples/source/languages/catalog.io) |
| ISCdhcpd | `dhcpd.conf` | [dhcpd.conf](../../examples/source/languages/dhcpd.conf) |
| J | `.ijs` | [catalog.ijs](../../examples/source/languages/catalog.ijs) |
| Janet | `.janet` `.jdn` | [catalog.janet](../../examples/source/languages/catalog.janet) |
| Java | `.java` | [Catalog.java](../../examples/source/Catalog.java) |
| JavaScript | `.cjs` `.js` `.jsm` `.mjs` | [catalog.js](../../examples/source/catalog.js) |
| JSON | `.avsc` `.json5` `.jsonc` `.jsonl` `.luaurc` | [catalog.jsonc](../../examples/source/languages/catalog.jsonc) |
| JSONata | `.jsonata` | [catalog.jsonata](../../examples/source/languages/catalog.jsonata) |
| Jsonnet | `.jsonnet` `.libsonnet` | [catalog.jsonnet](../../examples/source/languages/catalog.jsonnet) |
| Julia | `.jl` | [catalog.jl](../../examples/source/catalog.jl) |
| Jungle | `.jungle` | [catalog.jungle](../../examples/source/languages/catalog.jungle) |
| Kakoune | `.kak` `kakrc` | [catalog.kak](../../examples/source/languages/catalog.kak) |
| KDL | `.kdl` | [catalog.kdl](../../examples/source/languages/catalog.kdl) |
| Kotlin | `.kt` `.kts` | [catalog.kt](../../examples/source/catalog.kt) |
| Lateralus | `.ltl` | [catalog.ltl](../../examples/source/languages/catalog.ltl) |
| Lean4 | `.lean` | [Catalog.lean](../../examples/source/languages/Catalog.lean) |
| LilyPond | `.ly` | [catalog.ly](../../examples/source/languages/catalog.ly) |
| LLVM | `.ll` | [catalog.ll](../../examples/source/languages/catalog.ll) |
| lox | `.lox` | [catalog.lox](../../examples/source/languages/catalog.lox) |
| Lua | `.lua` `.wlua` | [catalog.lua](../../examples/source/catalog.lua) |
| Luau | `.luau` | [catalog.luau](../../examples/source/languages/catalog.luau) |
| Makefile | `.justfile` `.mak` `.mk` `BSDmakefile` `GNUmakefile` `Justfile` `Makefile` `Makefile.*` `justfile` `makefile` | [Makefile](../../examples/source/Makefile) |
| Mako | `.mao` | [catalog.mao](../../examples/source/languages/catalog.mao) |
| markdown | `.markdown` `.mkd` | [catalog.markdown](../../examples/source/languages/catalog.markdown) |
| Markless | `.markless` `.mess` | [catalog.mess](../../examples/source/languages/catalog.mess) |
| Mason | `.mhtml` `.mi` `autohandler` `dhandler` | [catalog.mi](../../examples/source/languages/catalog.mi) |
| Mathematica | `.cdf` `.ma` `.mt` `.mx` `.nb` `.nbp` `.wl` | [catalog.wl](../../examples/source/languages/catalog.wl) |
| MCFunction | `.mcfunction` | [catalog.mcfunction](../../examples/source/languages/catalog.mcfunction) |
| Meson | `meson.build` `meson.options` `meson_options.txt` | [meson.build](../../examples/source/languages/meson.build) |
| Metal | `.metal` | [catalog.metal](../../examples/source/languages/catalog.metal) |
| microcad | `.mcad` `.ucad` `.µcad` | [catalog.mcad](../../examples/source/languages/catalog.mcad) |
| MiniZinc | `.dzn` `.fzn` `.mzn` | [catalog.mzn](../../examples/source/languages/catalog.mzn) |
| MLIR | `.mlir` | [catalog.mlir](../../examples/source/languages/catalog.mlir) |
| Modelica | `.mo` | [Catalog.mo](../../examples/source/languages/Catalog.mo) |
| Modula-2 | `.def` | [Catalog.def](../../examples/source/languages/Catalog.def) |
| Mojo | `.mojo` `.🔥` | [catalog.mojo](../../examples/source/languages/catalog.mojo) |
| MonkeyC | `.mc` | [catalog.mc](../../examples/source/languages/catalog.mc) |
| MoonBit | `.mbt` | [catalog.mbt](../../examples/source/languages/catalog.mbt) |
| MoonScript | `.moon` | [catalog.moon](../../examples/source/languages/catalog.moon) |
| Myghty | `.myt` `autodelegate` | [catalog.myt](../../examples/source/languages/catalog.myt) |
| MySQL | `.sql` | [catalog.sql](../../examples/source/catalog.sql) |
| NASM | `.nasm` | [catalog.nasm](../../examples/source/languages/catalog.nasm) |
| Natural | `.NS7` `.NSA` `.NSC` `.NSG` `.NSH` `.NSL` `.NSM` `.NSN` `.NSP` `.NSS` | [catalog.NSP](../../examples/source/languages/catalog.NSP) |
| Newspeak | `.ns2` | [catalog.ns2](../../examples/source/languages/catalog.ns2) |
| Nginx configuration file | `nginx.conf` | [nginx.conf](../../examples/source/languages/nginx.conf) |
| Nim | `.nim` `.nimrod` | [catalog.nim](../../examples/source/languages/catalog.nim) |
| Nix | `.nix` | [catalog.nix](../../examples/source/catalog.nix) |
| NSIS | `.nsh` `.nsi` | [catalog.nsi](../../examples/source/languages/catalog.nsi) |
| Nu | `.nu` | [catalog.nu](../../examples/source/languages/catalog.nu) |
| Objective-C | `.m` | [catalog.m](../../examples/source/languages/catalog.m) |
| ObjectPascal | `.dpk` `.dpr` `.lpk` `.lpr` `.pas` `.pp` | [catalog.pas](../../examples/source/languages/catalog.pas) |
| OCaml | `.ml` `.mli` `.mll` `.mly` | [catalog.ml](../../examples/source/languages/catalog.ml) |
| Odin | `.odin` | [catalog.odin](../../examples/source/languages/catalog.odin) |
| OnesEnterprise | `.epf` `.erf` | [catalog.epf](../../examples/source/languages/catalog.epf) |
| OpenEdge ABL | `.cls` `.i` `.p` `.w` | [catalog.p](../../examples/source/languages/catalog.p) |
| OpenSCAD | `.scad` | [catalog.scad](../../examples/source/languages/catalog.scad) |
| Org Mode | `.org` | [catalog.org](../../examples/source/languages/catalog.org) |
| PacmanConf | `pacman.conf` | [pacman.conf](../../examples/source/languages/pacman.conf) |
| Perl | `.pl` `.pm` `.t` | [catalog.pl](../../examples/source/catalog.pl) |
| PHP | `.inc` `.php` `.php[345]` | [catalog.php](../../examples/source/catalog.php) |
| PHTML | `.phtml` | [catalog.phtml](../../examples/source/languages/catalog.phtml) |
| Pig | `.pig` | [catalog.pig](../../examples/source/languages/catalog.pig) |
| PkgConfig | `.pc` | [catalog.pc](../../examples/source/languages/catalog.pc) |
| Plutus Core | `.plc` | [catalog.plc](../../examples/source/languages/catalog.plc) |
| Pony | `.pony` | [catalog.pony](../../examples/source/languages/catalog.pony) |
| PostScript | `.eps` `.epsf` `.epsi` `.pfa` `.ps` `.t42` | [catalog.ps](../../examples/source/languages/catalog.ps) |
| POVRay | `.pov` | [catalog.pov](../../examples/source/languages/catalog.pov) |
| PowerQuery | `.pq` | [catalog.pq](../../examples/source/languages/catalog.pq) |
| PowerShell | `.ps1` `.psd1` `.psm1` | [catalog.ps1](../../examples/source/catalog.ps1) |
| Prolog | `.ecl` `.pro` `.prolog` | [catalog.pro](../../examples/source/languages/catalog.pro) |
| Promela | `.pml` `.pr` `.prm` `.prom` `.promela` | [catalog.pml](../../examples/source/languages/catalog.pml) |
| PromQL | `.promql` | [catalog.promql](../../examples/source/languages/catalog.promql) |
| properties | `.mf` `.properties` | [MANIFEST.mf](../../examples/source/MANIFEST.mf) |
| Protocol Buffer | `.proto` | [catalog.proto](../../examples/source/catalog.proto) |
| Protocol Buffer Text Format | `.pbtxt` `.textpb` `.textproto` `.txtpb` | [catalog.textproto](../../examples/source/languages/catalog.textproto) |
| PRQL | `.prql` | [catalog.prql](../../examples/source/languages/catalog.prql) |
| PSL | `.BATCH` `.PROC` `.TRIG` `.psl` | [catalog.psl](../../examples/source/languages/catalog.psl) |
| Python | `.bzl` `.jy` `.py` `.pyi` `.pyw` `.sage` `.sc` `.star` `.tac` `BUCK` `BUILD` `BUILD.bazel` `MODULE.bazel` `REPO.bazel` `SConscript` `SConstruct` `WORKSPACE` `WORKSPACE.bazel` `WORKSPACE.bzlmod` | [catalog.py](../../examples/source/catalog.py) |
| QBasic | `.bas` | [catalog.bas](../../examples/source/languages/catalog.bas) |
| QML | `.qbs` `.qml` | [catalog.qml](../../examples/source/languages/catalog.qml) |
| R | `.Renviron` `.Rhistory` `.Rprofile` `.r` | [catalog.r](../../examples/source/catalog.r) |
| Racket | `.rkt` `.rktd` `.rktl` | [catalog.rkt](../../examples/source/languages/catalog.rkt) |
| Raku | `.6pl` `.6pm` `.nqp` `.p6` `.p6l` `.p6m` `.pl6` `.pm6` `.raku` `.rakudoc` `.rakumod` `.rakutest` | [catalog.raku](../../examples/source/languages/catalog.raku) |
| react | `.jsx` `.react` | [Catalog.jsx](../../examples/source/Catalog.jsx) |
| ReasonML | `.re` `.rei` | [catalog.re](../../examples/source/languages/catalog.re) |
| reg | `.reg` | [catalog.reg](../../examples/source/languages/catalog.reg) |
| Rego | `.rego` | [catalog.rego](../../examples/source/languages/catalog.rego) |
| reStructuredText | `.rest` | [catalog.rest](../../examples/source/languages/catalog.rest) |
| Rexx | `.arexx` `.rex` `.rexx` `.rx` | [catalog.rexx](../../examples/source/languages/catalog.rexx) |
| Ring | `.rform` `.rh` `.ring` | [catalog.ring](../../examples/source/languages/catalog.ring) |
| RPGLE | `.rpgle` `.sqlrpgle` | [catalog.rpgle](../../examples/source/languages/catalog.rpgle) |
| RPMSpec | `.spec` | [catalog.spec](../../examples/source/languages/catalog.spec) |
| Ruby | `.duby` `.gemfile` `.gemspec` `.json.jbuilder` `.pryrc` `.rake` `.rb` `.rbw` `.rbx` `Appraisals` `Gemfile` `Rakefile` `Vagrantfile` | [catalog.rb](../../examples/source/catalog.rb) |
| Rust | `.rs` `.rs.in` | [catalog.rs](../../examples/source/catalog.rs) |
| SAS | `.sas` | [catalog.sas](../../examples/source/languages/catalog.sas) |
| Sass | `.sass` | [catalog.sass](../../examples/source/languages/catalog.sass) |
| Scala | `.scala` | [Catalog.scala](../../examples/source/Catalog.scala) |
| scdoc | `.scd` `.scdoc` | [catalog.scd](../../examples/source/languages/catalog.scd) |
| Scheme | `.scm` `.ss` | [catalog.scm](../../examples/source/languages/catalog.scm) |
| Scilab | `.sce` `.sci` `.tst` | [catalog.sce](../../examples/source/languages/catalog.sce) |
| SCSS | `.scss` | [catalog.scss](../../examples/source/catalog.scss) |
| Sed | `.[gs]sed` `.sed` | [catalog.sed](../../examples/source/languages/catalog.sed) |
| Sieve | `.sieve` `.siv` | [catalog.sieve](../../examples/source/languages/catalog.sieve) |
| Smali | `.smali` | [catalog.smali](../../examples/source/languages/catalog.smali) |
| Smalltalk | `.st` | [catalog.st](../../examples/source/languages/catalog.st) |
| Smarty | `.tpl` | [catalog.tpl](../../examples/source/languages/catalog.tpl) |
| SNBT | `.snbt` | [catalog.snbt](../../examples/source/languages/catalog.snbt) |
| Snobol | `.snobol` | [catalog.snobol](../../examples/source/languages/catalog.snobol) |
| Solidity | `.sol` | [catalog.sol](../../examples/source/languages/catalog.sol) |
| SourcePawn | `.sp` | [catalog.sp](../../examples/source/languages/catalog.sp) |
| Spade | `.spade` | [catalog.spade](../../examples/source/languages/catalog.spade) |
| SPARQL | `.rq` `.sparql` | [catalog.rq](../../examples/source/languages/catalog.rq) |
| SquidConf | `squid.conf` | [squid.conf](../../examples/source/languages/squid.conf) |
| Standard ML | `.fun` `.sig` `.sml` | [catalog.sml](../../examples/source/languages/catalog.sml) |
| stas | `.stas` | [catalog.stas](../../examples/source/languages/catalog.stas) |
| Stylus | `.styl` | [catalog.styl](../../examples/source/languages/catalog.styl) |
| Svelte | `.svelte` | [Catalog.svelte](../../examples/source/Catalog.svelte) |
| Swift | `.swift` | [Catalog.swift](../../examples/source/Catalog.swift) |
| SYSTEMD | `.automount` `.device` `.dnssd` `.link` `.mount` `.netdev` `.network` `.path` `.scope` `.service` `.slice` `.socket` `.swap` `.target` `.timer` | [catalog.service](../../examples/source/languages/catalog.service) |
| systemverilog | `.sv` `.svh` | [catalog.sv](../../examples/source/languages/catalog.sv) |
| TableGen | `.td` | [catalog.td](../../examples/source/languages/catalog.td) |
| Tal | `.tal` | [catalog.tal](../../examples/source/languages/catalog.tal) |
| TASM | `.tasm` | [catalog.tasm](../../examples/source/languages/catalog.tasm) |
| Tcl | `.rvt` `.tcl` | [catalog.tcl](../../examples/source/languages/catalog.tcl) |
| Tcsh | `.csh` `.tcsh` | [catalog.csh](../../examples/source/languages/catalog.csh) |
| Templ | `.templ` | [catalog.templ](../../examples/source/languages/catalog.templ) |
| Termcap | `termcap` `termcap.src` | [termcap](../../examples/source/languages/termcap) |
| Terminfo | `terminfo` `terminfo.src` | [terminfo](../../examples/source/languages/terminfo) |
| Terraform | `.tf` | [catalog.tf](../../examples/source/catalog.tf) |
| TeX | `.aux` `.tex` `.toc` | [catalog.tex](../../examples/source/catalog.tex) |
| Thrift | `.thrift` | [catalog.thrift](../../examples/source/languages/catalog.thrift) |
| TOML | `.toml` `Pipfile` `poetry.lock` `uv.lock` | [catalog.toml](../../examples/source/catalog.toml) |
| TradingView | `.tv` | [catalog.tv](../../examples/source/languages/catalog.tv) |
| Turing | `.tu` `.turing` | [catalog.tu](../../examples/source/languages/catalog.tu) |
| Turtle | `.ttl` | [catalog.ttl](../../examples/source/languages/catalog.ttl) |
| Twig | `.twig` | [catalog.twig](../../examples/source/languages/catalog.twig) |
| TypeScript | `.cts` `.mts` `.ts` `.tsx` | [Catalog.tsx](../../examples/source/Catalog.tsx) |
| ucode | `.uc` | [catalog.uc](../../examples/source/languages/catalog.uc) |
| V | `.vv` | [catalog.vv](../../examples/source/languages/catalog.vv) |
| V shell | `.vsh` | [catalog.vsh](../../examples/source/languages/catalog.vsh) |
| Vala | `.vala` `.vapi` | [catalog.vala](../../examples/source/languages/catalog.vala) |
| VB.net | `.vb` | [catalog.vb](../../examples/source/languages/catalog.vb) |
| VHDL | `.vhd` `.vhdl` | [catalog.vhd](../../examples/source/languages/catalog.vhd) |
| VHS | `.tape` | [catalog.tape](../../examples/source/languages/catalog.tape) |
| VimL | `.exrc` `.gvimrc` `.vim` `.vimrc` `_exrc` `_gvimrc` `_vimrc` `gvimrc` `vimrc` | [catalog.vim](../../examples/source/languages/catalog.vim) |
| vue | `.vue` | [Catalog.vue](../../examples/source/Catalog.vue) |
| WDTE | `.wdte` | [catalog.wdte](../../examples/source/languages/catalog.wdte) |
| WebAssembly Text Format | `.wast` `.wat` | [catalog.wat](../../examples/source/languages/catalog.wat) |
| WebGPU Shading Language | `.wgsl` | [catalog.wgsl](../../examples/source/languages/catalog.wgsl) |
| WebVTT | `.vtt` | [catalog.vtt](../../examples/source/languages/catalog.vtt) |
| Whiley | `.whiley` | [catalog.whiley](../../examples/source/languages/catalog.whiley) |
| XML | `.csproj` `.fsproj` `.qrc` `.rss` `.vcxproj` `.wsdl` `.wsf` `.xml` `.xsd` `.xsl` | [catalog.xml](../../examples/source/catalog.xml) |
| Xorg | `xorg.conf` | [xorg.conf](../../examples/source/languages/xorg.conf) |
| YAML | `.yaml` `.yml` | [catalog.yaml](../../examples/source/catalog.yaml) |
| YAML+Jinja | `.sls` | [catalog.sls](../../examples/source/languages/catalog.sls) |
| YANG | `.yang` | [catalog.yang](../../examples/source/languages/catalog.yang) |
| Z80 Assembly | `.z80` | [catalog.z80](../../examples/source/languages/catalog.z80) |
| Zed | `.zed` | [catalog.zed](../../examples/source/languages/catalog.zed) |
| Zig | `.zig` `.zon` | [catalog.zig](../../examples/source/catalog.zig) |

<!-- languages:end -->

[All file types](index.md) · [fb Help](../index.md)
