package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/csv"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing/fstest"
	"time"
	"unicode/utf8"

	"github.com/alecthomas/chroma/v2"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	_ "github.com/mattn/go-sqlite3"
	"github.com/xuri/excelize/v2"
)

const (
	listenAddr4 = "127.0.0.1:3030"
	listenAddr6 = "[::1]:3030"

	// assetPrefix is a reserved URL namespace for resources embedded in the
	// binary (MathJax and its fonts). Filesystem paths under it are shadowed.
	assetPrefix = "_localmd"

	// localCSSName, when present in a served markdown file's directory or any
	// ancestor up to the serve root, is linked into the rendered page. Outer
	// directories are linked first so the nearest file wins the cascade.
	localCSSName = ".localmd.css"
)

//go:embed assets/mathjax
var embeddedAssets embed.FS

func main() {
	rootArg, err := parseRootArg(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Usage: %s SERVE_PATH\n", filepath.Base(os.Args[0]))
		os.Exit(2)
	}

	root, err := resolveRoot(rootArg)
	if err != nil {
		log.Fatal(err)
	}

	if _, err := exec.LookPath("pandoc"); err != nil {
		log.Fatal("pandoc is required but was not found in PATH")
	}

	fsys := os.DirFS(root)
	handler := fileServer{fsys: fsys, pandoc: "pandoc"}

	log.Printf("serving %s at http://localhost:3030/ (%s and %s)", root, listenAddr4, listenAddr6)
	if err := serveLoopback(handler); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func serveLoopback(handler http.Handler) error {
	listeners := make([]net.Listener, 0, 2)
	for _, spec := range []struct {
		network string
		addr    string
	}{
		{network: "tcp4", addr: listenAddr4},
		{network: "tcp6", addr: listenAddr6},
	} {
		ln, err := net.Listen(spec.network, spec.addr)
		if err != nil {
			for _, open := range listeners {
				_ = open.Close()
			}
			return fmt.Errorf("listen %s: %w", spec.addr, err)
		}
		listeners = append(listeners, ln)
	}

	errc := make(chan error, len(listeners))
	for _, ln := range listeners {
		srv := &http.Server{Handler: handler}
		go func() {
			errc <- srv.Serve(ln)
		}()
	}

	return <-errc
}

func parseRootArg(args []string) (string, error) {
	if len(args) != 1 || args[0] == "-h" || args[0] == "--help" {
		return "", errors.New("expected one serve path")
	}
	return args[0], nil
}

func resolveRoot(root string) (string, error) {
	if root == "" {
		return "", errors.New("serve path is required")
	}

	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", abs)
	}

	return abs, nil
}

type fileServer struct {
	fsys   fs.FS
	pandoc string

	// base is the URL path prefix at which fsys is mounted: "" for the serve
	// root, or the archive's own URL path (e.g. "/notes/bundle.zip") when
	// fsys is the contents of a zip file.
	base string
}

func (s fileServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := requestName(r.URL.Path)

	if name == assetPrefix || strings.HasPrefix(name, assetPrefix+"/") {
		serveAsset(w, r, name)
		return
	}

	preventCaching(w.Header(), r.Header)

	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.route(w, r, name, 0)
}

// route dispatches a request for name within s.fsys, descending into zip
// archives as if they were directories.
func (s fileServer) route(w http.ResponseWriter, r *http.Request, name string, depth int) {
	info, err := fs.Stat(s.fsys, name)
	if err != nil {
		s.routeIntoArchive(w, r, name, depth)
		return
	}

	if info.IsDir() {
		s.serveDirectory(w, r, name)
		return
	}

	ext := strings.ToLower(path.Ext(name))
	browserView := r.URL.Query().Get("raw") != "1" && wantsDocument(r.Header)

	// Markdown renders for every client (the server's original purpose);
	// other pandoc-supported documents render only on browser navigation so
	// curl and scripts still get the original bytes.
	if format, ok := pandocFormats[ext]; ok && (ext == ".md" || browserView) {
		s.serveDocument(w, r, name, info, format)
		return
	}

	if browserView {
		switch {
		case tableDelims[ext] != 0:
			s.serveTable(w, r, name, info)
			return
		case ext == ".xlsx":
			s.serveXLSX(w, r, name, info)
			return
		case ext == ".sqlite" || ext == ".sqlite3" || ext == ".db":
			s.serveSQLite(w, r, name, info)
			return
		case highlightable(name):
			s.serveSource(w, r, name, info)
			return
		case isArchiveName(name):
			s.descendArchive(w, r, name, info, ".", depth)
			return
		}
	}

	s.serveRaw(w, r, name, info)
}

// routeIntoArchive handles paths that do not exist directly in s.fsys by
// checking whether some prefix of them is a zip file to descend into.
func (s fileServer) routeIntoArchive(w http.ResponseWriter, r *http.Request, name string, depth int) {
	elems := strings.Split(name, "/")
	for i := range elems[:len(elems)-1] {
		prefix := path.Join(elems[:i+1]...)
		info, err := fs.Stat(s.fsys, prefix)
		if err != nil {
			break
		}
		if info.IsDir() {
			continue
		}
		if isArchiveName(prefix) {
			s.descendArchive(w, r, prefix, info, path.Join(elems[i+1:]...), depth)
			return
		}
		break
	}
	http.NotFound(w, r)
}

const (
	// maxArchiveBytes caps how much of a non-seekable file (an archive member,
	// including a nested archive) is buffered in memory for serving.
	maxArchiveBytes = 128 << 20

	// maxArchiveDepth bounds zip-within-zip nesting.
	maxArchiveDepth = 3
)

func isArchiveName(name string) bool {
	return strings.EqualFold(path.Ext(name), ".zip") || isTarName(name)
}

func isTarName(name string) bool {
	l := strings.ToLower(name)
	return strings.HasSuffix(l, ".tar") || strings.HasSuffix(l, ".tar.gz") ||
		strings.HasSuffix(l, ".tgz") || strings.HasSuffix(l, ".tar.bz2")
}

