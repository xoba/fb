package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
	"golang.org/x/image/tiff"
)

func TestServesMarkdownThroughPandoc(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "note.md"), []byte("# Hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	pandoc := fakePandoc(t)
	server := fileServer{fsys: os.DirFS(root), pandoc: pandoc}

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

	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}

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

func TestServesGoSourceHighlighted(t *testing.T) {
	root := t.TempDir()
	src := "package main\n\nfunc main() {}\n"
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "prog.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}

	req := httptest.NewRequest(http.MethodGet, "/sub/prog.go", nil)
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Fatalf("Content-Type = %q, want text/html", got)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `id="L1"`) {
		t.Fatalf("body = %q, want linkable line numbers", body)
	}
	if !strings.Contains(body, "package") || !strings.Contains(body, "<span style=") {
		t.Fatalf("body = %q, want highlighted source tokens", body)
	}
	if !strings.Contains(body, `href="prog.go?raw=1"`) {
		t.Fatalf("body = %q, want raw link", body)
	}
}

func TestServesTypstHighlighted(t *testing.T) {
	root := t.TempDir()
	src := "#set page(width: 10cm)\n= Heading\nSome *bold* text.\n"
	if err := os.WriteFile(filepath.Join(root, "doc.typ"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}

	req := httptest.NewRequest(http.MethodGet, "/doc.typ", nil)
	req.Header.Set("Sec-Fetch-Dest", "document")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Fatalf("Content-Type = %q, want highlighted text/html", got)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Heading") || !strings.Contains(body, "<span style=") {
		t.Fatalf("body = %q, want highlighted typst tokens", body)
	}
}

func TestTypstReformatted(t *testing.T) {
	if _, err := exec.LookPath("typstyle"); err != nil {
		t.Skip("typstyle not available")
	}

	root := t.TempDir()
	long := "#let xs = (111111111, 222222222, 333333333, 444444444, 555555555, 666666666, 777777777, 888888888)\n"
	if err := os.WriteFile(filepath.Join(root, "long.typ"), []byte(long), 0o644); err != nil {
		t.Fatal(err)
	}

	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}

	req := httptest.NewRequest(http.MethodGet, "/long.typ", nil)
	req.Header.Set("Sec-Fetch-Dest", "document")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	// typstyle wraps the over-long one-line array onto ten lines.
	if body := rec.Body.String(); !strings.Contains(body, `id="L10"`) {
		t.Fatalf("body = %q, want ten reformatted lines", body)
	}
}

func TestExtensionlessNamesHighlighted(t *testing.T) {
	root := t.TempDir()
	src := "all:\n\tgo build\n"
	if err := os.WriteFile(filepath.Join(root, "Makefile"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}

	nav := httptest.NewRequest(http.MethodGet, "/Makefile", nil)
	nav.Header.Set("Sec-Fetch-Dest", "document")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, nav)
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Fatalf("navigation Content-Type = %q, want highlighted text/html", got)
	}

	plain := httptest.NewRequest(http.MethodGet, "/Makefile", nil)
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, plain)
	if got := rec.Body.String(); got != src {
		t.Fatalf("non-browser body = %q, want verbatim Makefile", got)
	}
}

func TestSourceServedVerbatimToNonBrowserClients(t *testing.T) {
	root := t.TempDir()
	src := "package main\n"
	if err := os.WriteFile(filepath.Join(root, "prog.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}

	req := httptest.NewRequest(http.MethodGet, "/prog.go", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != src {
		t.Fatalf("body = %q, want verbatim source for a request without browser headers", got)
	}
}

func TestCSSHighlightedOnlyForNavigations(t *testing.T) {
	root := t.TempDir()
	src := "body { color: red; }\n"
	if err := os.WriteFile(filepath.Join(root, "style.css"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}

	nav := httptest.NewRequest(http.MethodGet, "/style.css", nil)
	nav.Header.Set("Sec-Fetch-Dest", "document")
	nav.Header.Set("Accept", "text/html,application/xhtml+xml")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, nav)
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Fatalf("navigation Content-Type = %q, want highlighted text/html", got)
	}

	link := httptest.NewRequest(http.MethodGet, "/style.css", nil)
	link.Header.Set("Sec-Fetch-Dest", "style")
	link.Header.Set("Accept", "text/css,*/*;q=0.1")
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, link)
	if got := rec.Body.String(); got != src {
		t.Fatalf("stylesheet fetch body = %q, want verbatim css", got)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/css") {
		t.Fatalf("stylesheet fetch Content-Type = %q, want text/css", got)
	}
}

func TestIndexHTMLServedInPlace(t *testing.T) {
	root := t.TempDir()
	content := "<html><body>hello</body></html>\n"
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}

	req := httptest.NewRequest(http.MethodGet, "/index.html", nil)
	req.Header.Set("Sec-Fetch-Dest", "document")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (no redirect back to the directory)", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != content {
		t.Fatalf("body = %q, want index.html contents", got)
	}
}

func TestRawQueryServesSourceVerbatim(t *testing.T) {
	root := t.TempDir()
	src := "package main\n"
	if err := os.WriteFile(filepath.Join(root, "prog.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}

	req := httptest.NewRequest(http.MethodGet, "/prog.go?raw=1", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != src {
		t.Fatalf("body = %q, want verbatim source", got)
	}
}

func TestServesNonMarkdownIgnoresConditionalCache(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "plain.txt"), []byte("fresh text\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}

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

	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}

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

	server := fileServer{fsys: os.DirFS(root), pandoc: fakePandoc(t)}

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
	for _, name := range []string{".fb.css", "sub/.fb.css"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("body {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "note.md"), []byte("# Hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	server := fileServer{fsys: os.DirFS(root), pandoc: fakePandoc(t)}

	req := httptest.NewRequest(http.MethodGet, "/sub/note.md", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := rec.Body.String()
	rootCSS := strings.Index(body, `href="/.fb.css"`)
	subCSS := strings.Index(body, `href="/sub/.fb.css"`)
	if rootCSS < 0 || subCSS < 0 {
		t.Fatalf("body = %q, want both .fb.css links", body)
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
		".hidden":   "shh\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	server := fileServer{fsys: os.DirFS(root), pandoc: fakePandoc(t)}

	req := httptest.NewRequest(http.MethodGet, "/?sort=name", nil)
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
	// a.txt (6) + notes.md (8) + README.md (9) = 23 bytes; the dot-file is
	// excluded from both counts and the byte total.
	if want := `id="listsum">1 dir · 3 files · 23 B</p>`; !strings.Contains(body, want) {
		t.Fatalf("body = %q, want summary %q", body, want)
	}
}

func TestDirectoryRendersReadmeTxtFallback(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "readme.txt"), []byte("plain notes & things\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "readme.txt") {
		t.Fatalf("body = %q, want readme.txt section title", body)
	}
	if !strings.Contains(body, "plain notes &amp; things") {
		t.Fatalf("body = %q, want escaped readme.txt contents inline", body)
	}
}

func TestDirectoryShowsBlurb(t *testing.T) {
	for _, tc := range []struct {
		name     string
		content  []byte
		wantSeen bool
	}{
		{name: "small plain text", content: []byte("summary of this directory\n"), wantSeen: true},
		{name: "too large", content: []byte(strings.Repeat("x", 600)), wantSeen: false},
		{name: "binary", content: []byte{'h', 'i', 0x00, 0x01}, wantSeen: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "blurb.txt"), tc.content, 0o644); err != nil {
				t.Fatal(err)
			}

			server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			server.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			body := rec.Body.String()
			if got := strings.Contains(body, `<p class="blurb">`); got != tc.wantSeen {
				t.Fatalf("blurb shown = %v, want %v; body: %q", got, tc.wantSeen, body)
			}
			if tc.wantSeen && !strings.Contains(body, "summary of this directory") {
				t.Fatalf("body = %q, want blurb contents", body)
			}
			if !strings.Contains(body, `href="blurb.txt"`) {
				t.Fatalf("body = %q, want blurb.txt still listed as a file", body)
			}
		})
	}
}

func TestListingShowsChildDirectoryBlurbs(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "proj"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "proj", "blurb.txt"), []byte("a neat project\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), `<td class="blurb" title="a neat project">a neat project</td>`) {
		t.Fatalf("body = %q, want child directory blurb in its listing row", rec.Body.String())
	}
}

func TestDotEntriesCollapsed(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".hidden.txt", "visible.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	body := rec.Body.String()
	details := strings.Index(body, "<details")
	visible := strings.Index(body, `href="visible.txt"`)
	hidden := strings.Index(body, `href=".hidden.txt"`)
	gitDir := strings.Index(body, `href=".git/"`)
	if details < 0 || visible < 0 || hidden < 0 || gitDir < 0 {
		t.Fatalf("body = %q, want details section plus all three entries", body)
	}
	if visible > details {
		t.Fatalf("visible.txt at %d after <details> at %d, want it in the main listing", visible, details)
	}
	if hidden < details || gitDir < details {
		t.Fatalf("dot entries at %d,%d before <details> at %d, want them collapsed", hidden, gitDir, details)
	}
	if !strings.Contains(body, "2 dot-files") {
		t.Fatalf("body = %q, want dot-file count in summary", body)
	}
}

func TestDirectoriesShowModTime(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	stamp := time.Date(2020, 3, 4, 5, 6, 0, 0, time.Local)
	if err := os.Chtimes(filepath.Join(root, "sub"), stamp, stamp); err != nil {
		t.Fatal(err)
	}

	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if !strings.Contains(rec.Body.String(), "2020-03-04 05:06") {
		t.Fatalf("body = %q, want directory mod time in listing", rec.Body.String())
	}
}

func TestSortNewestByDefault(t *testing.T) {
	root := t.TempDir()
	old := time.Now().Add(-48 * time.Hour)
	for name, when := range map[string]time.Time{
		"aaa-old.txt": old,
		"zzz-new.txt": time.Now(),
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(filepath.Join(root, name), when, when); err != nil {
			t.Fatal(err)
		}
	}

	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}

	for _, tc := range []struct {
		target    string
		wantFirst string
	}{
		{target: "/", wantFirst: "zzz-new.txt"},
		{target: "/?sort=name", wantFirst: "aaa-old.txt"},
		{target: "/?sort=name&dir=desc", wantFirst: "zzz-new.txt"},
		{target: "/?sort=time&dir=asc", wantFirst: "aaa-old.txt"},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.target, nil)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)

		body := rec.Body.String()
		newer := strings.Index(body, `href="zzz-new.txt"`)
		older := strings.Index(body, `href="aaa-old.txt"`)
		if newer < 0 || older < 0 {
			t.Fatalf("GET %s body = %q, want both files listed", tc.target, body)
		}
		first := "zzz-new.txt"
		if older < newer {
			first = "aaa-old.txt"
		}
		if first != tc.wantFirst {
			t.Fatalf("GET %s lists %s first, want %s", tc.target, first, tc.wantFirst)
		}
	}
}

