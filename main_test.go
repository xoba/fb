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
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--include-in-header" ]; then
    shift
    header="$1"
    break
  fi
  shift
done

if [ -z "$header" ]; then
  echo "missing --include-in-header" >&2
  exit 2
fi

if ! grep -q "color: #000" "$header"; then
  echo "missing readability CSS" >&2
  exit 3
fi

printf '<!doctype html><html><body><p>rendered markdown</p></body></html>'
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}