// descendArchive serves inner (an fs path within the named archive) by
// mounting the archive as an fs.FS and routing through the ordinary pipeline,
// so members get listings, highlighting, and markdown rendering like any
// other file. An unreadable or oversized archive falls back to verbatim
// serving.
func (s fileServer) descendArchive(w http.ResponseWriter, r *http.Request, arcName string, info fs.FileInfo, inner string, depth int) {
	if depth >= maxArchiveDepth {
		http.NotFound(w, r)
		return
	}

	f, err := s.fsys.Open(arcName)
	if err != nil {
		http.Error(w, "cannot read archive", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	var arc fs.FS
	if isTarName(arcName) {
		arc, err = tarFS(arcName, f, info)
	} else {
		arc, err = zipFS(f, info)
	}
	if err != nil {
		log.Printf("open archive %s: %v", arcName, err)
		s.serveRaw(w, r, arcName, info)
		return
	}

	sub := fileServer{fsys: arc, pandoc: s.pandoc, base: s.base + "/" + arcName}
	sub.route(w, r, inner, depth+1)
}

// zipFS mounts a zip file lazily via its central directory; the caller must
// keep f open while the returned fs.FS is in use.
func zipFS(f fs.File, info fs.FileInfo) (fs.FS, error) {
	ra, ok := f.(io.ReaderAt)
	size := info.Size()
	if !ok {
		if size > maxArchiveBytes {
			return nil, fmt.Errorf("nested archive too large (%d bytes)", size)
		}
		data, err := io.ReadAll(f)
		if err != nil {
			return nil, err
		}
		ra, size = bytes.NewReader(data), int64(len(data))
	}
	return zip.NewReader(ra, size)
}

const (
	// maxTarFileBytes gates tar browsing on the raw file size as stored
	// (compressed or not); larger archives are served verbatim. Tars have no
	// index, so unlike zips they are read in full up front.
	maxTarFileBytes = 100 << 20

	// maxTarExtractedBytes is a safety ceiling on the decompressed content
	// held in memory, so a pathological compression bomb cannot exhaust it.
	maxTarExtractedBytes = 1 << 30
)

// tarFS reads an entire (possibly gzip- or bzip2-compressed) tar archive
// into an in-memory filesystem.
func tarFS(name string, f fs.File, info fs.FileInfo) (fs.FS, error) {
	if info.Size() > maxTarFileBytes {
		return nil, fmt.Errorf("tar too large (%d bytes)", info.Size())
	}

	var r io.Reader = f
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".gz") || strings.HasSuffix(lower, ".tgz"):
		gz, err := gzip.NewReader(f)
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		r = gz
	case strings.HasSuffix(lower, ".bz2"):
		r = bzip2.NewReader(f)
	}

	mfs := fstest.MapFS{}
	tr := tar.NewReader(r)
	var total int64
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		member := path.Clean(strings.TrimPrefix(hdr.Name, "/"))
		if member == "." || !fs.ValidPath(member) {
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			mfs[member] = &fstest.MapFile{Mode: fs.ModeDir | 0o755, ModTime: hdr.ModTime}
		case tar.TypeReg:
			data, err := io.ReadAll(io.LimitReader(tr, maxTarExtractedBytes-total+1))
			if err != nil {
				return nil, err
			}
			total += int64(len(data))
			if total > maxTarExtractedBytes {
				return nil, fmt.Errorf("tar contents exceed %d bytes", int64(maxTarExtractedBytes))
			}
			mfs[member] = &fstest.MapFile{Data: data, Mode: 0o644, ModTime: hdr.ModTime}
		}
	}
	return mfs, nil
}

// serveRaw serves a file verbatim. Unlike http.ServeFileFS it neither
// redirects index.html requests back to their directory nor requires a
// seekable file (zip members are not); non-seekable content is buffered.
func (s fileServer) serveRaw(w http.ResponseWriter, r *http.Request, name string, info fs.FileInfo) {
	f, err := s.fsys.Open(name)
	if err != nil {
		http.Error(w, "cannot read file", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	if rs, ok := f.(io.ReadSeeker); ok {
		http.ServeContent(w, r, path.Base(name), info.ModTime(), rs)
		return
	}

	data, err := io.ReadAll(io.LimitReader(f, maxArchiveBytes+1))
	if err != nil {
		http.Error(w, "cannot read file", http.StatusInternalServerError)
		return
	}
	if int64(len(data)) > maxArchiveBytes {
		http.Error(w, "archive member too large", http.StatusRequestEntityTooLarge)
		return
	}
	http.ServeContent(w, r, path.Base(name), info.ModTime(), bytes.NewReader(data))
}

// wantsDocument reports whether a request is a browser navigation, as opposed
// to a subresource fetch (<link rel="stylesheet">, <script>, ...) or a
// non-browser client like curl. Only navigations get highlighted HTML; other
// fetches need the file verbatim.
func wantsDocument(h http.Header) bool {
	if dest := h.Get("Sec-Fetch-Dest"); dest != "" {
		return dest == "document"
	}
	return strings.Contains(h.Get("Accept"), "text/html")
}

// serveAsset serves resources compiled into the binary. They change only when
// the binary does, so unlike everything else they are allowed to be cached.
func serveAsset(w http.ResponseWriter, r *http.Request, name string) {
	w.Header().Set("Cache-Control", "max-age=86400")
	http.ServeFileFS(w, r, embeddedAssets, path.Join("assets", strings.TrimPrefix(name, assetPrefix+"/")))
}

func preventCaching(response, request http.Header) {
	response.Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	response.Set("Pragma", "no-cache")
	response.Set("Expires", "0")

	request.Del("If-Modified-Since")
	request.Del("If-None-Match")
	request.Del("If-Match")
	request.Del("If-Unmodified-Since")
	request.Del("If-Range")
}

func requestName(urlPath string) string {
	name := strings.TrimPrefix(path.Clean("/"+urlPath), "/")
	if name == "" {
		return "."
	}
	return name
}

// pandocFormats maps file extensions to the pandoc input format used to
// render them as HTML.
var pandocFormats = map[string]string{
	".md":    "markdown+footnotes+lists_without_preceding_blankline+tex_math_single_backslash+gfm_auto_identifiers+autolink_bare_uris+emoji",
	".ipynb": "ipynb",
	".docx":  "docx",
	".odt":   "odt",
	".rtf":   "rtf",
	".epub":  "epub",
}

func (s fileServer) serveDocument(w http.ResponseWriter, r *http.Request, name string, info fs.FileInfo, format string) {
	html, err := s.renderDocument(r.Context(), name, renderOptions{
		format:     format,
		standalone: true,
		toc:        r.URL.Query().Get("toc") == "1",
	})
	if err != nil {
		log.Printf("pandoc failed for %s: %v", name, err)
		if format != pandocFormats[".md"] {
			s.serveRaw(w, r, name, info)
			return
		}
		http.Error(w, "pandoc failed", http.StatusInternalServerError)
		return
	}

	outName := strings.TrimSuffix(path.Base(name), path.Ext(name)) + ".html"
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeContent(w, r, outName, info.ModTime(), bytes.NewReader(html))
}

type renderOptions struct {
	// format is the pandoc input format; empty means markdown.
	format string
	// standalone produces a complete styled document; otherwise pandoc emits a
	// body fragment for embedding (used for README previews in listings).
	standalone bool
	toc        bool
}

func (s fileServer) renderDocument(ctx context.Context, name string, opts renderOptions) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Feed the source through stdin rather than as a file path so documents
	// work from any fs.FS, including the inside of archives.
	content, err := fs.ReadFile(s.fsys, name)
	if err != nil {
		return nil, err
	}

	format := opts.format
	if format == "" {
		format = pandocFormats[".md"]
	}

	args := []string{
		"-f", format,
		"--mathjax=/" + assetPrefix + "/mathjax/tex-mml-chtml.js",
		"--reference-location=section",
	}

	if format != pandocFormats[".md"] {
		// Binary formats carry their images internally (pandoc's media bag);
		// embed them as data URIs so they survive as a single HTML page.
		args = append(args, "--embed-resources")
	}

	if opts.toc {
		args = append(args, "--toc", "--toc-depth=3")
	}

	if opts.standalone {
		header, cleanup, err := writePandocHeader(s.stylesheetLinks(name))
		if err != nil {
			return nil, err
		}
		defer cleanup()

		args = append(args,
			"-s",
			// Reading from stdin leaves pandoc no filename to derive <title>
			// from (it would fall back to "-"); pagetitle sets the title
			// element without adding a visible title block.
			"-V", "pagetitle="+path.Base(name),
			"--include-in-header", header,
			"-V", `mainfont=-apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif`,
			"-V", "monofont=ui-monospace, SFMono-Regular, Menlo, Consolas, monospace",
			"-V", "linkcolor=#0969da",
			"-V", "monobackgroundcolor=#f6f8fa",
			"-V", "maxwidth=42em",
		)
	}

	cmd := exec.CommandContext(ctx, s.pandoc, args...)
	cmd.Stdin = bytes.NewReader(content)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("%s: %w", msg, err)
	}

	return out, nil
}