func TestZipListing(t *testing.T) {
	root := t.TempDir()
	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	for name, content := range map[string]string{
		"docs/notes.txt": "some notes",
		"prog.go":        "package main",
	} {
		f, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bundle.zip"), zbuf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}

	nav := func(target string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.Header.Set("Sec-Fetch-Dest", "document")
		req.Header.Set("Accept", "text/html,application/xhtml+xml")
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		return rec
	}

	rec := nav("/bundle.zip")
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "/bundle.zip/" {
		t.Fatalf("GET /bundle.zip = %d %q, want redirect to /bundle.zip/", rec.Code, rec.Header().Get("Location"))
	}

	rec = nav("/bundle.zip/")
	if rec.Code != http.StatusOK {
		t.Fatalf("zip listing status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `href="docs/"`) || !strings.Contains(body, `href="prog.go"`) {
		t.Fatalf("body = %q, want directory-style links into the archive", body)
	}
	if !strings.Contains(body, `href="/bundle.zip?raw=1"`) {
		t.Fatalf("body = %q, want raw download link for the archive", body)
	}

	rec = nav("/bundle.zip/docs/")
	if !strings.Contains(rec.Body.String(), `href="notes.txt"`) {
		t.Fatalf("nested listing = %q, want notes.txt link", rec.Body.String())
	}

	rec = nav("/bundle.zip/prog.go")
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Fatalf("member navigation Content-Type = %q, want highlighted text/html", got)
	}
	if !strings.Contains(rec.Body.String(), `id="L1"`) {
		t.Fatalf("member body = %q, want highlighted member source", rec.Body.String())
	}

	plainMember := httptest.NewRequest(http.MethodGet, "/bundle.zip/docs/notes.txt", nil)
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, plainMember)
	if got := rec.Body.String(); got != "some notes" {
		t.Fatalf("member fetch body = %q, want verbatim member contents", got)
	}

	plain := httptest.NewRequest(http.MethodGet, "/bundle.zip", nil)
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, plain)
	if !bytes.Equal(rec.Body.Bytes(), zbuf.Bytes()) {
		t.Fatalf("non-browser fetch returned %d bytes, want verbatim %d-byte zip", rec.Body.Len(), zbuf.Len())
	}
}

func TestPandocDocumentFormats(t *testing.T) {
	for ext, wantFormat := range map[string]string{
		".ipynb": "ipynb",
		".docx":  "docx",
		".odt":   "odt",
		".rtf":   "rtf",
		".epub":  "epub",
		".rst":   "rst",
	} {
		t.Run(ext, func(t *testing.T) {
			root := t.TempDir()
			content := []byte("not really " + ext + " but the fake pandoc does not care")
			if err := os.WriteFile(filepath.Join(root, "doc"+ext), content, 0o644); err != nil {
				t.Fatal(err)
			}

			server := fileServer{fsys: os.DirFS(root), pandoc: fakePandoc(t)}

			nav := httptest.NewRequest(http.MethodGet, "/doc"+ext, nil)
			nav.Header.Set("Sec-Fetch-Dest", "document")
			rec := httptest.NewRecorder()
			server.ServeHTTP(rec, nav)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d; body: %s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "<!--fmt:"+wantFormat+"-->") {
				t.Fatalf("body = %q, want pandoc invoked with -f %s", rec.Body.String(), wantFormat)
			}

			plain := httptest.NewRequest(http.MethodGet, "/doc"+ext, nil)
			rec = httptest.NewRecorder()
			server.ServeHTTP(rec, plain)
			if !bytes.Equal(rec.Body.Bytes(), content) {
				t.Fatalf("non-browser fetch = %q, want verbatim bytes", rec.Body.String())
			}
		})
	}
}

func TestLegacyDocRendered(t *testing.T) {
	if _, err := exec.LookPath("textutil"); err != nil {
		t.Skip("textutil not available")
	}

	root := t.TempDir()
	src := filepath.Join(root, "src.txt")
	if err := os.WriteFile(src, []byte("hello from a legacy word document"), 0o644); err != nil {
		t.Fatal(err)
	}
	docPath := filepath.Join(root, "cv.doc")
	if out, err := exec.Command("textutil", "-convert", "doc", "-output", docPath, src).CombinedOutput(); err != nil {
		t.Skipf("cannot create .doc fixture: %v: %s", err, out)
	}
	if err := os.Remove(src); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatal(err)
	}

	server := fileServer{fsys: os.DirFS(root), pandoc: fakePandoc(t)}

	nav := httptest.NewRequest(http.MethodGet, "/cv.doc", nil)
	nav.Header.Set("Sec-Fetch-Dest", "document")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, nav)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "<!--fmt:html-->") {
		t.Fatalf("body = %q, want textutil conversion fed to pandoc as html", rec.Body.String())
	}

	plain := httptest.NewRequest(http.MethodGet, "/cv.doc", nil)
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, plain)
	if !bytes.Equal(rec.Body.Bytes(), raw) {
		t.Fatalf("non-browser fetch = %d bytes, want verbatim %d-byte doc", rec.Body.Len(), len(raw))
	}
}

func TestXLSXRenderedAsTables(t *testing.T) {
	root := t.TempDir()
	wb := excelize.NewFile()
	if err := wb.SetSheetName("Sheet1", "cities"); err != nil {
		t.Fatal(err)
	}
	for i, row := range [][]any{
		{"city", "population"},
		{"new orleans", 364136},
	} {
		if err := wb.SetSheetRow("cities", fmt.Sprintf("A%d", i+1), &row); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := wb.NewSheet("empty"); err != nil {
		t.Fatal(err)
	}
	// data starting below row 1, as when a chart occupies the top of a sheet
	if _, err := wb.NewSheet("offset"); err != nil {
		t.Fatal(err)
	}
	for i, row := range [][]any{
		{"metric", "value"},
		{"count", 7},
	} {
		if err := wb.SetSheetRow("offset", fmt.Sprintf("B%d", i+3), &row); err != nil {
			t.Fatal(err)
		}
	}
	xlsxPath := filepath.Join(root, "data.xlsx")
	if err := wb.SaveAs(xlsxPath); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(xlsxPath)
	if err != nil {
		t.Fatal(err)
	}

	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}

	nav := func(target string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.Header.Set("Sec-Fetch-Dest", "document")
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		return rec
	}

	rec := nav("/data.xlsx")
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "/data.xlsx/" {
		t.Fatalf("GET /data.xlsx = %d %q, want redirect to /data.xlsx/", rec.Code, rec.Header().Get("Location"))
	}

	rec = nav("/data.xlsx/")
	if rec.Code != http.StatusOK {
		t.Fatalf("listing status = %d; body: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`href="cities"`,
		`href="empty"`,
		`href="offset"`,
		"1 row", // the cities and offset sheets have one data row each
		`id="listsum">3 sheets · 2 rows</p>`,
		`href="/data.xlsx?raw=1"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("listing body = %q, want %q", body, want)
		}
	}

	rec = nav("/data.xlsx/cities")
	if rec.Code != http.StatusOK {
		t.Fatalf("sheet status = %d; body: %s", rec.Code, rec.Body.String())
	}
	body = rec.Body.String()
	for _, want := range []string{
		"<th>city</th><th>population</th>",
		"<td>new orleans</td><td>364136</td>",
		"1 row × 2 columns",
		`href="cities?raw=1"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("sheet body = %q, want %q", body, want)
		}
	}

	rec = nav("/data.xlsx/offset")
	if rec.Code != http.StatusOK {
		t.Fatalf("offset sheet status = %d; body: %s", rec.Code, rec.Body.String())
	}
	body = rec.Body.String()
	for _, want := range []string{
		"<th></th><th>metric</th><th>value</th>", // blank rows above the data dropped
		"<td></td><td>count</td><td>7</td>",
		"1 row × 3 columns",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("offset sheet body = %q, want %q", body, want)
		}
	}

	csvReq := httptest.NewRequest(http.MethodGet, "/data.xlsx/cities?raw=1", nil)
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, csvReq)
	if got := rec.Body.String(); got != "city,population\nnew orleans,364136\n" {
		t.Fatalf("sheet csv = %q, want csv export", got)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/csv") {
		t.Fatalf("sheet csv Content-Type = %q, want text/csv", got)
	}

	csvReq = httptest.NewRequest(http.MethodGet, "/data.xlsx/offset?raw=1", nil)
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, csvReq)
	if got := rec.Body.String(); got != ",metric,value\n,count,7\n" {
		t.Fatalf("offset sheet csv = %q, want export without leading blank rows", got)
	}

	plain := httptest.NewRequest(http.MethodGet, "/data.xlsx", nil)
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, plain)
	if !bytes.Equal(rec.Body.Bytes(), raw) {
		t.Fatalf("non-browser fetch = %d bytes, want verbatim %d-byte xlsx", rec.Body.Len(), len(raw))
	}
}

