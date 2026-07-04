package main

import (
	"archive/zip"
	"bytes"
	"context"
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
	"time"
	"unicode/utf8"

	"github.com/alecthomas/chroma/v2"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
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

	if strings.EqualFold(path.Ext(name), ".md") {
		s.serveMarkdown(w, r, name, info)
		return
	}

	if r.URL.Query().Get("raw") != "1" && wantsDocument(r.Header) {
		if _, ok := tableDelims[strings.ToLower(path.Ext(name))]; ok {
			s.serveTable(w, r, name, info)
			return
		}
		if highlightable(name) {
			s.serveSource(w, r, name, info)
			return
		}
		if strings.EqualFold(path.Ext(name), ".zip") {
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
		if strings.EqualFold(path.Ext(prefix), ".zip") {
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

// descendArchive serves inner (an fs path within the named zip file) by
// mounting the archive as an fs.FS and routing through the ordinary pipeline,
// so members get listings, highlighting, and markdown rendering like any
// other file. An unreadable archive falls back to verbatim serving.
func (s fileServer) descendArchive(w http.ResponseWriter, r *http.Request, zipName string, info fs.FileInfo, inner string, depth int) {
	if depth >= maxArchiveDepth {
		http.NotFound(w, r)
		return
	}

	f, err := s.fsys.Open(zipName)
	if err != nil {
		http.Error(w, "cannot read archive", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	ra, ok := f.(io.ReaderAt)
	size := info.Size()
	if !ok {
		if size > maxArchiveBytes {
			http.Error(w, "nested archive too large", http.StatusRequestEntityTooLarge)
			return
		}
		data, err := io.ReadAll(f)
		if err != nil {
			http.Error(w, "cannot read archive", http.StatusInternalServerError)
			return
		}
		ra, size = bytes.NewReader(data), int64(len(data))
	}

	zr, err := zip.NewReader(ra, size)
	if err != nil {
		log.Printf("open zip %s: %v", zipName, err)
		s.serveRaw(w, r, zipName, info)
		return
	}

	sub := fileServer{fsys: zr, pandoc: s.pandoc, base: s.base + "/" + zipName}
	sub.route(w, r, inner, depth+1)
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

func (s fileServer) serveMarkdown(w http.ResponseWriter, r *http.Request, name string, info fs.FileInfo) {
	html, err := s.renderMarkdown(r.Context(), name, renderOptions{
		standalone: true,
		toc:        r.URL.Query().Get("toc") == "1",
	})
	if err != nil {
		log.Printf("pandoc failed for %s: %v", name, err)
		http.Error(w, "pandoc failed", http.StatusInternalServerError)
		return
	}

	outName := strings.TrimSuffix(path.Base(name), path.Ext(name)) + ".html"
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeContent(w, r, outName, info.ModTime(), bytes.NewReader(html))
}

type renderOptions struct {
	// standalone produces a complete styled document; otherwise pandoc emits a
	// body fragment for embedding (used for README previews in listings).
	standalone bool
	toc        bool
}

func (s fileServer) renderMarkdown(ctx context.Context, name string, opts renderOptions) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Feed the source through stdin rather than as a file path so markdown
	// works from any fs.FS, including the inside of zip archives.
	content, err := fs.ReadFile(s.fsys, name)
	if err != nil {
		return nil, err
	}

	args := []string{
		"-f", "markdown+footnotes+lists_without_preceding_blankline+tex_math_single_backslash+gfm_auto_identifiers+autolink_bare_uris+emoji",
		"--mathjax=/" + assetPrefix + "/mathjax/tex-mml-chtml.js",
		"--reference-location=section",
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
	omitted := 0
	if len(rows) > maxTableRows {
		omitted = len(rows) - maxTableRows
		rows = rows[:maxTableRows]
	}

	page := tablePage{
		Title:   path.Base(name),
		Crumbs:  s.breadcrumbs(path.Dir(name)),
		RawHref: (&url.URL{Path: path.Base(name), RawQuery: "raw=1"}).String(),
		Summary: fmt.Sprintf("%d rows × %d columns", len(records)-1, len(header)),
		Header:  header,
		Rows:    rows,
		Omitted: omitted,
	}

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

type tablePage struct {
	Title   string
	Crumbs  []crumb
	RawHref string
	Summary string
	Header  []string
	Rows    [][]string
	Omitted int
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
  div.tablewrap { overflow-x: auto; }
  table.data {
    border-collapse: collapse;
    font-size: 0.85rem;
    font-variant-numeric: tabular-nums;
  }
  table.data th, table.data td {
    border: 1px solid #d0d7de;
    padding: 0.3rem 0.6rem;
    text-align: left;
    vertical-align: top;
  }
  table.data th { background-color: #f6f8fa; font-weight: 600; }
</style>
</head>
<body>
<nav>{{range $i, $c := .Crumbs}}{{if gt $i 1}}<span class="sep">/</span>{{end}}<a href="{{$c.Href}}">{{$c.Name}}</a>{{end}}{{if gt (len .Crumbs) 1}}<span class="sep">/</span>{{end}}<span class="file">{{.Title}}</span><a class="raw" href="{{.RawHref}}">raw</a></nav>
<p class="summary">{{.Summary}}</p>
<div class="tablewrap">
<table class="data">
<tr>{{range .Header}}<th>{{.}}</th>{{end}}</tr>
{{range .Rows}}<tr>{{range .}}<td>{{.}}</td>{{end}}</tr>
{{end}}</table>
</div>
{{if .Omitted}}<p class="summary">&hellip; and {{.Omitted}} more rows</p>{{end}}
</body>
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
		Blurb:     s.findBlurb(name, entries),
		SortLinks: sortLinks(key, desc),
	}
	if s.base != "" {
		page.RawHref = (&url.URL{Path: s.base, RawQuery: "raw=1"}).String()
	}
	if name != "." || s.base != "" {
		page.Entries = append(page.Entries, dirEntryView{Name: "..", Href: "../"})
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
			html, err := s.renderMarkdown(r.Context(), full, renderOptions{})
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

// maxBlurbBytes caps the size of a blurb.txt shown atop its directory
// listing; larger (or non-text) files are simply not inlined.
const maxBlurbBytes = 512

// findBlurb returns the trimmed contents of a directory's blurb.txt, or ""
// when there is none or it is too large or not plain text.
func (s fileServer) findBlurb(name string, entries []fs.DirEntry) string {
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(entry.Name(), "blurb.txt") {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.Size() > maxBlurbBytes {
			return ""
		}
		text, err := fs.ReadFile(s.fsys, path.Join(name, entry.Name()))
		if err != nil || !isPlainText(text) {
			return ""
		}
		return strings.TrimSpace(string(text))
	}
	return ""
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
    max-width: 42em;
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
  table.listing td.meta {
    color: #57606a;
    font-size: 0.85rem;
    text-align: right;
    white-space: nowrap;
    font-variant-numeric: tabular-nums;
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
{{define "rows"}}{{range .}}<tr><td><a href="{{.Href}}">{{.Name}}</a></td><td class="meta">{{.Size}}</td><td class="meta">{{.ModTime}}</td></tr>
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