// stylesheetLinks returns URL paths of every .localmd.css found between the
// filesystem root and the markdown file's directory, outermost first.
func (s fileServer) stylesheetLinks(name string) []string {
	var dirs []string
	for dir := path.Dir(name); dir != "."; dir = path.Dir(dir) {
		dirs = append(dirs, dir)
	}
	dirs = append(dirs, ".")

	var hrefs []string
	for i := len(dirs) - 1; i >= 0; i-- {
		cssPath := path.Join(dirs[i], localCSSName)
		if info, err := fs.Stat(s.fsys, cssPath); err == nil && !info.IsDir() {
			hrefs = append(hrefs, (&url.URL{Path: s.base + "/" + cssPath}).String())
		}
	}
	return hrefs
}

// writePandocHeader writes the shared header include, followed by stylesheet
// links for any .localmd.css files so they can override the built-in styles.
// (pandoc's --css links land before header-includes, which would invert the
// cascade — hence emitting the link tags here instead.)
func writePandocHeader(stylesheets []string) (string, func(), error) {
	f, err := os.CreateTemp("", "localmd-pandoc-header-*.html")
	if err != nil {
		return "", func() {}, err
	}

	cleanup := func() {
		if err := os.Remove(f.Name()); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Printf("remove temp pandoc header %s: %v", f.Name(), err)
		}
	}

	var content strings.Builder
	content.WriteString(pandocHeader)
	for _, href := range stylesheets {
		fmt.Fprintf(&content, "<link rel=\"stylesheet\" href=\"%s\">\n", href)
	}

	if _, err := f.WriteString(content.String()); err != nil {
		name := f.Name()
		_ = f.Close()
		cleanup()
		return "", func() {}, fmt.Errorf("write %s: %w", name, err)
	}
	if err := f.Close(); err != nil {
		cleanup()
		return "", func() {}, err
	}

	return f.Name(), cleanup, nil
}

// highlightExts lists source-file extensions served as syntax-highlighted HTML
// pages when a browser navigates to them. Subresource fetches and non-browser
// clients (see wantsDocument), plus anything requested with ?raw=1, get the
// file verbatim — so .localmd.css and stylesheets referenced by served HTML
// keep working as real stylesheets. Deliberately absent: .md (pandoc), .html
// and .svg (the browser renders those), and .txt (prose reads better plain).
var highlightExts = map[string]bool{
	".awk":     true,
	".bash":    true,
	".bat":     true,
	".c":       true,
	".cc":      true,
	".clj":     true,
	".cpp":     true,
	".cs":      true,
	".css":     true,
	".dart":    true,
	".diff":    true,
	".el":      true,
	".erl":     true,
	".ex":      true,
	".exs":     true,
	".fish":    true,
	".go":      true,
	".gradle":  true,
	".graphql": true,
	".groovy":  true,
	".h":       true,
	".hcl":     true,
	".hpp":     true,
	".hs":      true,
	".ini":     true,
	".java":    true,
	".jl":      true,
	".js":      true,
	".json":    true,
	".jsx":     true,
	".kt":      true,
	".lisp":    true,
	".lua":     true,
	".mjs":     true,
	".nix":     true,
	".patch":   true,
	".php":     true,
	".pl":      true,
	".proto":   true,
	".ps1":     true,
	".py":      true,
	".r":       true,
	".rb":      true,
	".rs":      true,
	".scala":   true,
	".scss":    true,
	".sh":      true,
	".sql":     true,
	".svelte":  true,
	".swift":   true,
	".tex":     true,
	".tf":      true,
	".toml":    true,
	".ts":      true,
	".tsx":     true,
	".vue":     true,
	".xml":     true,
	".yaml":    true,
	".yml":     true,
	".zig":     true,
	".zsh":     true,
}

// highlightNames lists exact basenames without a useful extension that also
// get the highlighted treatment. (go.mod is deliberately absent: chroma
// mis-matches *.mod to its AMPL lexer.)
var highlightNames = map[string]bool{
	".bashrc":        true,
	".zshrc":         true,
	"CMakeLists.txt": true,
	"Dockerfile":     true,
	"GNUmakefile":    true,
	"Makefile":       true,
	"makefile":       true,
}