func TestSQLiteRenderedAsTables(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "app.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE users (name TEXT, email TEXT, age INTEGER);
		INSERT INTO users VALUES ('mike', 'mike@example.com', 55), ('nobody', NULL, NULL);`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}

	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}

	nav := func(target string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.Header.Set("Sec-Fetch-Dest", "document")
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		return rec
	}

	rec := nav("/app.sqlite")
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "/app.sqlite/" {
		t.Fatalf("GET /app.sqlite = %d %q, want redirect to /app.sqlite/", rec.Code, rec.Header().Get("Location"))
	}

	rec = nav("/app.sqlite/")
	if rec.Code != http.StatusOK {
		t.Fatalf("listing status = %d; body: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`href="users"`,
		"2 rows × 3 columns", // per-table stats in the metadata column
		`id="listsum">1 table · 2 rows · 4.0 KB</p>`, // summary totals; footprint via dbstat
		`name="q"`, // the query form
		`href="/app.sqlite?raw=1"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("listing body = %q, want %q", body, want)
		}
	}

	rec = nav("/app.sqlite/?q=" + url.QueryEscape("select name from users order by name"))
	body = rec.Body.String()
	for _, want := range []string{
		"<th>name</th>",
		"<td>mike</td>",
		"<td>nobody</td>",
		"2 rows × 1 column",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("query result body = %q, want %q", body, want)
		}
	}

	rec = nav("/app.sqlite/?q=" + url.QueryEscape("delete from users"))
	if !strings.Contains(rec.Body.String(), `class="queryerror"`) {
		t.Fatalf("write query body = %q, want a query error (read-only db)", rec.Body.String())
	}

	rec = nav("/app.sqlite/users")
	if rec.Code != http.StatusOK {
		t.Fatalf("table status = %d; body: %s", rec.Code, rec.Body.String())
	}
	body = rec.Body.String()
	for _, want := range []string{
		"2 rows × 3 columns",
		"<th>name</th><th>email</th><th>age</th>",
		"<td>mike</td><td>mike@example.com</td><td>55</td>",
		"<td>NULL</td>",
		`href="users?raw=1"`,
		`<h2 class="sheet">schema</h2>`,
		"CREATE",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("table body = %q, want %q", body, want)
		}
	}

	csvReq := httptest.NewRequest(http.MethodGet, "/app.sqlite/users?raw=1", nil)
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, csvReq)
	if got := rec.Body.String(); got != "name,email,age\nmike,mike@example.com,55\nnobody,,\n" {
		t.Fatalf("table csv = %q, want csv export with empty NULLs", got)
	}

	plain := httptest.NewRequest(http.MethodGet, "/app.sqlite", nil)
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, plain)
	if !bytes.Equal(rec.Body.Bytes(), raw) {
		t.Fatalf("non-browser fetch = %d bytes, want verbatim %d-byte db", rec.Body.Len(), len(raw))
	}
}

func TestCSVAndTSVRenderedAsTables(t *testing.T) {
	for _, tc := range []struct {
		file    string
		content string
	}{
		{file: "data.csv", content: "city,population\nnew orleans,364136\n"},
		{file: "data.tsv", content: "city\tpopulation\nnew orleans\t364136\n"},
	} {
		t.Run(tc.file, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, tc.file), []byte(tc.content), 0o644); err != nil {
				t.Fatal(err)
			}

			server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}

			nav := httptest.NewRequest(http.MethodGet, "/"+tc.file, nil)
			nav.Header.Set("Sec-Fetch-Dest", "document")
			rec := httptest.NewRecorder()
			server.ServeHTTP(rec, nav)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
			}
			if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
				t.Fatalf("Content-Type = %q, want table as text/html", got)
			}
			body := rec.Body.String()
			for _, want := range []string{
				`<th class="corner"></th><th>city</th><th>population</th>`,
				`<tr class="coords"><td class="rownum"></td><td class="colnum">1</td><td class="colnum">2</td></tr>`,
				`<td class="rownum">1</td><td>new orleans</td><td>364136</td>`,
				"1 rows × 2 columns",
				`href="` + tc.file + `?raw=1"`,
			} {
				if !strings.Contains(body, want) {
					t.Fatalf("body = %q, want %q", body, want)
				}
			}
			if strings.Index(body, `class="coords"`) > strings.Index(body, `<th class="corner">`) {
				t.Fatalf("body = %q, want column-number row above the header row", body)
			}

			plain := httptest.NewRequest(http.MethodGet, "/"+tc.file, nil)
			rec = httptest.NewRecorder()
			server.ServeHTTP(rec, plain)
			if got := rec.Body.String(); got != tc.content {
				t.Fatalf("non-browser body = %q, want verbatim file", got)
			}
		})
	}
}

func TestTruncatedCellsLinkToFullView(t *testing.T) {
	longText := strings.Repeat("lorem ipsum ", 30) // 360 chars, plain text
	binaryText := strings.Repeat("x", maxCellChars) + "\x01binary tail"

	nav := func(server fileServer, target string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.Header.Set("Sec-Fetch-Dest", "document")
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		return rec
	}

	t.Run("csv", func(t *testing.T) {
		root := t.TempDir()
		content := "name,comment,junk\nmike," + longText + "," + binaryText + "\n"
		if err := os.WriteFile(filepath.Join(root, "notes.csv"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}

		body := nav(server, "/notes.csv").Body.String()
		if !strings.Contains(body, `<a class="more" href="?cell=1,2">`) {
			t.Fatalf("table body = %q, want a more-link on the truncated text cell", body)
		}
		if !strings.Contains(body, "scrollIntoView") {
			t.Fatalf("table body = %q, want the cell-focus script for #cell= fragments", body)
		}
		if strings.Contains(body, longText) {
			t.Fatalf("table body = %q, want the long cell truncated", body)
		}
		if strings.Contains(body, `href="?cell=1,3"`) {
			t.Fatalf("table body = %q, want no more-link on the binary cell", body)
		}

		rec := nav(server, "/notes.csv?cell=1,2")
		if rec.Code != http.StatusOK {
			t.Fatalf("cell status = %d; body: %s", rec.Code, rec.Body.String())
		}
		body = rec.Body.String()
		for _, want := range []string{
			longText,
			"row 1, column 2 (comment)",
			`href="notes.csv#cell=1,2"`,
			"back to table",
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("cell body = %q, want %q", body, want)
			}
		}

		for _, target := range []string{
			"/notes.csv?cell=1,3", // binary content
			"/notes.csv?cell=2,1", // beyond the last row
			"/notes.csv?cell=1,9", // beyond the last column
			"/notes.csv?cell=bogus",
		} {
			if rec := nav(server, target); rec.Code != http.StatusNotFound {
				t.Fatalf("GET %s = %d, want %d", target, rec.Code, http.StatusNotFound)
			}
		}
	})

	t.Run("sqlite", func(t *testing.T) {
		root := t.TempDir()
		db, err := sql.Open("sqlite", filepath.Join(root, "app.sqlite"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`CREATE TABLE notes (author TEXT, comment TEXT, data BLOB);
			INSERT INTO notes VALUES ('mike', ?, x'000102');`, longText); err != nil {
			t.Fatal(err)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}

		body := nav(server, "/app.sqlite/notes").Body.String()
		if !strings.Contains(body, `<a class="more" href="?cell=1,2">`) {
			t.Fatalf("table body = %q, want a more-link on the truncated text cell", body)
		}
		if !strings.Contains(body, "(3-byte blob)") {
			t.Fatalf("table body = %q, want the blob placeholder without a link", body)
		}

		rec := nav(server, "/app.sqlite/notes?cell=1,2")
		if rec.Code != http.StatusOK {
			t.Fatalf("cell status = %d; body: %s", rec.Code, rec.Body.String())
		}
		body = rec.Body.String()
		for _, want := range []string{longText, "row 1, column 2 (comment)", `href="notes#cell=1,2"`} {
			if !strings.Contains(body, want) {
				t.Fatalf("cell body = %q, want %q", body, want)
			}
		}

		if rec := nav(server, "/app.sqlite/notes?cell=2,1"); rec.Code != http.StatusNotFound {
			t.Fatalf("out-of-range cell = %d, want %d", rec.Code, http.StatusNotFound)
		}

		query := "select comment from notes"
		body = nav(server, "/app.sqlite/?q="+url.QueryEscape(query)).Body.String()
		if !strings.Contains(body, `class="more"`) || !strings.Contains(body, "cell=1,1") {
			t.Fatalf("query body = %q, want a more-link on the truncated result cell", body)
		}
		if !strings.Contains(body, "scrollIntoView") {
			t.Fatalf("query body = %q, want the cell-focus script for #cell= fragments", body)
		}

		rec = nav(server, "/app.sqlite/?q="+url.QueryEscape(query)+"&cell=1,1")
		if rec.Code != http.StatusOK {
			t.Fatalf("query cell status = %d; body: %s", rec.Code, rec.Body.String())
		}
		body = rec.Body.String()
		// html/template writes "+" as &#43; inside href attributes.
		backHref := strings.ReplaceAll(url.QueryEscape(query), "+", "&#43;")
		for _, want := range []string{longText, "row 1, column 1 (comment)", `href="?q=` + backHref + `#cell=1,1"`} {
			if !strings.Contains(body, want) {
				t.Fatalf("query cell body = %q, want %q", body, want)
			}
		}
	})

	t.Run("xlsx", func(t *testing.T) {
		root := t.TempDir()
		wb := excelize.NewFile()
		for i, row := range [][]any{
			{"name", "comment"},
			{"mike", longText},
		} {
			if err := wb.SetSheetRow("Sheet1", fmt.Sprintf("A%d", i+1), &row); err != nil {
				t.Fatal(err)
			}
		}
		if err := wb.SaveAs(filepath.Join(root, "data.xlsx")); err != nil {
			t.Fatal(err)
		}
		server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}

		body := nav(server, "/data.xlsx/Sheet1").Body.String()
		if !strings.Contains(body, `<a class="more" href="?cell=1,2">`) {
			t.Fatalf("sheet body = %q, want a more-link on the truncated text cell", body)
		}

		rec := nav(server, "/data.xlsx/Sheet1?cell=1,2")
		if rec.Code != http.StatusOK {
			t.Fatalf("cell status = %d; body: %s", rec.Code, rec.Body.String())
		}
		body = rec.Body.String()
		for _, want := range []string{longText, "row 1, column 2 (comment)", `href="Sheet1#cell=1,2"`} {
			if !strings.Contains(body, want) {
				t.Fatalf("cell body = %q, want %q", body, want)
			}
		}
	})
}

