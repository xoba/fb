package main

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
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
	handler := fileServer{fsys: fsys, root: root, pandoc: "pandoc"}

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
	root   string
	pandoc string
}

func (s fileServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := requestName(r.URL.Path)

	if name == assetPrefix || strings.HasPrefix(name, assetPrefix+"/") {
		serveAsset(w, r, name)
		return
	}

	preventCaching(w.Header(), r.Header)

	info, err := fs.Stat(s.fsys, name)
	if err == nil && info.IsDir() {
		s.serveDirectory(w, r, name)
		return
	}

	if err == nil && strings.EqualFold(path.Ext(name), ".md") {
		s.serveMarkdown(w, r, name, info)
		return
	}

	if err == nil && highlightable(name) &&
		r.URL.Query().Get("raw") != "1" && wantsDocument(r.Header) {
		s.serveSource(w, r, name, info)
		return
	}

	// http.ServeFileFS canonicalizes any request ending in /index.html into a
	// redirect back to its directory, which would make the listing link a
	// no-op loop. Serve the file by hand instead.
	if err == nil && !info.IsDir() && path.Base(name) == "index.html" {
		content, readErr := fs.ReadFile(s.fsys, name)
		if readErr != nil {
			http.Error(w, "cannot read file", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeContent(w, r, path.Base(name), info.ModTime(), bytes.NewReader(content))
		return
	}

	http.ServeFileFS(w, r, s.fsys, name)
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
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

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

	args = append(args, localPath(s.root, name))

	cmd := exec.CommandContext(ctx, s.pandoc, args...)
	cmd.Dir = filepath.Dir(localPath(s.root, name))

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
// serve root and the markdown file's directory, outermost first.
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
			hrefs = append(hrefs, (&url.URL{Path: "/" + cssPath}).String())
		}
	}
	return hrefs
}

func localPath(root, name string) string {
	if name == "." {
		return root
	}
	return filepath.Join(root, filepath.FromSlash(name))
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
	".csv":     true,
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
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if info.IsDir() || info.Size() > maxHighlightBytes {
		http.ServeFileFS(w, r, s.fsys, name)
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
		http.ServeFileFS(w, r, s.fsys, name)
		return
	}

	page := sourcePage{
		Title:   path.Base(name),
		Crumbs:  breadcrumbs(path.Dir(name)),
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

func (s fileServer) serveDirectory(w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

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

	page := directoryPage{
		Title:  "/" + strings.TrimSuffix(strings.TrimPrefix(name+"/", "./"), "/"),
		Crumbs: breadcrumbs(name),
		Blurb:  s.findBlurb(name, entries),
	}
	if name != "." {
		page.Entries = append(page.Entries, dirEntryView{Name: "..", Href: "../"})
	}

	sortDirEntries(entries)
	for _, entry := range entries {
		view := dirEntryView{Name: entry.Name(), Href: (&url.URL{Path: entry.Name()}).String()}
		if entry.IsDir() {
			view.Name += "/"
			view.Href += "/"
		} else if info, err := entry.Info(); err == nil {
			view.Size = humanSize(info.Size())
			view.ModTime = info.ModTime().Format("2006-01-02 15:04")
		}
		page.Entries = append(page.Entries, view)
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

// sortDirEntries orders a listing for reading: directories, then markdown
// files, then everything else, each group alphabetical and case-insensitive.
func sortDirEntries(entries []fs.DirEntry) {
	rank := func(e fs.DirEntry) int {
		switch {
		case e.IsDir():
			return 0
		case strings.EqualFold(path.Ext(e.Name()), ".md"):
			return 1
		default:
			return 2
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		ri, rj := rank(entries[i]), rank(entries[j])
		if ri != rj {
			return ri < rj
		}
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
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

func breadcrumbs(name string) []crumb {
	crumbs := []crumb{{Name: "/", Href: "/"}}
	if name == "." {
		return crumbs
	}

	href := "/"
	for _, seg := range strings.Split(name, "/") {
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
	Entries    []dirEntryView
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
  p.blurb span.blurb-label { font-weight: 600; }
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
<nav>{{range $i, $c := .Crumbs}}{{if gt $i 1}}<span class="sep">/</span>{{end}}<a href="{{$c.Href}}">{{$c.Name}}</a>{{end}}</nav>
{{if .Blurb}}<p class="blurb"><span class="blurb-label">blurb:</span> {{.Blurb}}</p>
{{end}}<table class="listing">
{{range .Entries}}<tr><td><a href="{{.Href}}">{{.Name}}</a></td><td class="meta">{{.Size}}</td><td class="meta">{{.ModTime}}</td></tr>
{{end}}</table>
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