func highlightable(name string) bool {
	base := path.Base(name)
	return highlightNames[base] || highlightExts[strings.ToLower(path.Ext(base))]
}

// maxHighlightBytes caps how large a file gets the highlighted treatment;
// anything bigger is served verbatim rather than ballooning into huge HTML.
const maxHighlightBytes = 2 << 20

func (s fileServer) serveSource(w http.ResponseWriter, r *http.Request, name string, info fs.FileInfo) {
	if info.Size() > maxHighlightBytes {
		s.serveRaw(w, r, name, info)
		return
	}

	src, err := fs.ReadFile(s.fsys, name)
	if err != nil {
		http.Error(w, "cannot read file", http.StatusInternalServerError)
		return
	}

	code, err := highlightSource(path.Base(name), string(src))
	if err != nil {
		log.Printf("highlight failed for %s: %v", name, err)
		s.serveRaw(w, r, name, info)
		return
	}

	page := sourcePage{
		Title:   path.Base(name),
		Crumbs:  s.breadcrumbs(path.Dir(name)),
		RawHref: (&url.URL{Path: path.Base(name), RawQuery: "raw=1"}).String(),
		Code:    code,
	}

	var buf bytes.Buffer
	if err := sourceTemplate.Execute(&buf, page); err != nil {
		log.Printf("render source %s: %v", name, err)
		http.Error(w, "cannot render source", http.StatusInternalServerError)
		return
	}

	outName := strings.TrimSuffix(path.Base(name), path.Ext(name)) + ".html"
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeContent(w, r, outName, info.ModTime(), bytes.NewReader(buf.Bytes()))
}

func highlightSource(filename, src string) (template.HTML, error) {
	lexer := lexers.Match(filename)
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)

	iterator, err := lexer.Tokenise(nil, src)
	if err != nil {
		return "", err
	}

	formatter := chromahtml.New(
		chromahtml.WithLineNumbers(true),
		chromahtml.WithLinkableLineNumbers(true, "L"),
		chromahtml.TabWidth(4),
	)

	var buf bytes.Buffer
	if err := formatter.Format(&buf, styles.Get("github"), iterator); err != nil {
		return "", err
	}
	return template.HTML(buf.String()), nil
}

type sourcePage struct {
	Title   string
	Crumbs  []crumb
	RawHref string
	Code    template.HTML
}