func TestURLCellsHyperlinked(t *testing.T) {
	longURL := "https://example.com/path?id=" + strings.Repeat("x", maxCellChars)

	root := t.TempDir()
	content := "name,link\n" +
		"plain,https://example.com/a?b=1&c=2\n" +
		"spaced,https://example.com and some words\n" +
		"scheme,ftp://example.com/file\n" +
		"long," + longURL + "\n"
	if err := os.WriteFile(filepath.Join(root, "links.csv"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}

	nav := func(target string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.Header.Set("Sec-Fetch-Dest", "document")
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		return rec
	}

	body := nav("/links.csv").Body.String()
	if !strings.Contains(body, `<a href="https://example.com/a?b=1&amp;c=2" target="_blank" rel="noopener">https://example.com/a?b=1&amp;c=2</a>`) {
		t.Fatalf("table body = %q, want the URL cell hyperlinked", body)
	}
	if strings.Contains(body, `href="https://example.com and`) || strings.Contains(body, `href="ftp:`) {
		t.Fatalf("table body = %q, want no links for non-URL cells", body)
	}
	// A cut URL cell links its visible prefix to the full URL and keeps
	// its "… more" link.
	if !strings.Contains(body, `<a href="`+longURL+`" target="_blank"`) ||
		!strings.Contains(body, `<a class="more" href="?cell=4,2">`) {
		t.Fatalf("table body = %q, want the long URL cell linked and cut", body)
	}

	body = nav("/links.csv?cell=4,2").Body.String()
	if !strings.Contains(body, `<a href="`+longURL+`" target="_blank" rel="noopener">`+longURL+`</a>`) {
		t.Fatalf("cell body = %q, want the full URL hyperlinked", body)
	}
}

func TestTarBrowsing(t *testing.T) {
	makeTar := func(t *testing.T, gzipped bool) []byte {
		t.Helper()
		var buf bytes.Buffer
		var w io.Writer = &buf
		var gzw *gzip.Writer
		if gzipped {
			gzw = gzip.NewWriter(&buf)
			w = gzw
		}
		tw := tar.NewWriter(w)
		if err := tw.WriteHeader(&tar.Header{Name: "docs/", Typeflag: tar.TypeDir, Mode: 0o755}); err != nil {
			t.Fatal(err)
		}
		for name, content := range map[string]string{
			"docs/notes.txt": "tar notes",
			"prog.go":        "package main",
		} {
			if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content))}); err != nil {
				t.Fatal(err)
			}
			if _, err := tw.Write([]byte(content)); err != nil {
				t.Fatal(err)
			}
		}
		if err := tw.Close(); err != nil {
			t.Fatal(err)
		}
		if gzw != nil {
			if err := gzw.Close(); err != nil {
				t.Fatal(err)
			}
		}
		return buf.Bytes()
	}

	for _, tc := range []struct {
		file    string
		gzipped bool
	}{
		{file: "bundle.tar", gzipped: false},
		{file: "bundle.tar.gz", gzipped: true},
	} {
		t.Run(tc.file, func(t *testing.T) {
			root := t.TempDir()
			data := makeTar(t, tc.gzipped)
			if err := os.WriteFile(filepath.Join(root, tc.file), data, 0o644); err != nil {
				t.Fatal(err)
			}

			server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}

			nav := httptest.NewRequest(http.MethodGet, "/"+tc.file+"/", nil)
			nav.Header.Set("Sec-Fetch-Dest", "document")
			rec := httptest.NewRecorder()
			server.ServeHTTP(rec, nav)
			if rec.Code != http.StatusOK {
				t.Fatalf("listing status = %d; body: %s", rec.Code, rec.Body.String())
			}
			body := rec.Body.String()
			if !strings.Contains(body, `href="docs/"`) || !strings.Contains(body, `href="prog.go"`) {
				t.Fatalf("body = %q, want directory-style links into the tar", body)
			}

			member := httptest.NewRequest(http.MethodGet, "/"+tc.file+"/docs/notes.txt", nil)
			rec = httptest.NewRecorder()
			server.ServeHTTP(rec, member)
			if got := rec.Body.String(); got != "tar notes" {
				t.Fatalf("member fetch body = %q, want verbatim member contents", got)
			}

			plain := httptest.NewRequest(http.MethodGet, "/"+tc.file, nil)
			rec = httptest.NewRecorder()
			server.ServeHTTP(rec, plain)
			if !bytes.Equal(rec.Body.Bytes(), data) {
				t.Fatalf("non-browser fetch returned %d bytes, want verbatim %d-byte tar", rec.Body.Len(), len(data))
			}
		})
	}
}

func TestOversizedTarServedVerbatim(t *testing.T) {
	root := t.TempDir()
	big := make([]byte, maxTarFileBytes+1)
	if err := os.WriteFile(filepath.Join(root, "big.tar"), big, 0o644); err != nil {
		t.Fatal(err)
	}

	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}

	nav := httptest.NewRequest(http.MethodGet, "/big.tar/", nil)
	nav.Header.Set("Sec-Fetch-Dest", "document")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, nav)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (fall back to raw)", rec.Code, http.StatusOK)
	}
	if rec.Body.Len() != len(big) {
		t.Fatalf("body = %d bytes, want verbatim %d-byte tar", rec.Body.Len(), len(big))
	}
}

func TestZipWithLatin1NamesBrowsable(t *testing.T) {
	root := t.TempDir()
	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	for _, name := range []string{"docs/r\xe9sum\xe9.txt", "plain.txt"} {
		hdr := &zip.FileHeader{Name: name, NonUTF8: true}
		f, err := zw.CreateHeader(hdr)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte("contents of " + name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "legacy.zip"), zbuf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}

	nav := func(target string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.Header.Set("Sec-Fetch-Dest", "document")
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		return rec
	}

	rec := nav("/legacy.zip/docs/")
	if rec.Code != http.StatusOK {
		t.Fatalf("listing status = %d; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "résumé.txt") {
		t.Fatalf("listing = %q, want Latin-1 name decoded to résumé.txt", rec.Body.String())
	}

	member := httptest.NewRequest(http.MethodGet, "/legacy.zip/docs/r%C3%A9sum%C3%A9.txt", nil)
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, member)
	if got := rec.Body.String(); got != "contents of docs/r\xe9sum\xe9.txt" {
		t.Fatalf("member fetch = %q, want original contents under decoded name", got)
	}
}

