package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"database/sql"
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
	for _, name := range []string{".localmd.css", "sub/.localmd.css"} {
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
		"1 row", // the cities sheet has one data row
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

	csvReq := httptest.NewRequest(http.MethodGet, "/data.xlsx/cities?raw=1", nil)
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, csvReq)
	if got := rec.Body.String(); got != "city,population\nnew orleans,364136\n" {
		t.Fatalf("sheet csv = %q, want csv export", got)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/csv") {
		t.Fatalf("sheet csv Content-Type = %q, want text/csv", got)
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
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE users (name TEXT, email TEXT, age INTEGER);
		INSERT INTO users VALUES ('mike', 'mra@xoba.com', 55), ('nobody', NULL, NULL);`); err != nil {
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
		`name="q"`,           // the query form
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
		"<td>mike</td><td>mra@xoba.com</td><td>55</td>",
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
	if got := rec.Body.String(); got != "name,email,age\nmike,mra@xoba.com,55\nnobody,,\n" {
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
	db, err := sql.Open("sqlite3", dbPath)
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
	for _, want := range []string{`id="statprog"`, `data-name="t"`, "computing table stats"} {
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
	if got := rec.Body.String(); !strings.Contains(got, "1 row × 1 column") {
		t.Fatalf("stat body = %q, want row and column counts", got)
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
		">png<",
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
	post := httptest.NewRequest(http.MethodPost, "/_localmd/drop?name="+url.QueryEscape("../sneaky/prog.go"), strings.NewReader(src))
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, post)
	if rec.Code != http.StatusOK {
		t.Fatalf("drop status = %d; body: %s", rec.Code, rec.Body.String())
	}
	href := rec.Body.String()
	if !strings.HasPrefix(href, "/_localmd/drops/") || !strings.HasSuffix(href, "/prog.go") {
		t.Fatalf("drop href = %q, want sanitized path under /_localmd/drops/", href)
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
	if !strings.Contains(rec.Body.String(), "/_localmd/drop?name=") {
		t.Fatalf("directory listing lacks the drag-and-drop script")
	}
	if !strings.Contains(rec.Body.String(), "/_localmd/favicon.png") {
		t.Fatalf("directory listing lacks the favicon link")
	}

	big := httptest.NewRequest(http.MethodPost, "/_localmd/drop?name=big.tar", bytes.NewReader(make([]byte, maxDropBytes+1)))
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, big)
	if rec.Code == http.StatusOK {
		t.Fatalf("oversized drop accepted, want rejection")
	}
}

func TestServesFavicon(t *testing.T) {
	server := fileServer{fsys: os.DirFS(t.TempDir()), pandoc: "unused"}

	req := httptest.NewRequest(http.MethodGet, "/_localmd/favicon.png", nil)
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
pagetitle=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -f) shift; format="$1" ;;
    --include-in-header) shift; header="$1" ;;
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
printf '<p>rendered markdown</p><!--fmt:%s--></body></html>' "$format"
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}
