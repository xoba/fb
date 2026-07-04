package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestServesMarkdownThroughPandoc(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "note.md"), []byte("# Hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	pandoc := fakePandoc(t)
	server := fileServer{fsys: os.DirFS(root), root: root, pandoc: pandoc}

	req := httptest.NewRequest(http.MethodGet, "/note.md", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	res := rec.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", res.StatusCode, http.StatusOK, rec.Body.String())
	}
	if got := res.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Fatalf("Content-Type = %q, want text/html", got)
	}
	if !strings.Contains(rec.Body.String(), "<p>rendered markdown</p>") {
		t.Fatalf("body = %q, want rendered markdown", rec.Body.String())
	}
}

func TestServesNonMarkdownFilesDirectly(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "plain.txt"), []byte("plain text\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	server := fileServer{fsys: os.DirFS(root), root: root, pandoc: "unused"}

	req := httptest.NewRequest(http.MethodGet, "/plain.txt", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != "plain text\n" {
		t.Fatalf("body = %q, want plain text", got)
	}
}

func TestServesNonMarkdownIgnoresConditionalCache(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "plain.txt"), []byte("fresh text\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	server := fileServer{fsys: os.DirFS(root), root: root, pandoc: "unused"}

	req := httptest.NewRequest(http.MethodGet, "/plain.txt", nil)
	req.Header.Set("If-Modified-Since", time.Now().Add(24*time.Hour).UTC().Format(http.TimeFormat))
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != "fresh text\n" {
		t.Fatalf("body = %q, want fresh text", got)
	}
}

func TestServesDirectoryFromConfiguredRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "root-marker.txt"), []byte("root\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	server := fileServer{fsys: os.DirFS(root), root: root, pandoc: "unused"}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store, no-cache, must-revalidate, max-age=0" {
		t.Fatalf("Cache-Control = %q, want aggressive no-cache policy", got)
	}
	if got := rec.Header().Get("Pragma"); got != "no-cache" {
		t.Fatalf("Pragma = %q, want no-cache", got)
	}
	if got := rec.Header().Get("Expires"); got != "0" {
		t.Fatalf("Expires = %q, want 0", got)
	}
	if !strings.Contains(rec.Body.String(), "root-marker.txt") {
		t.Fatalf("directory listing = %q, want configured root contents", rec.Body.String())
	}
}

func TestTocQueryAddsTableOfContents(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "note.md"), []byte("# Hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	server := fileServer{fsys: os.DirFS(root), root: root, pandoc: fakePandoc(t)}

	for _, tc := range []struct {
		target  string
		wantTOC bool
	}{
		{target: "/note.md", wantTOC: false},
		{target: "/note.md?toc=1", wantTOC: true},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.target, nil)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, want %d; body: %s", tc.target, rec.Code, http.StatusOK, rec.Body.String())
		}
		if got := strings.Contains(rec.Body.String(), `id="TOC"`); got != tc.wantTOC {
			t.Fatalf("GET %s TOC present = %v, want %v", tc.target, got, tc.wantTOC)
		}
	}
}

