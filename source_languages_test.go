package main

import (
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// The source view covers every language Chroma reaches by filename. These tests
// keep that promise checked from both ends: every pattern that reaches a lexer
// is served highlighted, every language has an example that lexes cleanly, and
// the language table in the documentation is generated from the same data.

const (
	sourceExamplesDir = "examples/source"
	sourceDocsPage    = "docs/file-types/source-code.md"
	languagesBegin    = "<!-- languages:begin (generated: FB_UPDATE_DOCS=1 go test -run TestSourceLanguageTable) -->"
	languagesEnd      = "<!-- languages:end -->"
)

// sourceGlobs lists every filename pattern that could reach the source view:
// all of Chroma's, plus the extensions fb assigns a lexer itself.
func sourceGlobs() []string {
	var globs []string
	for _, lexer := range lexers.GlobalLexerRegistry.Lexers {
		globs = append(globs, lexer.Config().Filenames...)
	}
	for ext := range forcedLexers {
		globs = append(globs, "*"+ext)
	}
	return append(globs, "*.asm")
}

// globSample is a filename that glob matches.
func globSample(glob string) string {
	sample := strings.NewReplacer("[1-9]", "1", "[gs]", "g", "[345]", "5", "[bp]", "b").Replace(glob)
	if strings.HasPrefix(sample, "*") {
		sample = "sample" + sample[1:]
	}
	if strings.HasSuffix(sample, "*") {
		sample = strings.TrimSuffix(sample, "*") + "local"
	}
	return sample
}

// globLabel is how the documentation names a glob: extensions without the
// leading star, and other patterns as Chroma writes them.
func globLabel(glob string) string {
	if strings.HasPrefix(glob, "*.") {
		return glob[1:]
	}
	return glob
}

// sourceLanguages maps each language the source view reaches by filename to
// the labels of the patterns that reach it. A pattern shared by several
// lexers belongs to the one fb actually picks. fb ignores case, so .ASM and
// .asm are one entry, but Chroma's case-sensitive pairs that lead to
// different languages (.C is C++, .c is C) stay apart.
func sourceLanguages() map[string][]string {
	labels := map[string]map[string]bool{}
	for _, glob := range sourceGlobs() {
		sample := globSample(glob)
		if !highlightable(sample) {
			continue
		}
		lang := sourceLexer(sample).Config().Name
		label := globLabel(glob)
		if strings.HasPrefix(glob, "*.") {
			if lower := sourceLexer(strings.ToLower(sample)); lower != nil && lower.Config().Name == lang {
				label = strings.ToLower(label)
			}
		}
		if labels[lang] == nil {
			labels[lang] = map[string]bool{}
		}
		labels[lang][label] = true
	}
	languages := map[string][]string{}
	for lang, set := range labels {
		languages[lang] = sortedKeys(set)
	}
	return languages
}

// sortedLanguageNames orders languages the way a reader scans a table.
func sortedLanguageNames(languages map[string][]string) []string {
	names := sortedKeys(languages)
	sort.SliceStable(names, func(i, j int) bool {
		return strings.ToLower(names[i]) < strings.ToLower(names[j])
	})
	return names
}

// sourceExamples maps each language to the example files under
// examples/source that the source view shows with its lexer.
func sourceExamples(t *testing.T) map[string][]string {
	t.Helper()
	examples := map[string][]string{}
	err := filepath.WalkDir(sourceExamplesDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !highlightable(p) {
			return err
		}
		lang := sourceLexer(p).Config().Name
		examples[lang] = append(examples[lang], filepath.ToSlash(p))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for lang := range examples {
		sort.Strings(examples[lang])
	}
	return examples
}

func TestEveryLanguageHasAnExample(t *testing.T) {
	examples := sourceExamples(t)
	for _, lang := range sortedLanguageNames(sourceLanguages()) {
		if len(examples[lang]) == 0 {
			t.Errorf("no example in %s/ is shown as %s; add one", sourceExamplesDir, lang)
		}
	}
}

// Each example is served as a highlighted page and lexes without error tokens,
// so a sample that a lexer only half understands, or one that lands on the
// wrong lexer, fails here instead of looking broken in the app.
func TestSourceExamplesHighlightCleanly(t *testing.T) {
	server := fileServer{fsys: os.DirFS(sourceExamplesDir), pandoc: "unused"}
	for _, files := range sourceExamples(t) {
		for _, file := range files {
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if !isNumberableText(data) {
				t.Errorf("%s is not UTF-8 text without NUL bytes", file)
				continue
			}
			lexer := sourceLexer(file)
			errs, colored := lexReport(lexer, string(data))
			if gap, ok := knownLexerGaps[file]; ok {
				if len(errs) == 0 {
					t.Errorf("%s lexes cleanly now; remove it from knownLexerGaps (%s)", file, gap)
				}
			} else if len(errs) > 0 {
				t.Errorf("%s: %s reports %d error tokens, first at %s", file, lexer.Config().Name, len(errs), errs[0])
			}
			if colored == 0 {
				t.Errorf("%s: %s colors nothing in it; pick a sample that shows the language", file, lexer.Config().Name)
			}

			rel := strings.TrimPrefix(file, sourceExamplesDir+"/")
			req := httptest.NewRequest(http.MethodGet, "/"+rel, nil)
			req.Header.Set("Sec-Fetch-Dest", "document")
			rec := httptest.NewRecorder()
			server.ServeHTTP(rec, req)
			if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
				t.Errorf("%s Content-Type = %q, want the highlighted page", file, got)
			}
		}
	}
}

// knownLexerGaps lists examples of valid code that a Chroma lexer does not
// fully understand. fb shows error tokens as plain text, so these only lose
// some color, but each is named so a Chroma upgrade that fixes it is noticed.
var knownLexerGaps = map[string]string{
	"examples/source/Catalog.jsx": "the react lexer rejects hyphenated attributes such as aria-label",
	"examples/source/Catalog.vue": "the vue lexer rejects hyphenated attributes such as aria-label",
}

// lexReport describes where lexer produced error tokens in src, ignoring
// whitespace (several lexers, such as CMake's and Docker's, reject line ends
// they do not expect, which shows as nothing), and counts the tokens it
// colored.
func lexReport(lexer chroma.Lexer, src string) (errs []string, colored int) {
	iterator, err := chroma.Coalesce(lexer).Tokenise(nil, src)
	if err != nil {
		return []string{err.Error()}, 0
	}
	line := 1
	for _, token := range iterator.Tokens() {
		switch {
		case token.Type == chroma.Error && strings.TrimSpace(token.Value) != "":
			errs = append(errs, fmt.Sprintf("line %d (%q)", line, token.Value))
		case token.Type != chroma.Error && token.Type != chroma.Text && token.Type.Category() != chroma.Text &&
			strings.TrimSpace(token.Value) != "":
			colored++
		}
		line += strings.Count(token.Value, "\n")
	}
	return errs, colored
}

// Every pattern in the language table opens in the source view, end to end.
func TestEverySourcePatternIsHighlighted(t *testing.T) {
	root := t.TempDir()
	var names []string
	for _, glob := range sourceGlobs() {
		sample := globSample(glob)
		if !highlightable(sample) {
			continue
		}
		if err := os.WriteFile(filepath.Join(root, sample), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		names = append(names, sample)
	}
	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}
	for _, name := range names {
		req := httptest.NewRequest(http.MethodGet, "/"+name, nil)
		req.Header.Set("Sec-Fetch-Dest", "document")
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
			t.Errorf("%s Content-Type = %q, want the highlighted page", name, got)
		}
	}
}