func TestMarkdownInsideZipRendered(t *testing.T) {
	root := t.TempDir()
	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	f, err := zw.Create("doc.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("# Hello\n")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bundle.zip"), zbuf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	server := fileServer{fsys: os.DirFS(root), pandoc: fakePandoc(t)}

	req := httptest.NewRequest(http.MethodGet, "/bundle.zip/doc.md", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "<p>rendered markdown</p>") {
		t.Fatalf("body = %q, want markdown inside zip rendered through pandoc", rec.Body.String())
	}
}

func TestOversizedContainerDownloadsUnderOwnName(t *testing.T) {
	root := t.TempDir()
	big := make([]byte, maxXLSXBytes+1)
	if err := os.WriteFile(filepath.Join(root, "big.xlsx"), big, 0o644); err != nil {
		t.Fatal(err)
	}

	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}

	req := httptest.NewRequest(http.MethodGet, "/big.xlsx", nil)
	req.Header.Set("Sec-Fetch-Dest", "document")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (raw fallback, not a redirect)", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Disposition"); !strings.Contains(got, `filename=big.xlsx`) && !strings.Contains(got, `filename="big.xlsx"`) {
		t.Fatalf("Content-Disposition = %q, want the real filename", got)
	}
	if rec.Body.Len() != len(big) {
		t.Fatalf("body = %d bytes, want verbatim %d", rec.Body.Len(), len(big))
	}
}

func TestSQLiteOpenedInPlaceWithDir(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "big.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE t (x TEXT); INSERT INTO t VALUES ('direct');`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	server := fileServer{fsys: os.DirFS(root), pandoc: "unused", dir: root}

	req := httptest.NewRequest(http.MethodGet, "/big.sqlite/t", nil)
	req.Header.Set("Sec-Fetch-Dest", "document")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %.200s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "<td>direct</td>") {
		t.Fatalf("body = %q, want table contents via in-place open", rec.Body.String())
	}

	// On-disk databases list instantly: names only, stats filled in by the
	// async script against the ?stat endpoint.
	listing := httptest.NewRequest(http.MethodGet, "/big.sqlite/", nil)
	listing.Header.Set("Sec-Fetch-Dest", "document")
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, listing)
	body := rec.Body.String()
	for _, want := range []string{
		`id="statprog"`,
		`data-name="t"`,
		"computing table stats",
		`id="listsum">1 table</p>`, // member counts render instantly; totals arrive with the async stats
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("listing body = %q, want async stats marker %q", body, want)
		}
	}
	if strings.Contains(body, "1 row × 1 column") {
		t.Fatalf("listing body computed stats synchronously, want async")
	}

	stat := httptest.NewRequest(http.MethodGet, "/big.sqlite/?stat=t", nil)
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, stat)
	if rec.Code != http.StatusOK {
		t.Fatalf("stat status = %d; body: %s", rec.Code, rec.Body.String())
	}
	got := rec.Body.String()
	if !strings.Contains(got, "1 row × 1 column") {
		t.Fatalf("stat body = %q, want row and column counts", got)
	}
	if !strings.Contains(got, `"rows":1`) {
		t.Fatalf("stat body = %q, want raw row count for the summary totals", got)
	}
}

func TestPrettySQL(t *testing.T) {
	in := `CREATE TABLE classifications (gmail_id TEXT PRIMARY KEY, category TEXT NOT NULL, amount DECIMAL(10,2), blurb TEXT DEFAULT '', note TEXT DEFAULT 'a,b', CHECK (category IN ('x','y'))) WITHOUT ROWID`
	want := `CREATE TABLE classifications (
  gmail_id TEXT PRIMARY KEY,
  category TEXT NOT NULL,
  amount DECIMAL(10,2),
  blurb TEXT DEFAULT '',
  note TEXT DEFAULT 'a,b',
  CHECK (category IN ('x','y'))
) WITHOUT ROWID`
	if got := prettySQL(in); got != want {
		t.Fatalf("prettySQL = %q, want %q", got, want)
	}

	for _, passthrough := range []string{
		"CREATE INDEX idx ON t (a, b)",
		"CREATE TRIGGER tr AFTER INSERT ON t BEGIN SELECT 1, 2; END",
		"CREATE TABLE already (\n  a TEXT,\n  b TEXT\n)",
		"CREATE TABLE single (a TEXT)",
	} {
		if got := prettySQL(passthrough); got != passthrough {
			t.Fatalf("prettySQL(%q) = %q, want unchanged", passthrough, got)
		}
	}
}

func TestJSONPrettyPrinted(t *testing.T) {
	root := t.TempDir()
	minified := `{"a":[1,2],"b":{"c":"d"}}`
	if err := os.WriteFile(filepath.Join(root, "data.json"), []byte(minified), 0o644); err != nil {
		t.Fatal(err)
	}

	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}

	nav := httptest.NewRequest(http.MethodGet, "/data.json", nil)
	nav.Header.Set("Sec-Fetch-Dest", "document")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, nav)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Fatalf("Content-Type = %q, want highlighted text/html", got)
	}
	// The one-line input indents onto nine lines.
	if body := rec.Body.String(); !strings.Contains(body, `id="L9"`) {
		t.Fatalf("body = %q, want pretty-printed onto multiple lines", body)
	}

	raw := httptest.NewRequest(http.MethodGet, "/data.json", nil)
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, raw)
	if got := rec.Body.String(); got != minified {
		t.Fatalf("non-browser body = %q, want verbatim %q", got, minified)
	}
}

func TestInvalidJSONShownAsIs(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "bad.json"), []byte(`{"a":`), 0o644); err != nil {
		t.Fatal(err)
	}

	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}

	req := httptest.NewRequest(http.MethodGet, "/bad.json", nil)
	req.Header.Set("Sec-Fetch-Dest", "document")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `id="L1"`) || strings.Contains(body, `id="L2"`) {
		t.Fatalf("body = %q, want the single original line untouched", body)
	}
}

func TestOversizedJSONShownAsPlainText(t *testing.T) {
	root := t.TempDir()
	big := "[" + strings.Repeat(`"x",`, maxHighlightBytes/4) + `"x"]`
	if err := os.WriteFile(filepath.Join(root, "big.json"), []byte(big), 0o644); err != nil {
		t.Fatal(err)
	}

	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}

	nav := httptest.NewRequest(http.MethodGet, "/big.json", nil)
	nav.Header.Set("Sec-Fetch-Dest", "document")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, nav)

	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/plain") {
		t.Fatalf("navigation Content-Type = %q, want text/plain", got)
	}
	if got := rec.Body.String(); got != big {
		t.Fatalf("navigation body = %d bytes, want the %d original bytes verbatim", len(got), len(big))
	}

	raw := httptest.NewRequest(http.MethodGet, "/big.json", nil)
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, raw)
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("non-browser Content-Type = %q, want application/json", got)
	}
}

func TestNewHighlightExtensions(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"prog.f90":        "program hello\nend program\n",
		"pkg.adb":         "procedure Hello is\nbegin\n null;\nend Hello;\n",
		"app.coffee":      "square = (x) -> x * x\n",
		"login.feature":   "Feature: login\n  Scenario: ok\n",
		"boot.s":          "mov r0, #0\n",
		"view.erb":        "<%= @name %>\n",
		"conf.properties": "key=value\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}
	for name := range files {
		req := httptest.NewRequest(http.MethodGet, "/"+name, nil)
		req.Header.Set("Sec-Fetch-Dest", "document")
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
			t.Errorf("%s Content-Type = %q, want highlighted text/html", name, got)
		}
	}
}

func TestJarBrowsesAsArchive(t *testing.T) {
	root := t.TempDir()
	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	f, err := zw.Create("META-INF/MANIFEST.MF")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("Manifest-Version: 1.0\n")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "lib.jar"), zbuf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}

	req := httptest.NewRequest(http.MethodGet, "/lib.jar/", nil)
	req.Header.Set("Sec-Fetch-Dest", "document")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `href="META-INF/"`) {
		t.Fatalf("jar listing status = %d body = %.300q, want archive listing", rec.Code, rec.Body.String())
	}
}

func TestVideoViewerPage(t *testing.T) {
	root := t.TempDir()
	content := []byte("not really a movie")
	if err := os.WriteFile(filepath.Join(root, "clip.mov"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}

	nav := httptest.NewRequest(http.MethodGet, "/clip.mov", nil)
	nav.Header.Set("Sec-Fetch-Dest", "document")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, nav)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %.200s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`<video class="subject" controls`, `src="clip.mov?raw=1"`, "file size", "video/quicktime"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body = %q, want %q", body, want)
		}
	}

	plain := httptest.NewRequest(http.MethodGet, "/clip.mov", nil)
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, plain)
	if !bytes.Equal(rec.Body.Bytes(), content) {
		t.Fatalf("non-browser fetch = %q, want verbatim bytes", rec.Body.String())
	}
}