var sourceTemplate = template.Must(template.New("source").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<link rel="icon" href="data:,">
<title>{{.Title}}</title>
<style>
  :root { color-scheme: light; }
  html { color: #1a1a1a; background-color: #fdfdfd; }
  body {
    margin: 0 auto;
    max-width: 70em;
    padding: 50px;
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif;
  }
  @media (max-width: 600px) { body { padding: 12px; } }
  a { color: #0969da; text-decoration: none; }
  a:hover { text-decoration: underline; }
  nav { margin-bottom: 1rem; font-size: 1.1rem; }
  nav a { font-weight: 600; }
  nav span.sep { color: #57606a; }
  nav span.file { font-weight: 600; }
  nav a.raw { float: right; font-weight: 400; font-size: 0.85rem; }
  div.source {
    border: 1px solid #d0d7de;
    border-radius: 6px;
    overflow-x: auto;
    background-color: #fff;
  }
  div.source pre {
    margin: 0;
    padding: 0.75rem 0;
    font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
    font-size: 0.85rem;
    line-height: 1.45;
  }
</style>
</head>
<body>
<nav>{{range $i, $c := .Crumbs}}{{if gt $i 1}}<span class="sep">/</span>{{end}}<a href="{{$c.Href}}">{{$c.Name}}</a>{{end}}{{if gt (len .Crumbs) 1}}<span class="sep">/</span>{{end}}<span class="file">{{.Title}}</span><a class="raw" href="{{.RawHref}}">raw</a></nav>
<div class="source">
{{.Code}}
</div>
</body>
</html>
`))

// tableDelims maps delimited-text extensions rendered as HTML tables to
// their field separator.
var tableDelims = map[string]rune{
	".csv": ',',
	".tsv": '\t',
}

// maxTableRows caps how many data rows a table view shows; the remainder is
// summarized as a count.
const maxTableRows = 2000

func (s fileServer) serveTable(w http.ResponseWriter, r *http.Request, name string, info fs.FileInfo) {
	if info.Size() > maxHighlightBytes {
		s.serveRaw(w, r, name, info)
		return
	}

	src, err := fs.ReadFile(s.fsys, name)
	if err != nil {
		http.Error(w, "cannot read file", http.StatusInternalServerError)
		return
	}

	reader := csv.NewReader(bytes.NewReader(src))
	reader.Comma = tableDelims[strings.ToLower(path.Ext(name))]
	reader.FieldsPerRecord = -1 // tolerate ragged rows
	reader.LazyQuotes = true

	records, err := reader.ReadAll()
	if err != nil || len(records) == 0 {
		if err != nil {
			log.Printf("parse %s: %v", name, err)
		}
		s.serveRaw(w, r, name, info)
		return
	}

	header, rows := records[0], records[1:]

	page := s.tablePageFor(name)
	page.Summary = fmt.Sprintf("%d rows × %d columns", len(rows), len(header))
	page.Sheets = []sheetView{makeSheet("", header, rows)}
	s.renderTablePage(w, r, name, info, page)
}

func (s fileServer) tablePageFor(name string) tablePage {
	return tablePage{
		Title:   path.Base(name),
		Crumbs:  s.breadcrumbs(path.Dir(name)),
		RawHref: (&url.URL{Path: path.Base(name), RawQuery: "raw=1"}).String(),
	}
}

func (s fileServer) renderTablePage(w http.ResponseWriter, r *http.Request, name string, info fs.FileInfo, page tablePage) {
	var buf bytes.Buffer
	if err := tableTemplate.Execute(&buf, page); err != nil {
		log.Printf("render table %s: %v", name, err)
		http.Error(w, "cannot render table", http.StatusInternalServerError)
		return
	}

	outName := strings.TrimSuffix(path.Base(name), path.Ext(name)) + ".html"
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeContent(w, r, outName, info.ModTime(), bytes.NewReader(buf.Bytes()))
}

// makeSheet builds one displayed table, capping rows at maxTableRows.
// Column numbers span the widest displayed row so coordinates stay
// meaningful even when rows are ragged.
func makeSheet(name string, header []string, rows [][]string) sheetView {
	sheet := sheetView{Name: name, Header: header}

	if len(rows) > maxTableRows {
		sheet.Omitted = len(rows) - maxTableRows
		rows = rows[:maxTableRows]
	}

	maxCols := len(header)
	for _, cells := range rows {
		if len(cells) > maxCols {
			maxCols = len(cells)
		}
	}
	sheet.ColNums = make([]int, maxCols)
	for i := range sheet.ColNums {
		sheet.ColNums[i] = i + 1
	}

	sheet.Rows = make([]tableRow, len(rows))
	for i, cells := range rows {
		sheet.Rows[i] = tableRow{N: i + 1, Cells: cells}
	}
	return sheet
}

type tablePage struct {
	Title   string
	Crumbs  []crumb
	RawHref string
	Summary string
	Sheets  []sheetView
}

type sheetView struct {
	Name    string
	Summary string
	Header  []string
	ColNums []int
	Rows    []tableRow
	Omitted int
}

type tableRow struct {
	N     int
	Cells []string
}

// maxXLSXBytes caps how large a workbook is parsed for the table view.
const maxXLSXBytes = 10 << 20

func (s fileServer) serveXLSX(w http.ResponseWriter, r *http.Request, name string, info fs.FileInfo) {
	if info.Size() > maxXLSXBytes {
		s.serveRaw(w, r, name, info)
		return
	}

	data, err := fs.ReadFile(s.fsys, name)
	if err != nil {
		http.Error(w, "cannot read file", http.StatusInternalServerError)
		return
	}

	wb, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		log.Printf("open xlsx %s: %v", name, err)
		s.serveRaw(w, r, name, info)
		return
	}
	defer wb.Close()

	page := s.tablePageFor(name)
	sheets := wb.GetSheetList()
	page.Summary = fmt.Sprintf("%d sheet%s", len(sheets), plural(len(sheets)))
	for _, sheetName := range sheets {
		rows, err := wb.GetRows(sheetName)
		if err != nil {
			log.Printf("read xlsx sheet %s of %s: %v", sheetName, name, err)
			continue
		}
		var header []string
		var body [][]string
		if len(rows) > 0 {
			header, body = rows[0], rows[1:]
		}
		sheet := makeSheet(sheetName, header, body)
		if len(sheet.ColNums) > 0 {
			sheet.Summary = fmt.Sprintf("%d row%s × %d column%s",
				len(body), plural(len(body)), len(sheet.ColNums), plural(len(sheet.ColNums)))
		}
		page.Sheets = append(page.Sheets, sheet)
	}

	s.renderTablePage(w, r, name, info, page)
}

const (
	// maxSQLiteBytes caps how large a database is copied out for viewing;
	// the copy is needed because the sqlite driver wants a real file path,
	// which also lets databases inside archives work.
	maxSQLiteBytes = 100 << 20

	maxSQLiteTables       = 50
	maxSQLiteRowsPerTable = 50
)

func (s fileServer) serveSQLite(w http.ResponseWriter, r *http.Request, name string, info fs.FileInfo) {
	if info.Size() > maxSQLiteBytes {
		s.serveRaw(w, r, name, info)
		return
	}

	page, err := s.sqlitePage(name)
	if err != nil {
		log.Printf("sqlite %s: %v", name, err)
		s.serveRaw(w, r, name, info)
		return
	}

	s.renderTablePage(w, r, name, info, page)
}

func (s fileServer) sqlitePage(name string) (tablePage, error) {
	var page tablePage

	src, err := s.fsys.Open(name)
	if err != nil {
		return page, err
	}
	defer src.Close()

	tmp, err := os.CreateTemp("", "localmd-sqlite-*.db")
	if err != nil {
		return page, err
	}
	defer func() {
		if err := os.Remove(tmp.Name()); err != nil {
			log.Printf("remove temp db %s: %v", tmp.Name(), err)
		}
	}()

	n, err := io.Copy(tmp, io.LimitReader(src, maxSQLiteBytes+1))
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return page, err
	}
	if n > maxSQLiteBytes {
		return page, fmt.Errorf("database exceeds %d bytes", int64(maxSQLiteBytes))
	}

	db, err := sql.Open("sqlite3", "file:"+tmp.Name()+"?mode=ro&immutable=1")
	if err != nil {
		return page, err
	}
	defer db.Close()

	names, err := sqliteTableNames(db)
	if err != nil {
		return page, err
	}

	page = s.tablePageFor(name)
	page.Summary = fmt.Sprintf("%d table%s", len(names), plural(len(names)))
	if len(names) > maxSQLiteTables {
		page.Summary += fmt.Sprintf(", showing first %d", maxSQLiteTables)
		names = names[:maxSQLiteTables]
	}

	for _, table := range names {
		sheet, err := sqliteSheet(db, table)
		if err != nil {
			log.Printf("sqlite table %s of %s: %v", table, name, err)
			continue
		}
		page.Sheets = append(page.Sheets, sheet)
	}
	return page, nil
}

func sqliteTableNames(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`SELECT name FROM sqlite_master
		WHERE type IN ('table', 'view') AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		names = append(names, n)
	}
	return names, rows.Err()
}

func sqliteSheet(db *sql.DB, table string) (sheetView, error) {
	quoted := `"` + strings.ReplaceAll(table, `"`, `""`) + `"`

	var count int64
	if err := db.QueryRow("SELECT COUNT(*) FROM " + quoted).Scan(&count); err != nil {
		return sheetView{}, err
	}

	rows, err := db.Query(fmt.Sprintf("SELECT * FROM %s LIMIT %d", quoted, maxSQLiteRowsPerTable))
	if err != nil {
		return sheetView{}, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return sheetView{}, err
	}

	var body [][]string
	for rows.Next() {
		holders := make([]any, len(cols))
		for i := range holders {
			holders[i] = new(any)
		}
		if err := rows.Scan(holders...); err != nil {
			return sheetView{}, err
		}
		cells := make([]string, len(cols))
		for i, h := range holders {
			cells[i] = formatSQLiteValue(*h.(*any))
		}
		body = append(body, cells)
	}
	if err := rows.Err(); err != nil {
		return sheetView{}, err
	}

	sheet := makeSheet(table, cols, body)
	sheet.Summary = fmt.Sprintf("%d row%s × %d column%s", count, plural(int(count)), len(cols), plural(len(cols)))
	if count > int64(len(body)) {
		sheet.Omitted = int(count) - len(body)
	}
	return sheet, nil
}

// maxSQLiteCellChars keeps giant text or blob columns from wrecking the table.
const maxSQLiteCellChars = 200

func formatSQLiteValue(v any) string {
	var text string
	switch val := v.(type) {
	case nil:
		return "NULL"
	case []byte:
		if !isPlainText(val) {
			return fmt.Sprintf("(%d-byte blob)", len(val))
		}
		text = string(val)
	default:
		text = fmt.Sprint(val)
	}
	if runes := []rune(text); len(runes) > maxSQLiteCellChars {
		text = string(runes[:maxSQLiteCellChars]) + "…"
	}
	return text
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

var tableTemplate = template.Must(template.New("table").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<link rel="icon" href="data:,">
<title>{{.Title}}</title>
<style>
  :root { color-scheme: light; }
  html { color: #1a1a1a; background-color: #fdfdfd; }
  body {
    margin: 0 auto;
    max-width: 70em;
    padding: 50px;
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif;
  }
  @media (max-width: 600px) { body { padding: 12px; } }
  a { color: #0969da; text-decoration: none; }
  a:hover { text-decoration: underline; }
  nav { margin-bottom: 1rem; font-size: 1.1rem; }
  nav a { font-weight: 600; }
  nav span.sep { color: #57606a; }
  nav span.file { font-weight: 600; }
  nav a.raw { float: right; font-weight: 400; font-size: 0.85rem; }
  p.summary { color: #57606a; font-size: 0.85rem; }
  h2.sheet { font-size: 1rem; margin: 1.5rem 0 0.4rem; }
  div.tablewrap {
    overflow: auto;
    max-height: 85vh;
    width: fit-content;  /* hug narrow tables instead of spanning the page */
    max-width: 100%;
    border: 1px solid #d0d7de;
  }
  /* Sticky cells need border-collapse: separate — collapsed borders stay
     behind when a cell is stuck — and opaque backgrounds to cover the data
     scrolling beneath them. */
  table.data {
    border-collapse: separate;
    border-spacing: 0;
    font-size: 0.85rem;
    font-variant-numeric: tabular-nums;
  }
  table.data th, table.data td {
    border-right: 1px solid #d0d7de;
    border-bottom: 1px solid #d0d7de;
    padding: 0.3rem 0.6rem;
    text-align: left;
    vertical-align: top;
    background-color: #fff;
  }
  table.data th {
    position: sticky;
    top: 2rem;                /* pinned just below the coords row */
    z-index: 2;
    white-space: nowrap;
    background-color: #f6f8fa;
    font-weight: 600;
  }
  table.data td.rownum, table.data td.colnum, table.data th.corner {
    background-color: #e7f0fa;  /* faint blue sets coordinates apart from data */
    color: #57606a;
    text-align: center;
  }
  table.data tr.coords td {
    position: sticky;
    top: 0;
    z-index: 2;
    height: 2rem;             /* fixed so the header row can pin right below */
    box-sizing: border-box;
  }
  table.data td.rownum { position: sticky; left: 0; z-index: 1; }
  table.data th.corner { left: 0; z-index: 3; }
  table.data tr.coords td.rownum { left: 0; z-index: 3; }
</style>
</head>
<body>
<nav>{{range $i, $c := .Crumbs}}{{if gt $i 1}}<span class="sep">/</span>{{end}}<a href="{{$c.Href}}">{{$c.Name}}</a>{{end}}{{if gt (len .Crumbs) 1}}<span class="sep">/</span>{{end}}<span class="file">{{.Title}}</span><a class="raw" href="{{.RawHref}}">raw</a></nav>
<p class="summary">{{.Summary}}</p>
{{range .Sheets}}{{if .Name}}<h2 class="sheet">{{.Name}}</h2>
{{end}}{{if .Summary}}<p class="summary">{{.Summary}}</p>
{{end}}{{if .Header}}<div class="tablewrap">
<table class="data">
<tr class="coords"><td class="rownum"></td>{{range .ColNums}}<td class="colnum">{{.}}</td>{{end}}</tr>
<tr><th class="corner"></th>{{range .Header}}<th>{{.}}</th>{{end}}</tr>
{{range .Rows}}<tr><td class="rownum">{{.N}}</td>{{range .Cells}}<td>{{.}}</td>{{end}}</tr>
{{end}}</table>
</div>
{{else}}<p class="summary">(empty)</p>
{{end}}{{if .Omitted}}<p class="summary">&hellip; and {{.Omitted}} more rows</p>{{end}}
{{end}}</body>
</html>
`))

func (s fileServer) serveDirectory(w http.ResponseWriter, r *http.Request, name string) {
	if !strings.HasSuffix(r.URL.Path, "/") {
		u := *r.URL
		u.Path += "/"
		http.Redirect(w, r, u.String(), http.StatusMovedPermanently)
		return
	}

	entries, err := fs.ReadDir(s.fsys, name)
	if err != nil {
		http.Error(w, "cannot read directory", http.StatusInternalServerError)
		return
	}

	key, desc := normalizeSort(r.URL.Query())
	full := s.displayPath(name)
	page := directoryPage{
		Title:     "/" + strings.TrimSuffix(strings.TrimPrefix(full+"/", "./"), "/"),
		Crumbs:    s.breadcrumbs(name),
		Blurb:     s.readBlurb(name),
		SortLinks: sortLinks(key, desc),
	}
	if s.base != "" {
		page.RawHref = (&url.URL{Path: s.base, RawQuery: "raw=1"}).String()
	}
	if name != "." || s.base != "" {
		page.Entries = append(page.Entries, dirEntryView{Name: "..", Href: "../", IsDir: true})
	}

	items := make([]dirItem, 0, len(entries))
	for _, entry := range entries {
		info, _ := entry.Info()
		items = append(items, dirItem{entry: entry, info: info})
	}
	sortDirItems(items, key, desc)

	for _, item := range items {
		entry := item.entry
		view := dirEntryView{Name: entry.Name(), Href: (&url.URL{Path: entry.Name()}).String()}
		if entry.IsDir() {
			view.Name += "/"
			view.Href += "/"
			view.IsDir = true
			view.Blurb = s.readBlurb(path.Join(name, entry.Name()))
		} else if item.info != nil {
			view.Size = humanSize(item.info.Size())
		}
		if item.info != nil && !item.info.ModTime().IsZero() {
			view.ModTime = item.info.ModTime().Format("2006-01-02 15:04")
		}
		if strings.HasPrefix(entry.Name(), ".") {
			page.Dotted = append(page.Dotted, view)
		} else {
			page.Entries = append(page.Entries, view)
		}
	}

	if readme := findReadme(entries); readme != "" {
		full := path.Join(name, readme)
		if strings.EqualFold(path.Ext(readme), ".md") {
			html, err := s.renderDocument(r.Context(), full, renderOptions{})
			if err != nil {
				log.Printf("pandoc failed for %s: %v", full, err)
			} else {
				page.ReadmeName = readme
				page.Readme = template.HTML(html)
			}
		} else if text, err := fs.ReadFile(s.fsys, full); err == nil {
			page.ReadmeName = readme
			page.ReadmeText = string(text)
		} else {
			log.Printf("read %s: %v", full, err)
		}
	}

	var buf bytes.Buffer
	if err := directoryTemplate.Execute(&buf, page); err != nil {
		log.Printf("render directory %s: %v", name, err)
		http.Error(w, "cannot render directory", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(buf.Bytes()))
}

// Sort keys accepted in the ?sort= query parameter.
const (
	sortTime = "time"
	sortName = "name"
	sortSize = "size"
)

// normalizeSort maps ?sort= and ?dir= query values to a sort key and
// direction. Each key has a natural default direction: time newest-first,
// size largest-first, name A-Z. "newest" is accepted as a legacy alias.
func normalizeSort(q url.Values) (key string, desc bool) {
	switch q.Get("sort") {
	case sortName:
		key = sortName
	case sortSize:
		key = sortSize
	default:
		key = sortTime
	}
	switch q.Get("dir") {
	case "asc":
		desc = false
	case "desc":
		desc = true
	default:
		desc = key != sortName
	}
	return key, desc
}

type sortLink struct {
	Label  string
	Href   string
	Active bool
	Arrow  string
}

// sortLinks builds the sort selector: clicking the active key reverses its
// direction, clicking any other switches to it in its natural direction.
func sortLinks(key string, desc bool) []sortLink {
	dir := func(d bool) string {
		if d {
			return "desc"
		}
		return "asc"
	}
	mk := func(k string, defDesc bool) sortLink {
		l := sortLink{Label: k}
		if k == key {
			l.Active = true
			l.Arrow = "▴" // ▴ ascending
			if desc {
				l.Arrow = "▾" // ▾ descending
			}
			l.Href = "?sort=" + k + "&dir=" + dir(!desc)
		} else {
			l.Href = "?sort=" + k + "&dir=" + dir(defDesc)
		}
		return l
	}
	return []sortLink{mk(sortTime, true), mk(sortName, false), mk(sortSize, true)}
}

// dirItem pairs a directory entry with its FileInfo (nil when Stat failed) so
// sorting and rendering don't repeat the lookup.
type dirItem struct {
	entry fs.DirEntry
	info  fs.FileInfo
}

// sortDirItems orders a listing. Directories always come first; within that,
// the requested key and direction apply (with markdown grouped before other
// files under name sort — the original reading order). Name breaks all
// remaining ties, ascending and case-insensitively.
func sortDirItems(items []dirItem, key string, desc bool) {
	rank := func(it dirItem) int {
		switch {
		case it.entry.IsDir():
			return 0
		case key == sortName && strings.EqualFold(path.Ext(it.entry.Name()), ".md"):
			return 1
		default:
			return 2
		}
	}
	modTime := func(it dirItem) time.Time {
		if it.info == nil {
			return time.Time{}
		}
		return it.info.ModTime()
	}
	compare := func(a, b dirItem) int {
		switch key {
		case sortName:
			return strings.Compare(strings.ToLower(a.entry.Name()), strings.ToLower(b.entry.Name()))
		case sortSize:
			if a.entry.IsDir() || a.info == nil || b.info == nil {
				return 0
			}
			switch {
			case a.info.Size() < b.info.Size():
				return -1
			case a.info.Size() > b.info.Size():
				return 1
			}
			return 0
		default: // sortTime
			ta, tb := modTime(a), modTime(b)
			switch {
			case ta.Before(tb):
				return -1
			case tb.Before(ta):
				return 1
			}
			return 0
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if ra, rb := rank(a), rank(b); ra != rb {
			return ra < rb
		}
		c := compare(a, b)
		if desc {
			c = -c
		}
		if c != 0 {
			return c < 0
		}
		return strings.ToLower(a.entry.Name()) < strings.ToLower(b.entry.Name())
	})
}

// maxBlurbBytes caps the size of a blurb.txt shown in directory listings;
// larger (or non-text) files are simply not inlined.
const maxBlurbBytes = 512

// readBlurb returns the trimmed contents of the blurb.txt directly inside
// dir, or "" when there is none or it is too large or not plain text.
func (s fileServer) readBlurb(dir string) string {
	blurbPath := path.Join(dir, "blurb.txt")
	info, err := fs.Stat(s.fsys, blurbPath)
	if err != nil || info.IsDir() || info.Size() > maxBlurbBytes {
		return ""
	}
	text, err := fs.ReadFile(s.fsys, blurbPath)
	if err != nil || !isPlainText(text) {
		return ""
	}
	return strings.TrimSpace(string(text))
}

// isPlainText reports whether b is valid UTF-8 free of control characters
// other than ordinary whitespace.
func isPlainText(b []byte) bool {
	if !utf8.Valid(b) {
		return false
	}
	for _, r := range string(b) {
		if r < 0x20 && r != '\n' && r != '\r' && r != '\t' {
			return false
		}
	}
	return true
}

func findReadme(entries []fs.DirEntry) string {
	for _, want := range []string{"README.md", "README.txt"} {
		for _, entry := range entries {
			if !entry.IsDir() && strings.EqualFold(entry.Name(), want) {
				return entry.Name()
			}
		}
	}
	return ""
}

// displayPath is name as seen from the serve root, including any enclosing
// zip archives ("." at the root itself).
func (s fileServer) displayPath(name string) string {
	return path.Join(strings.TrimPrefix(s.base, "/"), name)
}

func (s fileServer) breadcrumbs(name string) []crumb {
	crumbs := []crumb{{Name: "/", Href: "/"}}
	full := s.displayPath(name)
	if full == "." || full == "" {
		return crumbs
	}

	href := "/"
	for _, seg := range strings.Split(full, "/") {
		href += (&url.URL{Path: seg}).String() + "/"
		crumbs = append(crumbs, crumb{Name: seg, Href: href})
	}
	return crumbs
}

func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

type directoryPage struct {
	Title      string
	Crumbs     []crumb
	Blurb      string
	SortLinks  []sortLink
	RawHref    string
	Entries    []dirEntryView
	Dotted     []dirEntryView
	ReadmeName string
	Readme     template.HTML
	ReadmeText string
}

type crumb struct {
	Name string
	Href string
}

type dirEntryView struct {
	Name    string
	Href    string
	IsDir   bool
	Blurb   string
	Size    string
	ModTime string
}

var directoryTemplate = template.Must(template.New("directory").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<link rel="icon" href="data:,">
<title>{{.Title}}</title>
<style>
  :root { color-scheme: light; }
  html { color: #1a1a1a; background-color: #fdfdfd; }
  body {
    margin: 0 auto;
    max-width: 56em;  /* wider than content pages: listings carry name, blurb, and metadata columns */
    padding: 50px;
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif;
  }
  @media (max-width: 600px) { body { padding: 12px; } }
  a { color: #0969da; text-decoration: none; }
  a:hover { text-decoration: underline; }
  nav { margin-bottom: 1rem; font-size: 1.1rem; }
  nav a { font-weight: 600; }
  nav span.sep { color: #57606a; }
  p.blurb { margin: -0.5rem 0 1rem; color: #57606a; }
  div.sort { margin-bottom: 0.5rem; font-size: 0.85rem; color: #57606a; }
  div.sort a.active { font-weight: 600; color: #1a1a1a; }
  nav a.raw { float: right; font-weight: 400; font-size: 0.85rem; }
  details.dotfiles { margin-top: 1rem; }
  details.dotfiles summary {
    cursor: pointer;
    color: #57606a;
    font-size: 0.85rem;
    margin-bottom: 0.3rem;
  }
  table.listing { border-collapse: collapse; width: 100%; }
  table.listing td { padding: 0.3rem 0.5rem 0.3rem 0; }
  /* Directory rows use separate name and blurb cells; file rows (which never
     have blurbs) span both with colspan=2, so a long filename neither widens
     the directory-name column nor leaves an empty blurb gap. Caps live on the
     anchors, not the cells: a max-width on a td makes its column reserve the
     cap width even when every name is short. */
  table.listing td.dname { white-space: nowrap; width: 1%; }
  table.listing td.dname a,
  table.listing td.fname a {
    display: inline-block;
    max-width: 24em;
    overflow: hidden;
    text-overflow: ellipsis;
    vertical-align: bottom;
  }
  table.listing td.fname { white-space: nowrap; }
  /* Let filenames use the combined name+blurb region, but never force the
     table wider than the window: cap against the viewport with room for the
     size/date columns. */
  table.listing td.fname a { max-width: min(42em, calc(100vw - 26em)); }
  /* width 100% + max-width 0 makes the blurb cell absorb exactly the spare
     table width (names and dates shrink to content via width 1%), truncating
     with an ellipsis at whatever space is actually available. */
  table.listing td.blurb {
    color: #57606a;
    font-size: 0.85rem;
    width: 100%;
    max-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    padding-left: 1rem;
  }
  /* On narrow windows, tighten the name caps; blurbs shrink on their own. */
  @media (max-width: 48em) {
    table.listing td.dname a, table.listing td.fname a { max-width: 14em; }
  }
  table.listing td.meta {
    color: #57606a;
    font-size: 0.85rem;
    text-align: right;
    white-space: nowrap;
    font-variant-numeric: tabular-nums;
    width: 1%;
  }
  section.readme { margin-top: 2rem; border-top: 1px solid #d0d7de; }
  section.readme h1.readme-title { font-size: 1rem; color: #57606a; font-weight: 600; }
  section.readme pre.readme-text {
    font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
    font-size: 0.85rem;
    white-space: pre-wrap;
  }
</style>
</head>
<body>
<nav>{{range $i, $c := .Crumbs}}{{if gt $i 1}}<span class="sep">/</span>{{end}}<a href="{{$c.Href}}">{{$c.Name}}</a>{{end}}{{if .RawHref}}<a class="raw" href="{{.RawHref}}">raw</a>{{end}}</nav>
{{if .Blurb}}<p class="blurb">{{.Blurb}}</p>
{{end}}<div class="sort">sort: {{range $i, $l := .SortLinks}}{{if $i}} &middot; {{end}}<a {{if $l.Active}}class="active" {{end}}href="{{$l.Href}}">{{$l.Label}}{{if $l.Active}} {{$l.Arrow}}{{end}}</a>{{end}}</div>
{{define "rows"}}{{range .}}<tr>{{if .IsDir}}<td class="dname"><a href="{{.Href}}" title="{{.Name}}">{{.Name}}</a></td><td class="blurb"{{with .Blurb}} title="{{.}}"{{end}}>{{.Blurb}}</td>{{else}}<td class="fname" colspan="2"><a href="{{.Href}}" title="{{.Name}}">{{.Name}}</a></td>{{end}}<td class="meta">{{.Size}}</td><td class="meta">{{.ModTime}}</td></tr>
{{end}}{{end}}<table class="listing">
{{template "rows" .Entries}}</table>
{{if .Dotted}}<details class="dotfiles">
<summary>{{len .Dotted}} dot-file{{if ne (len .Dotted) 1}}s{{end}}</summary>
<table class="listing">
{{template "rows" .Dotted}}</table>
</details>{{end}}
{{if .ReadmeName}}<section class="readme">
<h1 class="readme-title">{{.ReadmeName}}</h1>
{{if .Readme}}{{.Readme}}{{else}}<pre class="readme-text">{{.ReadmeText}}</pre>{{end}}
</section>{{end}}
</body>
</html>
`))

const pandocHeader = `<link rel="icon" href="data:,">
<script>
  MathJax = {
    chtml: { fontURL: "/` + assetPrefix + `/mathjax/output/chtml/fonts/woff-v2" }
  };
</script>
<style>
  :root {
    color-scheme: light;
  }

  blockquote {
    color: #000;
    border-left: 0.2rem solid #222;
    margin-left: 0;
    padding-left: 1rem;
  }

  @media print {
    @page {
      margin: 0.6in 0.6in 1.0in 0.6in;
      @bottom-center {
        content: counter(page) " / " counter(pages);
        color: #000 !important;
        -webkit-text-fill-color: #000 !important;
        font-size: 11pt;
      }
    }

    html,
    body,
    body * {
      color: #000 !important;
      -webkit-text-fill-color: #000 !important;
      opacity: 1 !important;
      text-shadow: none !important;
    }
  }
</style>
`