// Types with their own viewer keep it even though Chroma has a lexer for them,
// while Chroma's exact filenames beat an extension owned elsewhere.
func TestChromaDefersToOtherViewers(t *testing.T) {
	for _, name := range []string{
		"notes.md", "notes.MD", "guide.rst", "table.csv", "table.tsv", "data.json", "paper.typ",
		"page.html", "page.htm", "logo.svg", "notes.txt", "Info.plist", "go.mod", "cal.ics",
		"mail.eml", "book.epub", "doc.pdf", "lib.jar", "photo.png", "README.md~",
	} {
		if highlightable(name) {
			t.Errorf("highlightable(%q) = true, want its own viewer", name)
		}
	}
	for _, name := range []string{
		"CMakeLists.txt", "meson_options.txt", "Dockerfile", "Makefile", ".zshrc",
		"Proof.lean", "MAIN.PY", "app.C", "view.m", "Program.fs", "notes.markdown", "page.xhtml",
		"main.py~", "config.h.in", "MANIFEST.MF",
	} {
		if !highlightable(name) {
			t.Errorf("highlightable(%q) = false, want the source view", name)
		}
	}
	for name, want := range map[string]string{
		"Proof.lean": "Lean4", "MAIN.PY": "Python", "app.C": "C++", "app.c": "C",
		"view.m": "Objective-C", "Program.fs": "FSharp", "boot.s": "GAS", "boot.asm": "Assembly",
		"MANIFEST.MF": "properties",
	} {
		if got := sourceLexer(name).Config().Name; got != want {
			t.Errorf("sourceLexer(%q) = %s, want %s", name, got, want)
		}
	}
}