func TestHEICViewer(t *testing.T) {
	if _, err := exec.LookPath("sips"); err != nil {
		t.Skip("sips not available")
	}

	root := t.TempDir()
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, image.NewRGBA(image.Rect(0, 0, 40, 30))); err != nil {
		t.Fatal(err)
	}
	pngPath := filepath.Join(root, "src.png")
	if err := os.WriteFile(pngPath, pngBuf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	heicPath := filepath.Join(root, "photo.heic")
	if out, err := exec.Command("sips", "-s", "format", "heic", pngPath, "--out", heicPath).CombinedOutput(); err != nil {
		t.Skipf("cannot create heic fixture: %v: %s", err, out)
	}
	if err := os.Remove(pngPath); err != nil {
		t.Fatal(err)
	}

	server := fileServer{fsys: os.DirFS(root), pandoc: "unused", dir: root}

	nav := httptest.NewRequest(http.MethodGet, "/photo.heic", nil)
	nav.Header.Set("Sec-Fetch-Dest", "document")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, nav)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %.200s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`src="photo.heic?jpeg=1"`,
		"40 × 30",
		">image/heic<",
		"converted to JPEG with sips",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body = %q, want %q", body, want)
		}
	}

	preview := httptest.NewRequest(http.MethodGet, "/photo.heic?jpeg=1", nil)
	preview.Header.Set("Sec-Fetch-Dest", "image")
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, preview)
	if rec.Code != http.StatusOK || !bytes.HasPrefix(rec.Body.Bytes(), []byte{0xFF, 0xD8}) {
		t.Fatalf("preview status = %d, want JPEG bytes", rec.Code)
	}
}

func TestBackupFilesUseBaseViewer(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{
		"note.md~":  "# Hello\n",
		"prog.go~":  "package main\n",
		"data.csv~": "city,population\nnew orleans,364136\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	server := fileServer{fsys: os.DirFS(root), pandoc: fakePandoc(t)}

	nav := func(target string) string {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.Header.Set("Sec-Fetch-Dest", "document")
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d; body: %.200s", target, rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}

	if body := nav("/note.md~"); !strings.Contains(body, "<p>rendered markdown</p>") {
		t.Fatalf("note.md~ = %q, want markdown rendering", body)
	}
	if body := nav("/prog.go~"); !strings.Contains(body, `color:#cf222e">package<`) {
		t.Fatalf("prog.go~ = %q, want go-highlighted keyword", body)
	}
	if body := nav("/data.csv~"); !strings.Contains(body, "<td>new orleans</td><td>364136</td>") {
		t.Fatalf("data.csv~ = %q, want table rendering", body)
	}

	plain := httptest.NewRequest(http.MethodGet, "/prog.go~", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, plain)
	if got := rec.Body.String(); got != "package main\n" {
		t.Fatalf("non-browser fetch = %q, want verbatim backup file", got)
	}
}

func TestPlistViewer(t *testing.T) {
	if _, err := exec.LookPath("plutil"); err != nil {
		t.Skip("plutil not available")
	}

	root := t.TempDir()
	xmlPath := filepath.Join(root, "prefs.plist")
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>magicSetting</key><true/></dict></plist>`
	if err := os.WriteFile(xmlPath, []byte(xml), 0o644); err != nil {
		t.Fatal(err)
	}
	// Make a genuine binary plist out of it.
	binPath := filepath.Join(root, "binary.plist")
	if out, err := exec.Command("plutil", "-convert", "binary1", "-o", binPath, xmlPath).CombinedOutput(); err != nil {
		t.Skipf("cannot create binary plist: %v: %s", err, out)
	}
	raw, err := os.ReadFile(binPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(raw, []byte("bplist")) {
		t.Fatalf("fixture is not a binary plist")
	}

	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}

	for _, target := range []string{"/binary.plist", "/prefs.plist"} {
		nav := httptest.NewRequest(http.MethodGet, target, nil)
		nav.Header.Set("Sec-Fetch-Dest", "document")
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, nav)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d; body: %.200s", target, rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if !strings.Contains(body, "magicSetting") || !strings.Contains(body, `id="L1"`) {
			t.Fatalf("%s body = %q, want highlighted XML with the plist key", target, body)
		}
	}

	plain := httptest.NewRequest(http.MethodGet, "/binary.plist", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, plain)
	if !bytes.Equal(rec.Body.Bytes(), raw) {
		t.Fatalf("non-browser fetch = %d bytes, want verbatim %d-byte binary plist", rec.Body.Len(), len(raw))
	}
}

func TestImageViewerPage(t *testing.T) {
	root := t.TempDir()
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, image.NewRGBA(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pic.png"), pngBuf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}

	nav := httptest.NewRequest(http.MethodGet, "/pic.png", nil)
	nav.Header.Set("Sec-Fetch-Dest", "document")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, nav)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %.200s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`<img class="subject" src="pic.png?raw=1"`,
		"3 × 2",
		">image/png<",
		fmt.Sprintf("%d bytes", pngBuf.Len()),
		`href="pic.png?raw=1"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body = %q, want %q", body, want)
		}
	}

	// Subresource and non-browser fetches still get the raw image.
	img := httptest.NewRequest(http.MethodGet, "/pic.png", nil)
	img.Header.Set("Sec-Fetch-Dest", "image")
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, img)
	if !bytes.Equal(rec.Body.Bytes(), pngBuf.Bytes()) {
		t.Fatalf("image subresource fetch = %d bytes, want verbatim %d", rec.Body.Len(), pngBuf.Len())
	}
}

func TestDragAndDropUpload(t *testing.T) {
	server := fileServer{fsys: os.DirFS(t.TempDir()), pandoc: "unused"}

	src := "package main\n"
	post := httptest.NewRequest(http.MethodPost, "/_fb/drop?name="+url.QueryEscape("../sneaky/prog.go"), strings.NewReader(src))
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, post)
	if rec.Code != http.StatusOK {
		t.Fatalf("drop status = %d; body: %s", rec.Code, rec.Body.String())
	}
	href := rec.Body.String()
	if !strings.HasPrefix(href, "/_fb/drops/") || !strings.HasSuffix(href, "/prog.go") {
		t.Fatalf("drop href = %q, want sanitized path under /_fb/drops/", href)
	}

	nav := httptest.NewRequest(http.MethodGet, href, nil)
	nav.Header.Set("Sec-Fetch-Dest", "document")
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, nav)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `id="L1"`) {
		t.Fatalf("dropped file nav status = %d, want highlighted source; body: %.200s", rec.Code, rec.Body.String())
	}

	plain := httptest.NewRequest(http.MethodGet, href, nil)
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, plain)
	if got := rec.Body.String(); got != src {
		t.Fatalf("dropped file raw fetch = %q, want original bytes", got)
	}

	listing := httptest.NewRequest(http.MethodGet, "/", nil)
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, listing)
	if !strings.Contains(rec.Body.String(), "/_fb/drop?name=") {
		t.Fatalf("directory listing lacks the drag-and-drop script")
	}
	if !strings.Contains(rec.Body.String(), "/_fb/favicon.png") {
		t.Fatalf("directory listing lacks the favicon link")
	}

	big := httptest.NewRequest(http.MethodPost, "/_fb/drop?name=big.tar", bytes.NewReader(make([]byte, maxDropBytes+1)))
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, big)
	if rec.Code == http.StatusOK {
		t.Fatalf("oversized drop accepted, want rejection")
	}
}

// postCd sends a cd request for a dragged-in directory and returns the
// response recorder.
func postCd(t *testing.T, server fileServer, req cdRequest) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	post := httptest.NewRequest(http.MethodPost, "/_fb/cd", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, post)
	return rec
}

func TestDirectoryDropResolvesURI(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "my photos", "trip"), 0o755); err != nil {
		t.Fatal(err)
	}
	server := fileServer{fsys: os.DirFS(root), pandoc: "unused", dir: root}

	uri := (&url.URL{Scheme: "file", Path: filepath.Join(root, "my photos", "trip")}).String()
	rec := postCd(t, server, cdRequest{Name: "trip", URIs: []string{uri}})
	if rec.Code != http.StatusOK {
		t.Fatalf("cd status = %d; body: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); got != "/my%20photos/trip/" {
		t.Fatalf("cd href = %q, want /my%%20photos/trip/", got)
	}
}

func TestDirectoryDropRejectsURIOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	server := fileServer{fsys: os.DirFS(root), pandoc: "unused", dir: root}

	uri := (&url.URL{Scheme: "file", Path: outside}).String()
	rec := postCd(t, server, cdRequest{Name: filepath.Base(outside), URIs: []string{uri}})
	if rec.Code == http.StatusOK {
		t.Fatalf("cd resolved %q outside the root, want rejection; body: %s", outside, rec.Body.String())
	}
}