func TestLocalCSSLinkedOutermostFirst(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".localmd.css", "sub/.localmd.css"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("body {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "note.md"), []byte("# Hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	server := fileServer{fsys: os.DirFS(root), root: root, pandoc: fakePandoc(t)}

	req := httptest.NewRequest(http.MethodGet, "/sub/note.md", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := rec.Body.String()
	rootCSS := strings.Index(body, `href="/.localmd.css"`)
	subCSS := strings.Index(body, `href="/sub/.localmd.css"`)
	if rootCSS < 0 || subCSS < 0 {
		t.Fatalf("body = %q, want both .localmd.css links", body)
	}
	if rootCSS > subCSS {
		t.Fatalf("root css at %d after sub css at %d, want outermost first", rootCSS, subCSS)
	}
}

func TestDirectoryListingGroupsAndRendersReadme(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "zdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"a.txt":     "plain\n",
		"notes.md":  "# Notes\n",
		"README.md": "# Readme\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	server := fileServer{fsys: os.DirFS(root), root: root, pandoc: fakePandoc(t)}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := rec.Body.String()

	dir := strings.Index(body, `href="zdir/"`)
	md := strings.Index(body, `href="notes.md"`)
	txt := strings.Index(body, `href="a.txt"`)
	if dir < 0 || md < 0 || txt < 0 {
		t.Fatalf("body = %q, want links to zdir/, notes.md, a.txt", body)
	}
	if !(dir < md && md < txt) {
		t.Fatalf("listing order dir=%d md=%d txt=%d, want directories, then markdown, then others", dir, md, txt)
	}
	if !strings.Contains(body, "<p>rendered readme</p>") {
		t.Fatalf("body = %q, want inline rendered README", body)
	}
}

func TestDirectoryWithoutTrailingSlashRedirects(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}

	server := fileServer{fsys: os.DirFS(root), root: root, pandoc: "unused"}

	req := httptest.NewRequest(http.MethodGet, "/sub", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMovedPermanently)
	}
	if got := rec.Header().Get("Location"); got != "/sub/" {
		t.Fatalf("Location = %q, want /sub/", got)
	}
}

func TestServesEmbeddedMathJax(t *testing.T) {
	server := fileServer{fsys: os.DirFS(t.TempDir()), root: "unused", pandoc: "unused"}

	req := httptest.NewRequest(http.MethodGet, "/_localmd/mathjax/tex-mml-chtml.js", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.Len() == 0 {
		t.Fatal("empty MathJax asset")
	}
	if got := rec.Header().Get("Cache-Control"); got != "max-age=86400" {
		t.Fatalf("Cache-Control = %q, want max-age=86400", got)
	}
}

func TestParseRootArg(t *testing.T) {
	root, err := parseRootArg([]string{"/"})
	if err != nil {
		t.Fatal(err)
	}
	if root != "/" {
		t.Fatalf("parseRootArg returned %q, want /", root)
	}

	for _, args := range [][]string{
		nil,
		{},
		{"/", "/tmp"},
		{"--root", "/"},
		{"-h"},
		{"--help"},
	} {
		if _, err := parseRootArg(args); err == nil {
			t.Fatalf("parseRootArg(%q) succeeded, want error", args)
		}
	}
}

func TestResolveRootSlash(t *testing.T) {
	root, err := resolveRoot("/")
	if err != nil {
		t.Fatal(err)
	}
	if root != string(os.PathSeparator) {
		t.Fatalf("resolveRoot(\"/\") = %q, want %q", root, string(os.PathSeparator))
	}
}

func fakePandoc(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "pandoc")
	script := `#!/bin/sh
set -eu

header=""
format=""
standalone=0
toc=0
while [ "$#" -gt 0 ]; do
  case "$1" in
    -f) shift; format="$1" ;;
    --include-in-header) shift; header="$1" ;;
    -s) standalone=1 ;;
    --toc) toc=1 ;;
  esac
  shift
done

case "$format" in
  *+gfm_auto_identifiers*) ;;
  *)
    echo "missing gfm auto identifiers" >&2
    exit 4
    ;;
esac

case "$format" in
  *+autolink_bare_uris*) ;;
  *)
    echo "missing autolink_bare_uris" >&2
    exit 4
    ;;
esac

if [ "$standalone" -eq 0 ]; then
  printf '<p>rendered readme</p>'
  exit 0
fi

if [ -z "$header" ]; then
  echo "missing --include-in-header" >&2
  exit 2
fi

if ! grep -q "color-scheme: light" "$header"; then
  echo "missing readability CSS" >&2
  exit 3
fi

printf '<!doctype html><html><head>'
cat "$header"
printf '</head><body>'
if [ "$toc" -eq 1 ]; then
  printf '<nav id="TOC"></nav>'
fi
printf '<p>rendered markdown</p></body></html>'
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}