// Extensions are not proof of text: a compiled gettext catalog shares .mo
// with Modelica source, and must be served as the original file.
func TestBinaryFileWithSourceExtensionServedRaw(t *testing.T) {
	root := t.TempDir()
	binary := []byte("\xde\x12\x04\x95\x00\x00\x00\x00\x02\x00\x00\x00")
	if err := os.WriteFile(filepath.Join(root, "messages.mo"), binary, 0o644); err != nil {
		t.Fatal(err)
	}
	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}
	req := httptest.NewRequest(http.MethodGet, "/messages.mo", nil)
	req.Header.Set("Sec-Fetch-Dest", "document")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if got := rec.Header().Get("Content-Type"); strings.HasPrefix(got, "text/html") {
		t.Fatalf("binary .mo Content-Type = %q, want the original file", got)
	}
	if rec.Body.String() != string(binary) {
		t.Fatalf("binary .mo body = %q, want the original bytes", rec.Body.String())
	}
}

// languageTable renders the generated section of the source code page.
func languageTable(t *testing.T) string {
	t.Helper()
	languages := sourceLanguages()
	examples := sourceExamples(t)
	var b strings.Builder
	b.WriteString(languagesBegin + "\n\n")
	b.WriteString("| Language | Extensions and names | Example |\n")
	b.WriteString("|----------|----------------------|---------|\n")
	for _, lang := range sortedLanguageNames(languages) {
		var labels []string
		for _, label := range languages[lang] {
			labels = append(labels, "`"+label+"`")
		}
		example := ""
		if files := examples[lang]; len(files) > 0 {
			example = fmt.Sprintf("[%s](../../%s)", path.Base(files[0]), files[0])
		}
		fmt.Fprintf(&b, "| %s | %s | %s |\n", lang, strings.Join(labels, " "), example)
	}
	b.WriteString("\n" + languagesEnd)
	return b.String()
}

func TestSourceLanguageTable(t *testing.T) {
	data, err := os.ReadFile(sourceDocsPage)
	if err != nil {
		t.Fatal(err)
	}
	page := string(data)
	begin, end := strings.Index(page, languagesBegin), strings.Index(page, languagesEnd)
	if begin < 0 || end < begin {
		t.Fatalf("%s has no generated language table between %q and %q", sourceDocsPage, languagesBegin, languagesEnd)
	}
	want := page[:begin] + languageTable(t) + page[end+len(languagesEnd):]
	if want == page {
		return
	}
	if os.Getenv("FB_UPDATE_DOCS") == "1" {
		if err := os.WriteFile(sourceDocsPage, []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	t.Errorf("%s's language table is stale; regenerate it with FB_UPDATE_DOCS=1 go test -run TestSourceLanguageTable", sourceDocsPage)
}

// sourceDocLabels lists every label the language table documents, for the
// docs coverage test.
func sourceDocLabels() []string {
	var labels []string
	for _, set := range sourceLanguages() {
		labels = append(labels, set...)
	}
	sort.Strings(labels)
	return labels
}