func TestDirectoryDropInferredFromChildren(t *testing.T) {
	// No URI, as Chrome and Firefox provide: the folder must be found by
	// name under the root (a temp dir, unindexed by Spotlight, so this
	// exercises the walk fallback) and verified by its child names.
	root := t.TempDir()
	target := filepath.Join(root, "deep", "nested", "notes")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.md", "b.md"} {
		if err := os.WriteFile(filepath.Join(target, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// A decoy with the same name but different contents must not match.
	decoy := filepath.Join(root, "other", "notes")
	if err := os.MkdirAll(decoy, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(decoy, "a.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := fileServer{fsys: os.DirFS(root), pandoc: "unused", dir: root}

	rec := postCd(t, server, cdRequest{Name: "notes", Children: []string{"a.md", "b.md"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("cd status = %d; body: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); got != "/deep/nested/notes/" {
		t.Fatalf("cd href = %q, want /deep/nested/notes/", got)
	}
}

func TestDirectoryDropAmbiguityBrokenByModTime(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "one", "notes")
	fresh := filepath.Join(root, "two", "notes")
	for _, dir := range []string{old, fresh} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	when := time.Now().Add(-time.Hour)
	if err := os.Chtimes(old, when, when); err != nil {
		t.Fatal(err)
	}
	server := fileServer{fsys: os.DirFS(root), pandoc: "unused", dir: root}

	// Identical fingerprints and no usable mtime: the drop must fail
	// loudly rather than guess between the two.
	rec := postCd(t, server, cdRequest{Name: "notes", Children: []string{"a.md"}})
	if rec.Code == http.StatusOK {
		t.Fatalf("ambiguous cd resolved to %q, want failure", rec.Body.String())
	}

	// An mtime matching exactly one candidate settles it.
	rec = postCd(t, server, cdRequest{Name: "notes", Children: []string{"a.md"}, Modified: when.UnixMilli()})
	if rec.Code != http.StatusOK {
		t.Fatalf("cd status = %d; body: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); got != "/one/notes/" {
		t.Fatalf("cd href = %q, want /one/notes/", got)
	}
}

func TestMatchingDirsPrefersPriorLocation(t *testing.T) {
	root := t.TempDir()
	desktop := filepath.Join(root, "Desktop")
	preferred := filepath.Join(desktop, "notes")
	elsewhere := filepath.Join(root, "tmp", "notes")
	for _, dir := range []string{preferred, elsewhere} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	req := cdRequest{Name: "notes", Children: []string{"a.md"}}
	got := matchingDirs([]string{elsewhere, preferred}, req, []string{desktop})
	if len(got) != 1 || got[0] != preferred {
		t.Fatalf("matchingDirs = %v, want just %q via the prior", got, preferred)
	}

	// A complete browser listing must match entry counts exactly: a folder
	// with extra entries is not the one that was dragged.
	if err := os.WriteFile(filepath.Join(preferred, "extra.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got = matchingDirs([]string{elsewhere, preferred}, req, []string{desktop})
	if len(got) != 1 || got[0] != elsewhere {
		t.Fatalf("matchingDirs = %v, want just %q after count mismatch", got, elsewhere)
	}
}

func TestWalkDirsSeedsProbedTogether(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "Users", "me")
	target := filepath.Join(home, "projects", "notes")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}

	// The home seed overlaps the root seed; the visited set must keep the
	// match from being reported twice.
	got := walkDirs(context.Background(), []string{home, root}, "notes", 3*time.Second)
	if len(got) != 1 || got[0] != target {
		t.Fatalf("walkDirs = %v, want just %q", got, target)
	}

	// A seed whose own basename matches is itself a find: the dragged
	// folder may be the Desktop (or the root) itself.
	got = walkDirs(context.Background(), []string{target}, "notes", 3*time.Second)
	if len(got) != 1 || got[0] != target {
		t.Fatalf("walkDirs seed self-match = %v, want just %q", got, target)
	}
}

func TestServesFavicon(t *testing.T) {
	server := fileServer{fsys: os.DirFS(t.TempDir()), pandoc: "unused"}

	req := httptest.NewRequest(http.MethodGet, "/_fb/favicon.png", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "image/png") {
		t.Fatalf("Content-Type = %q, want image/png", got)
	}
	if rec.Body.Len() == 0 {
		t.Fatal("empty favicon")
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=tester", "GIT_AUTHOR_EMAIL=tester@example.com",
		"GIT_COMMITTER_NAME=tester", "GIT_COMMITTER_EMAIL=tester@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestGitDirectoryShowsStats(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	runGit(t, root, "init", "-b", "main")
	runGit(t, root, "remote", "add", "origin", "ssh://example.com/repo.git")
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "a.txt")
	runGit(t, root, "commit", "-m", "first commit subject")
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "commit", "-am", "second commit subject")
	runGit(t, root, "tag", "v1")

	server := fileServer{fsys: os.DirFS(root), pandoc: "unused", dir: root}

	req := httptest.NewRequest(http.MethodGet, "/.git/", nil)
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"git repository",
		"branch main",
		"2 commits",
		"1 branch",
		"1 tag",
		"of objects",
		"origin: ssh://example.com/repo.git",
		"first commit subject",
		"second commit subject",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("git dir listing lacks %q", want)
		}
	}

	// The worktree itself is not a git directory; its listing stays plain.
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "text/html")
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if strings.Contains(rec.Body.String(), "git repository") {
		t.Error("plain directory listing unexpectedly has a git section")
	}
}

func TestPagesLinkHealthzAndVersion(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "code.go"), []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "data.csv"), []byte("a,b\n1,2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.md"), []byte("# hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := fileServer{fsys: os.DirFS(root), pandoc: fakePandoc(t), dir: root}

	for _, url := range []string{"/", "/code.go", "/data.csv"} {
		req := httptest.NewRequest(http.MethodGet, url, nil)
		req.Header.Set("Sec-Fetch-Dest", "document")
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want %d", url, rec.Code, http.StatusOK)
			continue
		}
		body := rec.Body.String()
		if !strings.Contains(body, "/_fb/healthz") || !strings.Contains(body, "/_fb/version") {
			t.Errorf("%s lacks the healthz/version footer", url)
		}
	}

	// Markdown pages render without the footer.
	req := httptest.NewRequest(http.MethodGet, "/notes.md", nil)
	req.Header.Set("Sec-Fetch-Dest", "document")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("/notes.md: status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); strings.Contains(body, "/_fb/healthz") || strings.Contains(body, "/_fb/version") {
		t.Error("/notes.md unexpectedly has the healthz/version footer")
	}
}

func TestHealthzEndpoint(t *testing.T) {
	server := fileServer{fsys: os.DirFS(t.TempDir()), pandoc: "unused"}

	req := httptest.NewRequest(http.MethodGet, "/_fb/healthz", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != "ok" {
		t.Fatalf("body = %q, want %q", got, "ok")
	}
}

func TestVersionEndpoint(t *testing.T) {
	server := fileServer{fsys: os.DirFS(t.TempDir()), pandoc: "unused"}

	req := httptest.NewRequest(http.MethodGet, "/_fb/version", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	// Test binaries carry no VCS stamp, so the revision reads "unknown"
	// here; the fields themselves must always be present.
	for _, want := range []string{"revision: ", "vcs.time: ", "modified: ", "go: go"} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("version output %q lacks %q", rec.Body.String(), want)
		}
	}
}

func TestDirectoryWithoutTrailingSlashRedirects(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}

	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}

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
	server := fileServer{fsys: os.DirFS(t.TempDir()), pandoc: "unused"}

	req := httptest.NewRequest(http.MethodGet, "/_fb/mathjax/tex-mml-chtml.js", nil)
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

	// No argument serves the home directory.
	for _, args := range [][]string{nil, {}} {
		root, err := parseRootArg(args)
		if err != nil {
			t.Fatal(err)
		}
		if root == "" || root != userHome() {
			t.Fatalf("parseRootArg(%q) = %q, want home directory %q", args, root, userHome())
		}
	}

	for _, args := range [][]string{
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
footer=""
format=""
standalone=0
toc=0
pagetitle=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -f) shift; format="$1" ;;
    --include-in-header) shift; header="$1" ;;
    --include-after-body) shift; footer="$1" ;;
    -s) standalone=1 ;;
    --toc) toc=1 ;;
    -V)
      shift
      case "$1" in
        pagetitle=*) pagetitle="${1#pagetitle=}" ;;
      esac
      ;;
  esac
  shift
done

case "$format" in
  markdown*)
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

if [ -z "$pagetitle" ]; then
  echo "missing -V pagetitle" >&2
  exit 5
fi

case "$format" in
  markdown*)
    if [ -n "$footer" ]; then
      echo "unexpected --include-after-body for markdown" >&2
      exit 6
    fi
    ;;
  *)
    if [ -z "$footer" ]; then
      echo "missing --include-after-body" >&2
      exit 6
    fi
    ;;
esac

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
printf '<p>rendered markdown</p><!--fmt:%s-->' "$format"
if [ -n "$footer" ]; then
  cat "$footer"
fi
printf '</body></html>'
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestErrorTokensNotRedBoxed(t *testing.T) {
	// plan9-style assembly confuses every assembler lexer; the error tokens
	// must render as plain text, not the github style's red boxes.
	out, err := highlightSource("x.s", "DATA ·AVX2_iv0<>+0x00(SB)/8, $0x6a09e667f3bcc908\n")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "#82071e") {
		t.Fatalf("output = %q, want no error-token background", out)
	}
}

func TestTIFFPreviewConverted(t *testing.T) {
	if _, err := exec.LookPath("sips"); err != nil {
		t.Skip("sips not available")
	}

	root := t.TempDir()
	var buf bytes.Buffer
	if err := tiff.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 20, 10)), nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scan.tiff"), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	server := fileServer{fsys: os.DirFS(root), pandoc: "unused", dir: root}

	nav := httptest.NewRequest(http.MethodGet, "/scan.tiff", nil)
	nav.Header.Set("Sec-Fetch-Dest", "document")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, nav)
	body := rec.Body.String()
	for _, want := range []string{`src="scan.tiff?jpeg=1"`, "20 × 10", ">image/tiff<", "converted to JPEG with sips"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body = %q, want %q", body, want)
		}
	}

	preview := httptest.NewRequest(http.MethodGet, "/scan.tiff?jpeg=1", nil)
	preview.Header.Set("Sec-Fetch-Dest", "image")
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, preview)
	if rec.Code != http.StatusOK || !bytes.HasPrefix(rec.Body.Bytes(), []byte{0xFF, 0xD8}) {
		t.Fatalf("preview status = %d, want JPEG bytes", rec.Code)
	}
}

func TestManifestHighlighted(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "MANIFEST.MF"),
		[]byte("Manifest-Version: 1.0\nMain-Class: org.example.App\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}

	req := httptest.NewRequest(http.MethodGet, "/MANIFEST.MF", nil)
	req.Header.Set("Sec-Fetch-Dest", "document")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Fatalf("Content-Type = %q, want highlighted text/html", got)
	}
	if !strings.Contains(rec.Body.String(), "Manifest-Version") {
		t.Fatalf("body lacks manifest content")
	}
}

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
}

// listingRow fetches a directory listing and returns the <tr> line for one
// entry name, failing if it is absent.
func listingRow(t *testing.T, server fileServer, dir, name string) string {
	t.Helper()
	body := fetchListing(t, server, dir)
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, `data-name="`+name+`"`) {
			return line
		}
	}
	t.Fatalf("listing %s has no row for %q:\n%s", dir, name, body)
	return ""
}

func fetchListing(t *testing.T, server fileServer, dir string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, dir, nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200", dir, rec.Code)
	}
	return rec.Body.String()
}

func TestGitWorktreeAnnotations(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	writeAll := func(files map[string]string) {
		t.Helper()
		for name, content := range files {
			p := filepath.Join(root, name)
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	writeAll(map[string]string{
		".gitignore":    "ignored.txt\nbuild/\n",
		"tracked.txt":   "one\n",
		"sub/insub.txt": "one\n",
	})
	runGit(t, root, "init", "-b", "main")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "init")
	writeAll(map[string]string{
		"tracked.txt":   "changed\n", // modified, not staged
		"staged.txt":    "new\n",     // staged below
		"loose.txt":     "loose\n",   // untracked
		"ignored.txt":   "art\n",     // gitignored file
		"build/out.bin": "obj\n",     // gitignored directory
		"sub/insub.txt": "changed\n", // modified inside tracked subdir
		"untr/a.txt":    "a\n",       // wholly untracked directory
	})
	runGit(t, root, "add", "staged.txt")

	server := fileServer{fsys: os.DirFS(root), pandoc: "unused", dir: root}

	body := fetchListing(t, server, "/")
	for _, want := range []string{
		"branch main", "no upstream",
		`<span class="bad">2 modified</span>`,
		`<span class="bad">1 staged</span>`,
		`<span class="bad">2 untracked</span>`, // loose.txt and untr/
	} {
		if !strings.Contains(body, want) {
			t.Errorf("root listing lacks %q", want)
		}
	}

	for name, want := range map[string][]string{
		"tracked.txt": {`class="gitb modified"`, `>M</span>`, `title="modified — not staged"`},
		"staged.txt":  {`class="gitb staged"`, `>A</span>`, `title="new file — staged for commit"`},
		"loose.txt":   {`class="gitb untracked"`, `>?</span>`},
		"ignored.txt": {`class="gitb ignored"`, `>i</span>`, `title="gitignored"`},
		"build/":      {`class="gitb ignored"`, `>i</span>`, `title="gitignored"`},
		"untr/":       {`class="gitb untracked"`, `>?</span>`},
		"sub/":        {`class="gitb modified"`, `>●</span>`, `title="1 modified within"`},
	} {
		row := listingRow(t, server, "/", name)
		for _, w := range want {
			if !strings.Contains(row, w) {
				t.Errorf("row for %s = %s, want %q", name, row, w)
			}
		}
	}
	// A clean tracked file carries no annotation at all.
	runGit(t, root, "checkout", "--", "tracked.txt")
	if row := listingRow(t, server, "/", "tracked.txt"); strings.Contains(row, "gitb") {
		t.Errorf("clean file should be unannotated: %s", row)
	}

	// Inside a wholly untracked directory every entry is untracked.
	if row := listingRow(t, server, "/untr/", "a.txt"); !strings.Contains(row, `class="gitb untracked"`) {
		t.Errorf("file in untracked dir should be marked untracked: %s", row)
	}
	// Inside a gitignored directory every entry is badged ignored.
	if row := listingRow(t, server, "/build/", "out.bin"); !strings.Contains(row, `class="gitb ignored"`) {
		t.Errorf("file in ignored dir should be marked ignored: %s", row)
	}
	// Inside .git itself, no worktree annotations (the repository section
	// already covers it).
	if body := fetchListing(t, server, "/.git/"); strings.Contains(body, `<p class="gitline">`) || strings.Contains(body, `<span class="gitb`) {
		t.Errorf(".git listing should have no worktree annotations")
	}
}

func TestGitRootLineSyncWithOrigin(t *testing.T) {
	requireGit(t)
	origin := t.TempDir()
	runGit(t, origin, "init", "--bare")
	root := t.TempDir()
	runGit(t, root, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(root, "f.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "f.txt")
	runGit(t, root, "commit", "-m", "one")
	runGit(t, root, "remote", "add", "origin", origin)
	runGit(t, root, "push", "-q", "-u", "origin", "main")

	server := fileServer{fsys: os.DirFS(root), pandoc: "unused", dir: root}

	body := fetchListing(t, server, "/")
	for _, want := range []string{"in sync with origin/main", "clean"} {
		if !strings.Contains(body, want) {
			t.Errorf("clean synced root lacks %q; body: %s", want, body)
		}
	}

	runGit(t, root, "commit", "--allow-empty", "-m", "two")
	if body := fetchListing(t, server, "/"); !strings.Contains(body, `<span class="bad">ahead 1 of origin/main</span>`) {
		t.Errorf("root line lacks ahead marker; body: %s", body)
	}
}

func TestNonRepoListingHasNoGitLine(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "f.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := fileServer{fsys: os.DirFS(root), pandoc: "unused", dir: root}
	if body := fetchListing(t, server, "/"); strings.Contains(body, `<p class="gitline">`) || strings.Contains(body, `<span class="gitb`) {
		t.Errorf("non-repo listing should have no git annotations")
	}
}

func TestGitDeletedFilesGetGhostRowsAndSubdirHeaders(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	for name, content := range map[string]string{
		"sub/keep.txt":  "keep\n",
		"sub/gone.txt":  "bye\n",
		"gonedir/a.txt": "a\n",
		"gonedir/b.txt": "b\n",
		"staged.txt":    "s\n",
	} {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGit(t, root, "init", "-b", "main")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "init")
	if err := os.WriteFile(filepath.Join(root, "sub/keep.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "sub/gone.txt")); err != nil { // unstaged deletion
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, "gonedir")); err != nil { // whole tracked dir gone
		t.Fatal(err)
	}
	runGit(t, root, "rm", "-q", "staged.txt") // staged deletion

	server := fileServer{fsys: os.DirFS(root), pandoc: "unused", dir: root}

	// The subdirectory gets its own scoped header and a ghost row for the
	// deleted file.
	body := fetchListing(t, server, "/sub/")
	for _, want := range []string{
		`<p class="gitline">git: <span class="bad">1 modified</span><span class="sep"> &middot; </span><span class="bad">1 deleted</span></p>`,
		`<span class="gitghost" title="gone.txt — deleted — not staged">gone.txt</span>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("sub listing lacks %q; body: %s", want, body)
		}
	}

	// The root counts everything below, ghosts the staged deletion, and
	// ghosts the vanished directory with its counts on hover.
	body = fetchListing(t, server, "/")
	for _, want := range []string{
		"1 modified", "3 deleted", "branch main",
		`<span class="gitghost" title="staged.txt — deleted — staged for commit">staged.txt</span>`,
		`<span class="gitghost" title="gonedir/ — 2 deleted within">gonedir/</span>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("root listing lacks %q; body: %s", want, body)
		}
	}
	if strings.Contains(body, `data-name="gone.txt"`) {
		t.Errorf("sub/gone.txt should not appear at the root, only in sub/")
	}

	// A clean subtree stays headerless: revert everything and check.
	runGit(t, root, "checkout", "--", ".")
	runGit(t, root, "reset", "-q", "--hard", "HEAD")
	if body := fetchListing(t, server, "/sub/"); strings.Contains(body, `<p class="gitline">`) {
		t.Errorf("clean subdir should have no git line")
	}
	if body := fetchListing(t, server, "/"); !strings.Contains(body, "clean") {
		t.Errorf("clean root should say clean")
	}
}
