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
	"encoding/json"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"io/fs"
	"log"
	"math"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing/fstest"
	"time"
	"unicode/utf8"

	"github.com/alecthomas/chroma/v2"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	_ "github.com/mattn/go-sqlite3"
	"github.com/rwcarlsen/goexif/exif"
	exiftiff "github.com/rwcarlsen/goexif/tiff"
	"github.com/xuri/excelize/v2"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
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

//go:embed assets/mathjax assets/favicon.png
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
	handler := fileServer{fsys: fsys, pandoc: "pandoc", dir: root}

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

	// dir is the OS path of fsys's root when it is a real directory, and ""
	// when fsys is archive-backed. It lets sqlite databases be opened in
	// place — no copy, no size cap.
	dir string
}

func (s fileServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := requestName(r.URL.Path)

	if name == assetPrefix || strings.HasPrefix(name, assetPrefix+"/") {
		s.serveInternal(w, r, name)
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

	ext := strings.ToLower(path.Ext(viewName(name)))
	browserView := r.URL.Query().Get("raw") != "1" && wantsDocument(r.Header)

	// Markdown renders for every client (the server's original purpose);
	// other pandoc-supported documents render only on browser navigation so
	// curl and scripts still get the original bytes.
	if format, ok := pandocFormats[ext]; ok && (ext == ".md" || browserView) {
		s.serveDocument(w, r, name, info, format)
		return
	}

	// Async stat fetches come from scripts, not navigations, so handle them
	// ahead of the browser-view gating.
	if r.URL.Query().Get("stat") != "" && isTabularContainer(name) && ext != ".xlsx" {
		s.serveSQLiteStat(w, r, name, info)
		return
	}

	if browserView {
		switch {
		case tableDelims[ext] != 0:
			s.serveTable(w, r, name, info)
			return
		case isTabularContainer(name):
			// Gate on viewability before serveContainerListing's trailing
			// slash redirect: an oversized container must fall back to raw
			// at its own URL, so the browser downloads it under its real
			// filename rather than "download".
			if s.containerViewable(name, info) {
				s.serveContainerListing(w, r, name, info)
				return
			}
		case imageExts[ext]:
			s.serveImage(w, r, name, info)
			return
		case ext == ".plist":
			s.servePlist(w, r, name, info)
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
// checking whether some prefix of them is an archive to descend into or a
// tabular container (spreadsheet, database) whose sheet or table is named.
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
		if isTabularContainer(prefix) && i == len(elems)-2 {
			s.serveContainerMember(w, r, prefix, info, elems[len(elems)-1])
			return
		}
		break
	}
	http.NotFound(w, r)
}

// viewName strips a trailing editor-backup marker, so files like README.md~
// get the same viewer as their base file. Used for type detection only; the
// actual file is read under its real name.
func viewName(name string) string {
	return strings.TrimSuffix(name, "~")
}

// isXLSX reports whether name should be treated as a workbook (as opposed
// to a sqlite database, the other tabular container kind).
func isXLSX(name string) bool {
	return strings.EqualFold(path.Ext(viewName(name)), ".xlsx")
}

func isTabularContainer(name string) bool {
	switch strings.ToLower(path.Ext(viewName(name))) {
	case ".xlsx", ".sqlite", ".sqlite3", ".db":
		return true
	}
	return false
}

// containerViewable reports whether a tabular container is within its
// viewer's size limits. On-disk sqlite databases have none (they are opened
// in place); archive-backed ones are bounded by the temp-copy cap.
func (s fileServer) containerViewable(name string, info fs.FileInfo) bool {
	if isXLSX(name) {
		return info.Size() <= maxXLSXBytes
	}
	return s.dir != "" || info.Size() <= maxSQLiteBytes
}

const (
	// maxArchiveBytes caps how much of a non-seekable file (an archive member,
	// including a nested archive) is buffered in memory for serving.
	maxArchiveBytes = 128 << 20

	// maxArchiveDepth bounds zip-within-zip nesting.
	maxArchiveDepth = 3
)

func isArchiveName(name string) bool {
	return strings.EqualFold(path.Ext(viewName(name)), ".zip") || isTarName(name)
}

func isTarName(name string) bool {
	l := strings.ToLower(viewName(name))
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
		arc, err = tarFS(viewName(arcName), f, info)
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
	zr, err := zip.NewReader(ra, size)
	if err != nil {
		return nil, err
	}
	sanitizeZipNames(zr)
	return zr, nil
}

// sanitizeZipNames makes member names usable as fs paths. Legacy zips store
// names in Latin-1 (or CP437) rather than UTF-8, and Go's zip fs.FS refuses
// to list any directory containing such a name — one bad entry breaks the
// whole listing. Decode non-UTF-8 names as Latin-1 (lossless byte-to-rune)
// and drop the entries whose names are still unusable. Must run before the
// first fs operation on zr, which is when its file tree gets built.
func sanitizeZipNames(zr *zip.Reader) {
	kept := zr.File[:0]
	dropped := 0
	for _, f := range zr.File {
		if !utf8.ValidString(f.Name) {
			f.Name = latin1ToUTF8(f.Name)
		}
		trimmed := strings.TrimSuffix(f.Name, "/") // directory entries carry a trailing slash
		if trimmed == "" || !fs.ValidPath(trimmed) || strings.Contains(trimmed, `\`) {
			dropped++
			continue
		}
		kept = append(kept, f)
	}
	zr.File = kept
	if dropped > 0 {
		log.Printf("zip: dropped %d entries with unusable names", dropped)
	}
}

func latin1ToUTF8(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		b.WriteRune(rune(s[i]))
	}
	return b.String()
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

	// Keep inline display for viewable types, but make sure anything the
	// browser decides to download gets the file's real name — even when the
	// request URL ends in a slash.
	w.Header().Set("Content-Disposition",
		mime.FormatMediaType("inline", map[string]string{"filename": path.Base(name)}))

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

// serveInternal handles the reserved /_localmd/ namespace: embedded assets,
// the drag-and-drop upload endpoint, and browsing of dropped files.
func (s fileServer) serveInternal(w http.ResponseWriter, r *http.Request, name string) {
	if name == assetPrefix+"/drop" {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.handleDrop(w, r)
		return
	}

	if name == assetPrefix+"/drops" || strings.HasPrefix(name, assetPrefix+"/drops/") {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		dir, err := dropsDir()
		if err != nil {
			http.Error(w, "drops unavailable", http.StatusInternalServerError)
			return
		}
		preventCaching(w.Header(), r.Header)
		sub := fileServer{fsys: os.DirFS(dir), pandoc: s.pandoc, base: "/" + assetPrefix + "/drops", dir: dir}
		inner := strings.TrimPrefix(strings.TrimPrefix(name, assetPrefix+"/drops"), "/")
		if inner == "" {
			inner = "."
		}
		sub.route(w, r, inner, 0)
		return
	}

	serveAsset(w, r, name)
}

var (
	dropsOnce sync.Once
	dropsPath string
	dropsErr  error
	dropSeq   atomic.Int64
)

// dropsDir is the per-process directory holding drag-and-dropped files.
func dropsDir() (string, error) {
	dropsOnce.Do(func() {
		dropsPath, dropsErr = os.MkdirTemp("", "localmd-drops-")
	})
	return dropsPath, dropsErr
}

// maxDropBytes caps a single drag-and-dropped upload; nothing beyond the
// largest viewer cap (sqlite/tar at 100 MB) could be usefully viewed anyway.
const maxDropBytes = 100 << 20

// handleDrop stores an uploaded file under a fresh sequence directory (so
// original basenames are kept without collisions) and replies with the URL
// where the copy is served through the ordinary pipeline.
func (s fileServer) handleDrop(w http.ResponseWriter, r *http.Request) {
	dir, err := dropsDir()
	if err != nil {
		http.Error(w, "drops unavailable", http.StatusInternalServerError)
		return
	}

	base := path.Base(r.URL.Query().Get("name"))
	if base == "." || base == "/" || base == ".." {
		base = "dropped"
	}

	seq := fmt.Sprintf("%06d", dropSeq.Add(1))
	if err := os.MkdirAll(filepath.Join(dir, seq), 0o755); err != nil {
		http.Error(w, "cannot store drop", http.StatusInternalServerError)
		return
	}

	f, err := os.Create(filepath.Join(dir, seq, base))
	if err != nil {
		http.Error(w, "cannot store drop", http.StatusInternalServerError)
		return
	}
	_, err = io.Copy(f, http.MaxBytesReader(w, r.Body, maxDropBytes))
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		http.Error(w, "upload failed: "+err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, (&url.URL{Path: "/" + assetPrefix + "/drops/" + seq + "/" + base}).String())
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

// docFormat marks legacy binary Word documents, which pandoc cannot read;
// they are converted to HTML by macOS's textutil first.
const docFormat = "textutil-doc"

// pandocFormats maps file extensions to the pandoc input format used to
// render them as HTML.
var pandocFormats = map[string]string{
	".md":    "markdown+footnotes+lists_without_preceding_blankline+tex_math_single_backslash+gfm_auto_identifiers+autolink_bare_uris+emoji",
	".ipynb": "ipynb",
	".doc":   docFormat,
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

	if format == docFormat {
		converted, err := docToHTML(ctx, content)
		if err != nil {
			return nil, err
		}
		content, format = converted, "html"
	}

	var header string
	if opts.standalone {
		var cleanup func()
		header, cleanup, err = writePandocHeader(s.stylesheetLinks(name))
		if err != nil {
			return nil, err
		}
		defer cleanup()
	}

	buildArgs := func(embed bool) []string {
		// --embed-resources inlines everything the page references, so the
		// MathJax src must then be a real file, not our URL path: point it at
		// a temp copy extracted from the binary's embedded assets.
		mathjax := "--mathjax=/" + assetPrefix + "/mathjax/tex-mml-chtml.js"
		if embed {
			if p, err := mathjaxFile(); err == nil {
				mathjax = "--mathjax=" + p
			}
		}

		args := []string{"-f", format, mathjax, "--reference-location=section"}
		if embed {
			args = append(args, "--embed-resources")
		}
		if opts.toc {
			args = append(args, "--toc", "--toc-depth=3")
		}
		if opts.standalone {
			args = append(args,
				"-s",
				// Reading from stdin leaves pandoc no filename to derive
				// <title> from (it would fall back to "-"); pagetitle sets
				// the title element without a visible title block.
				"-V", "pagetitle="+path.Base(name),
				"--include-in-header", header,
				"-V", `mainfont=-apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif`,
				"-V", "monofont=ui-monospace, SFMono-Regular, Menlo, Consolas, monospace",
				"-V", "linkcolor=#0969da",
				"-V", "monobackgroundcolor=#f6f8fa",
				"-V", "maxwidth=42em",
			)
		}
		return args
	}

	// Binary formats carry their images internally (pandoc's media bag);
	// embed them as data URIs so they survive as a single HTML page. But
	// embedding fails hard when a document references a resource that no
	// longer exists, so fall back to a render with plain (possibly broken)
	// references rather than no render at all.
	embed := format != pandocFormats[".md"]
	out, err := runPandoc(ctx, s.pandoc, buildArgs(embed), content)
	if err != nil && embed {
		log.Printf("pandoc --embed-resources failed for %s, retrying without: %v", name, err)
		out, err = runPandoc(ctx, s.pandoc, buildArgs(false), content)
	}
	return out, err
}

// docToHTML converts a legacy binary Word document to HTML with macOS's
// built-in textutil.
func docToHTML(ctx context.Context, doc []byte) ([]byte, error) {
	textutil, err := exec.LookPath("textutil")
	if err != nil {
		return nil, fmt.Errorf(".doc conversion needs textutil: %w", err)
	}

	cmd := exec.CommandContext(ctx, textutil, "-stdin", "-stdout", "-format", "doc", "-convert", "html")
	cmd.Stdin = bytes.NewReader(doc)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("textutil: %s: %w", msg, err)
	}
	return out, nil
}

func runPandoc(ctx context.Context, pandoc string, args []string, input []byte) ([]byte, error) {
	cmd := exec.CommandContext(ctx, pandoc, args...)
	cmd.Stdin = bytes.NewReader(input)

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

var (
	mathjaxOnce sync.Once
	mathjaxPath string
	mathjaxErr  error
)

// mathjaxFile extracts the embedded MathJax bundle to a temp file, once per
// process, for pandoc --embed-resources to inline into rendered pages.
func mathjaxFile() (string, error) {
	mathjaxOnce.Do(func() {
		data, err := embeddedAssets.ReadFile("assets/mathjax/tex-mml-chtml.js")
		if err != nil {
			mathjaxErr = err
			return
		}
		f, err := os.CreateTemp("", "localmd-mathjax-*.js")
		if err != nil {
			mathjaxErr = err
			return
		}
		if _, err := f.Write(data); err != nil {
			f.Close()
			mathjaxErr = err
			return
		}
		mathjaxErr = f.Close()
		mathjaxPath = f.Name()
	})
	return mathjaxPath, mathjaxErr
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

// imageExts lists image types shown on a viewer page with a technical
// readout (dimensions, file size, EXIF) beneath the image. The image itself
// is fetched as a subresource, which gets the raw bytes as usual.
var imageExts = map[string]bool{
	".bmp":  true,
	".gif":  true,
	".jpeg": true,
	".jpg":  true,
	".png":  true,
	".tif":  true,
	".tiff": true,
	".webp": true,
}

// maxImageMetaBytes bounds how much of an image is read for metadata;
// dimensions and EXIF live near the start of the file.
const maxImageMetaBytes = 32 << 20

func (s fileServer) serveImage(w http.ResponseWriter, r *http.Request, name string, info fs.FileInfo) {
	f, err := s.fsys.Open(name)
	if err != nil {
		http.Error(w, "cannot read file", http.StatusInternalServerError)
		return
	}
	data, err := io.ReadAll(io.LimitReader(f, maxImageMetaBytes))
	f.Close()
	if err != nil {
		http.Error(w, "cannot read file", http.StatusInternalServerError)
		return
	}

	rawHref := (&url.URL{Path: path.Base(name), RawQuery: "raw=1"}).String()
	page := imagePage{
		Title:   path.Base(name),
		Crumbs:  s.breadcrumbs(path.Dir(name)),
		RawHref: rawHref,
		Src:     rawHref,
	}
	add := func(k, v string) {
		if v != "" {
			page.Rows = append(page.Rows, kvRow{K: k, V: v})
		}
	}

	add("file size", fmt.Sprintf("%s (%d bytes)", humanSize(info.Size()), info.Size()))
	if !info.ModTime().IsZero() {
		add("modified", info.ModTime().Format("2006-01-02 15:04:05"))
	}
	mimeType := mime.TypeByExtension(strings.ToLower(path.Ext(viewName(name))))
	if cfg, format, err := image.DecodeConfig(bytes.NewReader(data)); err == nil {
		mimeType = "image/" + format // from actual content, not the extension
		add("dimensions", fmt.Sprintf("%d × %d (%.1f MP)", cfg.Width, cfg.Height,
			float64(cfg.Width)*float64(cfg.Height)/1e6))
	}
	add("format", mimeType)

	var lat, long float64
	var hasGPS bool
	page.Exif, lat, long, hasGPS = exifRows(data)
	if hasGPS {
		page.Map = osmMap(lat, long)
		page.MapHref = fmt.Sprintf("https://www.openstreetmap.org/?mlat=%.6f&mlon=%.6f#map=16/%.6f/%.6f",
			lat, long, lat, long)
		page.AppleMaps = fmt.Sprintf("https://maps.apple.com/?ll=%.6f,%.6f", lat, long)
	}

	var buf bytes.Buffer
	if err := imageTemplate.Execute(&buf, page); err != nil {
		log.Printf("render image %s: %v", name, err)
		http.Error(w, "cannot render image page", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeContent(w, r, "index.html", info.ModTime(), bytes.NewReader(buf.Bytes()))
}

// exifWalkFunc adapts a function to goexif's Walker interface.
type exifWalkFunc func(name exif.FieldName, tag *exiftiff.Tag) error

func (f exifWalkFunc) Walk(name exif.FieldName, tag *exiftiff.Tag) error {
	return f(name, tag)
}

// exifRows extracts every readable EXIF field, sorted by name, with a
// friendly GPS coordinate row (and the decoded position) when present.
func exifRows(data []byte) (rows []kvRow, lat, long float64, hasGPS bool) {
	x, err := exif.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, 0, 0, false
	}

	x.Walk(exifWalkFunc(func(field exif.FieldName, tag *exiftiff.Tag) error {
		if field == "MakerNote" || strings.HasSuffix(string(field), "IFDPointer") {
			return nil // opaque vendor blob / internal file offsets
		}
		v := tag.String()
		if sv, err := tag.StringVal(); err == nil {
			v = strings.TrimSpace(sv)
		}
		v = strings.Trim(v, `"`)
		v = formatExifValue(field, v)
		if v == "" || len(v) > 150 {
			return nil
		}
		rows = append(rows, kvRow{K: string(field), V: v})
		return nil
	}))
	sort.Slice(rows, func(i, j int) bool { return rows[i].K < rows[j].K })

	if lat, long, err = x.LatLong(); err == nil {
		hasGPS = true
		rows = append([]kvRow{{K: "GPS position", V: fmt.Sprintf("%.6f, %.6f", lat, long)}}, rows...)
	}
	return rows, lat, long, hasGPS
}

// formatExifValue makes the common photography tags read naturally instead
// of as raw rationals: f/7.1 rather than 71/10.
func formatExifValue(field exif.FieldName, v string) string {
	switch field {
	case "FNumber":
		if f, ok := ratFloat(v); ok {
			return fmt.Sprintf("f/%.1f", f)
		}
	case "FocalLength":
		if f, ok := ratFloat(v); ok {
			return fmt.Sprintf("%g mm", f)
		}
	case "ExposureTime":
		return v + " s"
	case "ExposureBiasValue":
		if f, ok := ratFloat(v); ok {
			return fmt.Sprintf("%g EV", f)
		}
	case "DigitalZoomRatio", "MaxApertureValue", "ApertureValue", "ShutterSpeedValue", "BrightnessValue":
		if f, ok := ratFloat(v); ok {
			return fmt.Sprintf("%.4g", f)
		}
	}
	return v
}

// ratFloat parses an EXIF rational like "71/10".
func ratFloat(v string) (float64, bool) {
	num, den, ok := strings.Cut(v, "/")
	if !ok {
		return 0, false
	}
	n, err1 := strconv.ParseFloat(num, 64)
	d, err2 := strconv.ParseFloat(den, 64)
	if err1 != nil || err2 != nil || d == 0 {
		return 0, false
	}
	return n / d, true
}

type imagePage struct {
	Title     string
	Crumbs    []crumb
	RawHref   string
	Src       string
	Rows      []kvRow
	Exif      []kvRow
	Map       *mapView
	MapHref   string
	AppleMaps string
}

// mapView is a 3×3 grid of OpenStreetMap raster tiles with a dot positioned
// at the photo's GPS coordinates — plain images, no scripts or iframes.
type mapView struct {
	Rows       [][]string // tile URLs
	DotX, DotY int        // dot position within the grid, in pixels
}

func osmMap(lat, long float64) *mapView {
	const zoom = 15
	const tile = 256
	n := math.Exp2(zoom)
	xt := (long + 180) / 360 * n
	latRad := lat * math.Pi / 180
	yt := (1 - math.Log(math.Tan(latRad)+1/math.Cos(latRad))/math.Pi) / 2 * n
	if math.IsNaN(yt) || math.IsInf(yt, 0) || yt < 1 || yt > n-2 || xt < 1 || xt > n-2 {
		return nil
	}

	x0, y0 := int(xt)-1, int(yt)-1
	m := &mapView{
		DotX: int((xt - float64(x0)) * tile),
		DotY: int((yt - float64(y0)) * tile),
	}
	for dy := range 3 {
		var row []string
		for dx := range 3 {
			row = append(row, fmt.Sprintf("https://tile.openstreetmap.org/%d/%d/%d.png", zoom, x0+dx, y0+dy))
		}
		m.Rows = append(m.Rows, row)
	}
	return m
}

type kvRow struct {
	K, V string
}

var imageTemplate = template.Must(template.New("image").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<link rel="icon" href="/` + assetPrefix + `/favicon.png">
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
  img.subject {
    max-width: 100%;
    height: auto;
    border: 1px solid #d0d7de;
    border-radius: 6px;
    background:
      repeating-conic-gradient(#f0f0f0 0% 25%, #fff 0% 50%) 0 0/20px 20px; /* checkerboard shows transparency */
  }
  h2.sheet { font-size: 1rem; margin: 1.5rem 0 0.4rem; }
  table.kv { border-collapse: collapse; font-size: 0.85rem; }
  table.kv td { padding: 0.25rem 1.25rem 0.25rem 0; vertical-align: top; }
  table.kv td.k { color: #57606a; white-space: nowrap; }
  div.map {
    position: relative;
    width: 768px;
    max-width: 100%;
    overflow: hidden;
    line-height: 0;
    border: 1px solid #d0d7de;
    border-radius: 6px;
  }
  div.map .maprow { white-space: nowrap; }
  div.map .dot {
    position: absolute;
    width: 14px;
    height: 14px;
    margin: -7px 0 0 -7px;
    background: #f85149;
    border: 2px solid #fff;
    border-radius: 50%;
    box-shadow: 0 0 4px rgba(0,0,0,0.5);
  }
  p.summary { color: #57606a; font-size: 0.85rem; }
</style>
` + dropJS + `</head>
<body>
<nav>{{range $i, $c := .Crumbs}}{{if gt $i 1}}<span class="sep">/</span>{{end}}<a href="{{$c.Href}}">{{$c.Name}}</a>{{end}}{{if gt (len .Crumbs) 1}}<span class="sep">/</span>{{end}}<span class="file">{{.Title}}</span><a class="raw" href="{{.RawHref}}">raw</a></nav>
<img class="subject" src="{{.Src}}" alt="{{.Title}}">
<table class="kv">
{{range .Rows}}<tr><td class="k">{{.K}}</td><td>{{.V}}</td></tr>
{{end}}</table>
{{if .Exif}}<h2 class="sheet">exif</h2>
<table class="kv">
{{range .Exif}}<tr><td class="k">{{.K}}</td><td>{{.V}}</td></tr>
{{end}}</table>
{{end}}{{if .Map}}<h2 class="sheet">location</h2>
<div class="map">
{{range .Map.Rows}}<div class="maprow">{{range .}}<img src="{{.}}" width="256" height="256" alt="">{{end}}</div>
{{end}}<div class="dot" style="left:{{.Map.DotX}}px;top:{{.Map.DotY}}px"></div>
</div>
<p class="summary"><a href="{{.MapHref}}">OpenStreetMap</a> &middot; <a href="{{.AppleMaps}}">Apple Maps</a> &middot; map data &copy; OpenStreetMap contributors</p>
{{end}}</body>
</html>
`))

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
	base := path.Base(viewName(name))
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

	s.renderSourcePage(w, r, name, info, path.Base(viewName(name)), string(src))
}

// servePlist shows a property list as syntax-highlighted XML, converting
// binary plists with macOS's plutil first.
func (s fileServer) servePlist(w http.ResponseWriter, r *http.Request, name string, info fs.FileInfo) {
	if info.Size() > maxHighlightBytes {
		s.serveRaw(w, r, name, info)
		return
	}

	data, err := fs.ReadFile(s.fsys, name)
	if err != nil {
		http.Error(w, "cannot read file", http.StatusInternalServerError)
		return
	}

	if converted, err := plistToXML(r.Context(), data); err == nil {
		data = converted
	} else if bytes.HasPrefix(data, []byte("bplist")) {
		// Binary and unconvertible (plutil missing or file corrupt):
		// nothing sensible to show.
		log.Printf("convert plist %s: %v", name, err)
		s.serveRaw(w, r, name, info)
		return
	}

	s.renderSourcePage(w, r, name, info, "plist.xml", string(data))
}

func plistToXML(ctx context.Context, data []byte) ([]byte, error) {
	plutil, err := exec.LookPath("plutil")
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, plutil, "-convert", "xml1", "-o", "-", "-")
	cmd.Stdin = bytes.NewReader(data)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("plutil: %s: %w", msg, err)
	}
	return out, nil
}

// renderSourcePage shows src as a highlighted source view, lexed by
// lexName's extension (which may differ from name when the content was
// converted, as with binary plists).
func (s fileServer) renderSourcePage(w http.ResponseWriter, r *http.Request, name string, info fs.FileInfo, lexName, src string) {
	code, err := highlightSource(lexName, src)
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
<link rel="icon" href="/` + assetPrefix + `/favicon.png">
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
` + dropJS + `</head>
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
	reader.Comma = tableDelims[strings.ToLower(path.Ext(viewName(name)))]
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
	Schema  template.HTML
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

// Tabular containers — xlsx workbooks and sqlite databases — browse like
// directories: navigating to the file lists its sheets or tables, and each
// member renders as a single CSV-style table page. Non-navigation fetches of
// a member (and ?raw=1) export it as CSV.

const (
	// maxXLSXBytes caps how large a workbook is parsed.
	maxXLSXBytes = 10 << 20

	// maxSQLiteBytes caps how large a database is copied out for viewing;
	// the copy is needed because the sqlite driver wants a real file path,
	// which also lets databases inside archives work.
	maxSQLiteBytes = 100 << 20
)

// containerMember is one sheet or table inside a tabular container.
type containerMember struct {
	Name   string
	Detail string // listing metadata: "N rows × M columns", or "view" etc.
	Bytes  string // on-disk footprint (sqlite with dbstat only)
}

func (s fileServer) serveContainerListing(w http.ResponseWriter, r *http.Request, name string, info fs.FileInfo) {
	if !strings.HasSuffix(r.URL.Path, "/") {
		u := *r.URL
		u.Path += "/"
		http.Redirect(w, r, u.String(), http.StatusMovedPermanently)
		return
	}

	page := directoryPage{
		Title:   "/" + s.displayPath(name),
		Crumbs:  s.breadcrumbs(name),
		RawHref: (&url.URL{Path: s.base + "/" + name, RawQuery: "raw=1"}).String(),
		Entries: []dirEntryView{{Name: "..", Href: "../", IsDir: true}},
	}

	var members []containerMember
	var err error
	if isXLSX(name) {
		members, err = s.xlsxMembers(name, info)
	} else {
		db, cleanup, dbErr := s.openSQLite(name, info)
		if dbErr == nil {
			defer cleanup()
			// On-disk databases can be huge: serve the listing instantly
			// with names only and let statsJS fill in per-table stats with
			// a progress indicator. Archive-backed ones are small; compute
			// synchronously.
			page.StatsAsync = s.dir != ""
			members, err = sqliteMemberList(db, !page.StatsAsync)

			page.QueryForm = true
			page.Query = strings.TrimSpace(r.URL.Query().Get("q"))
			if page.Query != "" {
				sheet, qErr := sqliteQuerySheet(r.Context(), db, page.Query)
				if qErr != nil {
					page.QueryError = qErr.Error()
				} else {
					page.QuerySheet = &sheet
				}
			}
		} else {
			err = dbErr
		}
	}
	if err != nil {
		log.Printf("open container %s: %v", name, err)
		s.serveRaw(w, r, name, info)
		return
	}

	for _, m := range members {
		page.Entries = append(page.Entries, dirEntryView{
			Name:    m.Name,
			Href:    (&url.URL{Path: m.Name}).String(),
			Size:    m.Detail,
			ModTime: m.Bytes,
		})
	}

	var buf bytes.Buffer
	if err := directoryTemplate.Execute(&buf, page); err != nil {
		log.Printf("render container %s: %v", name, err)
		http.Error(w, "cannot render listing", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(buf.Bytes()))
}

// sqliteQuerySheet runs one read-only query and shapes the result as a
// displayable sheet, reading at most maxTableRows rows.
func sqliteQuerySheet(ctx context.Context, db *sql.DB, query string) (sheetView, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return sheetView{}, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return sheetView{}, err
	}

	var body [][]string
	truncated := false
	for rows.Next() {
		if len(body) == maxTableRows {
			truncated = true
			break
		}
		cells, err := scanSQLiteRow(rows, len(cols), formatSQLiteValue)
		if err != nil {
			return sheetView{}, err
		}
		body = append(body, cells)
	}
	if err := rows.Err(); err != nil {
		return sheetView{}, err
	}

	sheet := makeSheet("", cols, body)
	sheet.Summary = fmt.Sprintf("%d row%s × %d column%s", len(body), plural(len(body)), len(cols), plural(len(cols)))
	if truncated {
		sheet.Summary = fmt.Sprintf("first %d rows × %d column%s", maxTableRows, len(cols), plural(len(cols)))
	}
	return sheet, nil
}

// serveContainerMember serves one sheet or table: a table page for browser
// navigations, CSV bytes otherwise.
func (s fileServer) serveContainerMember(w http.ResponseWriter, r *http.Request, name string, info fs.FileInfo, member string) {
	if r.URL.Query().Get("raw") != "1" && wantsDocument(r.Header) {
		s.serveMemberTable(w, r, name, info, member)
		return
	}
	s.serveMemberCSV(w, r, name, info, member)
}

func (s fileServer) serveMemberTable(w http.ResponseWriter, r *http.Request, name string, info fs.FileInfo, member string) {
	var (
		header []string
		body   [][]string
		total  int64
		schema string
		err    error
	)
	if isXLSX(name) {
		header, body, total, err = s.xlsxMemberRows(name, info, member)
	} else {
		header, body, total, schema, err = s.sqliteMemberData(name, info, member, maxTableRows)
	}
	if err != nil {
		log.Printf("member %s of %s: %v", member, name, err)
		http.NotFound(w, r)
		return
	}

	sheet := makeSheet("", header, body)
	if omitted := int(total) - len(sheet.Rows); omitted > 0 {
		sheet.Omitted = omitted
	}
	cols := len(sheet.ColNums)

	page := tablePage{
		Title:   member,
		Crumbs:  s.breadcrumbs(name),
		RawHref: (&url.URL{Path: member, RawQuery: "raw=1"}).String(),
		Summary: fmt.Sprintf("%d row%s × %d column%s", total, plural(int(total)), cols, plural(cols)),
		Sheets:  []sheetView{sheet},
	}
	if schema != "" {
		page.Schema = highlightSchema(schema)
	}
	s.renderTablePage(w, r, name, info, page)
}

func (s fileServer) xlsxMemberRows(name string, info fs.FileInfo, member string) (header []string, body [][]string, total int64, err error) {
	wb, err := s.openXLSX(name, info)
	if err != nil {
		return nil, nil, 0, err
	}
	defer wb.Close()

	rows, err := wb.GetRows(member)
	if err != nil {
		return nil, nil, 0, err
	}
	if len(rows) > 0 {
		header, body = rows[0], rows[1:]
	}
	return header, body, int64(len(body)), nil
}

// sqliteMemberData fetches one table's display rows, total count, and its
// schema (the CREATE statements for the table and everything attached to it).
func (s fileServer) sqliteMemberData(name string, info fs.FileInfo, member string, limit int) (header []string, body [][]string, total int64, schema string, err error) {
	db, cleanup, err := s.openSQLite(name, info)
	if err != nil {
		return nil, nil, 0, "", err
	}
	defer cleanup()

	quoted := quoteSQLiteIdent(member)
	if err := db.QueryRow("SELECT COUNT(*) FROM " + quoted).Scan(&total); err != nil {
		return nil, nil, 0, "", err
	}

	rows, err := db.Query(fmt.Sprintf("SELECT * FROM %s LIMIT %d", quoted, limit))
	if err != nil {
		return nil, nil, 0, "", err
	}
	defer rows.Close()

	header, err = rows.Columns()
	if err != nil {
		return nil, nil, 0, "", err
	}
	body, err = scanSQLiteRows(rows, len(header), formatSQLiteValue)
	if err != nil {
		return nil, nil, 0, "", err
	}

	schemaRows, err := db.Query(
		"SELECT sql FROM sqlite_master WHERE tbl_name = ? AND sql IS NOT NULL ORDER BY rowid", member)
	if err != nil {
		return nil, nil, 0, "", err
	}
	defer schemaRows.Close()

	var stmts []string
	for schemaRows.Next() {
		var stmt string
		if err := schemaRows.Scan(&stmt); err != nil {
			return nil, nil, 0, "", err
		}
		stmts = append(stmts, prettySQL(stmt)+";")
	}
	schema = strings.Join(stmts, "\n\n")
	return header, body, total, schema, schemaRows.Err()
}

// prettySQL reformats a one-line CREATE TABLE statement so each column and
// constraint sits on its own line. Statements that are already multi-line
// (someone formatted them deliberately) and other statement kinds pass
// through untouched.
func prettySQL(stmt string) string {
	upper := strings.ToUpper(strings.TrimSpace(stmt))
	if strings.Contains(stmt, "\n") ||
		(!strings.HasPrefix(upper, "CREATE TABLE") && !strings.HasPrefix(upper, "CREATE VIRTUAL TABLE")) {
		return stmt
	}

	open, end, items := splitColumnList(stmt)
	if open < 0 || len(items) < 2 {
		return stmt
	}

	var b strings.Builder
	b.WriteString(strings.TrimSpace(stmt[:open]))
	b.WriteString(" (\n")
	for i, item := range items {
		b.WriteString("  ")
		b.WriteString(strings.TrimSpace(item))
		if i < len(items)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString(")")
	if tail := strings.TrimSpace(stmt[end+1:]); tail != "" {
		b.WriteString(" " + tail)
	}
	return b.String()
}

// splitColumnList locates the outermost parenthesized list in stmt and
// splits it on top-level commas, respecting nested parentheses, SQL string
// literals with doubled-quote escapes, and quoted identifiers (double
// quotes, backticks, brackets).
func splitColumnList(stmt string) (open, end int, items []string) {
	open, end = -1, -1
	depth := 0
	start := 0
	var quote byte
	for i := 0; i < len(stmt); i++ {
		c := stmt[i]
		if quote != 0 {
			switch {
			case quote == '[' && c == ']':
				quote = 0
			case c == quote:
				if i+1 < len(stmt) && stmt[i+1] == quote {
					i++ // doubled quote: escape, still inside
				} else {
					quote = 0
				}
			}
			continue
		}
		switch c {
		case '\'', '"', '`':
			quote = c
		case '[':
			quote = '['
		case '(':
			depth++
			if depth == 1 && open < 0 {
				open = i
				start = i + 1
			}
		case ')':
			depth--
			if depth == 0 && open >= 0 && end < 0 {
				end = i
				items = append(items, stmt[start:i])
			}
		case ',':
			if depth == 1 && end < 0 {
				items = append(items, stmt[start:i])
				start = i + 1
			}
		}
	}
	if end < 0 {
		return -1, -1, nil
	}
	return open, end, items
}

// highlightSchema renders SQL schema text with chroma, without line numbers.
func highlightSchema(schema string) template.HTML {
	escaped := template.HTML("<pre>" + template.HTMLEscapeString(schema) + "</pre>")

	lexer := lexers.Get("sql")
	if lexer == nil {
		return escaped
	}
	iterator, err := chroma.Coalesce(lexer).Tokenise(nil, schema)
	if err != nil {
		return escaped
	}
	var buf bytes.Buffer
	if err := chromahtml.New().Format(&buf, styles.Get("github"), iterator); err != nil {
		return escaped
	}
	return template.HTML(buf.String())
}

func (s fileServer) serveMemberCSV(w http.ResponseWriter, r *http.Request, name string, info fs.FileInfo, member string) {
	if isXLSX(name) {
		wb, err := s.openXLSX(name, info)
		if err != nil {
			log.Printf("open xlsx %s: %v", name, err)
			http.NotFound(w, r)
			return
		}
		defer wb.Close()

		rows, err := wb.GetRows(member)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		writeCSV(w, r, member, func(cw *csv.Writer) error {
			for _, row := range rows {
				if err := cw.Write(row); err != nil {
					return err
				}
			}
			return nil
		})
		return
	}

	db, cleanup, err := s.openSQLite(name, info)
	if err != nil {
		log.Printf("open sqlite %s: %v", name, err)
		http.NotFound(w, r)
		return
	}
	defer cleanup()

	rows, err := db.Query("SELECT * FROM " + quoteSQLiteIdent(member))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		http.Error(w, "cannot read table", http.StatusInternalServerError)
		return
	}

	writeCSV(w, r, member, func(cw *csv.Writer) error {
		if err := cw.Write(cols); err != nil {
			return err
		}
		for rows.Next() {
			cells, err := scanSQLiteRow(rows, len(cols), csvSQLiteValue)
			if err != nil {
				return err
			}
			if err := cw.Write(cells); err != nil {
				return err
			}
		}
		return rows.Err()
	})
}

// writeCSV streams a member as text/csv; the write callback runs after
// headers are committed, so failures mid-stream can only be logged.
func writeCSV(w http.ResponseWriter, r *http.Request, member string, write func(*csv.Writer) error) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", member+".csv"))
	if r.Method == http.MethodHead {
		return
	}
	cw := csv.NewWriter(w)
	if err := write(cw); err != nil {
		log.Printf("write csv for %s: %v", member, err)
		return
	}
	cw.Flush()
}

func (s fileServer) openXLSX(name string, info fs.FileInfo) (*excelize.File, error) {
	if info.Size() > maxXLSXBytes {
		return nil, fmt.Errorf("workbook too large (%d bytes)", info.Size())
	}
	data, err := fs.ReadFile(s.fsys, name)
	if err != nil {
		return nil, err
	}
	return excelize.OpenReader(bytes.NewReader(data))
}

func (s fileServer) xlsxMembers(name string, info fs.FileInfo) ([]containerMember, error) {
	wb, err := s.openXLSX(name, info)
	if err != nil {
		return nil, err
	}
	defer wb.Close()

	var members []containerMember
	for _, sheet := range wb.GetSheetList() {
		rows, err := wb.GetRows(sheet)
		if err != nil {
			return nil, err
		}
		n := max(len(rows)-1, 0)
		members = append(members, containerMember{Name: sheet, Detail: fmt.Sprintf("%d row%s", n, plural(n))})
	}
	return members, nil
}

// openSQLite opens a database read-only. On-disk databases are opened in
// place, with no size limit — sqlite pages in only what a query touches.
// Archive-backed databases are copied to a temp file first (the driver
// needs a real path), capped at maxSQLiteBytes; the returned cleanup closes
// the handle and removes any copy.
func (s fileServer) openSQLite(name string, info fs.FileInfo) (*sql.DB, func(), error) {
	if s.dir != "" {
		dsn := &url.URL{
			Scheme:   "file",
			Path:     filepath.ToSlash(filepath.Join(s.dir, filepath.FromSlash(name))),
			RawQuery: "mode=ro",
		}
		db, err := sql.Open("sqlite3", dsn.String())
		if err == nil {
			if err = db.Ping(); err == nil {
				return db, func() { db.Close() }, nil
			}
			db.Close()
		}
		log.Printf("open sqlite %s in place: %v (falling back to copy)", name, err)
	}

	if info.Size() > maxSQLiteBytes {
		return nil, nil, fmt.Errorf("database too large (%d bytes)", info.Size())
	}

	src, err := s.fsys.Open(name)
	if err != nil {
		return nil, nil, err
	}
	defer src.Close()

	tmp, err := os.CreateTemp("", "localmd-sqlite-*.db")
	if err != nil {
		return nil, nil, err
	}
	removeTmp := func() {
		if err := os.Remove(tmp.Name()); err != nil {
			log.Printf("remove temp db %s: %v", tmp.Name(), err)
		}
	}

	n, err := io.Copy(tmp, io.LimitReader(src, maxSQLiteBytes+1))
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil && n > maxSQLiteBytes {
		err = fmt.Errorf("database exceeds %d bytes", int64(maxSQLiteBytes))
	}
	if err != nil {
		removeTmp()
		return nil, nil, err
	}

	db, err := sql.Open("sqlite3", "file:"+tmp.Name()+"?mode=ro&immutable=1")
	if err != nil {
		removeTmp()
		return nil, nil, err
	}
	return db, func() { db.Close(); removeTmp() }, nil
}

// sqliteMemberList lists tables and views. With withStats it also computes
// each member's row/column counts and footprint synchronously — appropriate
// only for small (archive-backed) databases; large on-disk ones get their
// stats filled in asynchronously by statsJS instead.
func sqliteMemberList(db *sql.DB, withStats bool) ([]containerMember, error) {
	rows, err := db.Query(`SELECT name, type FROM sqlite_master
		WHERE type IN ('table', 'view') AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []containerMember
	var kinds []string
	for rows.Next() {
		var m containerMember
		var kind string
		if err := rows.Scan(&m.Name, &kind); err != nil {
			return nil, err
		}
		members = append(members, m)
		kinds = append(kinds, kind)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range members {
		if withStats {
			members[i].Detail, members[i].Bytes = sqliteTableStat(db, members[i].Name, kinds[i])
		} else {
			members[i].Detail = kinds[i]
		}
	}
	return members, nil
}

// sqliteTableStat computes one member's listing metadata: "N rows × M
// columns" (prefixed for views) and its on-disk footprint including indexes
// via dbstat (empty without the sqlite_dbstat build tag).
func sqliteTableStat(db *sql.DB, member, kind string) (detail, footprint string) {
	detail = kind

	var size int64
	if err := db.QueryRow(`SELECT COALESCE(SUM(s.pgsize), 0)
		FROM dbstat('main', 1) s JOIN sqlite_master m ON s.name = m.name
		WHERE m.tbl_name = ?`, member).Scan(&size); err == nil && size > 0 {
		footprint = humanSize(size)
	}

	var count int64
	if err := db.QueryRow("SELECT COUNT(*) FROM " + quoteSQLiteIdent(member)).Scan(&count); err != nil {
		return detail, footprint // e.g. a view over a missing table
	}
	var cols int
	if err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info(?)", member).Scan(&cols); err != nil {
		return detail, footprint
	}

	detail = fmt.Sprintf("%d row%s × %d column%s", count, plural(int(count)), cols, plural(cols))
	if kind == "view" {
		detail = "view · " + detail
	}
	return detail, footprint
}

// serveSQLiteStat answers the async stat fetches issued by statsJS: metadata
// for a single table as JSON.
func (s fileServer) serveSQLiteStat(w http.ResponseWriter, r *http.Request, name string, info fs.FileInfo) {
	member := r.URL.Query().Get("stat")

	db, cleanup, err := s.openSQLite(name, info)
	if err != nil {
		http.Error(w, "cannot open database", http.StatusInternalServerError)
		return
	}
	defer cleanup()

	var kind string
	if err := db.QueryRow("SELECT type FROM sqlite_master WHERE name = ?", member).Scan(&kind); err != nil {
		http.NotFound(w, r)
		return
	}

	detail, footprint := sqliteTableStat(db, member, kind)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"detail": detail, "bytes": footprint})
}

func quoteSQLiteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func scanSQLiteRows(rows *sql.Rows, cols int, format func(any) string) ([][]string, error) {
	var body [][]string
	for rows.Next() {
		cells, err := scanSQLiteRow(rows, cols, format)
		if err != nil {
			return nil, err
		}
		body = append(body, cells)
	}
	return body, rows.Err()
}

func scanSQLiteRow(rows *sql.Rows, cols int, format func(any) string) ([]string, error) {
	holders := make([]any, cols)
	for i := range holders {
		holders[i] = new(any)
	}
	if err := rows.Scan(holders...); err != nil {
		return nil, err
	}
	cells := make([]string, cols)
	for i, h := range holders {
		cells[i] = format(*h.(*any))
	}
	return cells, nil
}

// csvSQLiteValue formats a value for CSV export: faithful, no truncation.
func csvSQLiteValue(v any) string {
	switch val := v.(type) {
	case nil:
		return ""
	case []byte:
		return string(val)
	default:
		return fmt.Sprint(val)
	}
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

var tableTemplate = template.Must(template.New("table").Parse(dataTableDefine + `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<link rel="icon" href="/` + assetPrefix + `/favicon.png">
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
` + dataTableCSS + `</style>
` + dropJS + `</head>
<body>
<nav>{{range $i, $c := .Crumbs}}{{if gt $i 1}}<span class="sep">/</span>{{end}}<a href="{{$c.Href}}">{{$c.Name}}</a>{{end}}{{if gt (len .Crumbs) 1}}<span class="sep">/</span>{{end}}<span class="file">{{.Title}}</span><a class="raw" href="{{.RawHref}}">raw</a></nav>
<p class="summary">{{.Summary}}</p>
{{range .Sheets}}{{template "datatable" .}}{{end}}{{if .Schema}}<h2 class="sheet">schema</h2>
<div class="schema">{{.Schema}}</div>
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
	QueryForm  bool
	Query      string
	QueryError string
	QuerySheet *sheetView
	StatsAsync bool
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

// dropJS lets any rendered page accept a drag-and-dropped file: the file is
// uploaded to the drop endpoint (with a floating progress monitor, since
// large files take a while) and the browser navigates to the stored copy,
// which renders through the ordinary pipeline like any other file.
var dropJS = `<script>
(function () {
  var maxDrop = ` + strconv.Itoa(maxDropBytes) + `;
  var box, msg, bar;

  function human(n) {
    var units = ["B", "KB", "MB", "GB"], i = 0;
    while (n >= 1024 && i < units.length - 1) { n /= 1024; i++; }
    return (i ? n.toFixed(1) : n) + " " + units[i];
  }

  function panel() {
    if (box) return;
    box = document.createElement("div");
    box.style.cssText = "position:fixed;left:50%;bottom:2rem;transform:translateX(-50%);" +
      "background:#1f2328;color:#fff;padding:0.6rem 1rem;border-radius:8px;" +
      "font:0.85rem -apple-system,BlinkMacSystemFont,sans-serif;box-shadow:0 4px 12px rgba(0,0,0,0.35);" +
      "z-index:9999;min-width:18rem;max-width:80vw";
    msg = document.createElement("div");
    msg.style.cssText = "overflow:hidden;text-overflow:ellipsis;white-space:nowrap";
    bar = document.createElement("div");
    bar.style.cssText = "height:4px;background:#2b8aff;border-radius:2px;margin-top:0.45rem;width:0%";
    box.appendChild(msg);
    box.appendChild(bar);
    box.onclick = hide;
    document.body.appendChild(box);
  }

  function show(text, frac) {
    panel();
    box.style.display = "block";
    bar.style.background = "#2b8aff";
    msg.textContent = text;
    bar.style.width = (frac == null ? 0 : Math.round(frac * 100)) + "%";
  }

  function fail(text) {
    show(text, 1);
    bar.style.background = "#f85149";
    setTimeout(hide, 8000);
  }

  function hide() { if (box) box.style.display = "none"; }

  addEventListener("dragover", function (e) { e.preventDefault(); });
  addEventListener("drop", function (e) {
    e.preventDefault();
    var f = e.dataTransfer && e.dataTransfer.files && e.dataTransfer.files[0];
    if (!f) return;
    if (f.size > maxDrop) {
      fail(f.name + " is too large: " + human(f.size) + " (limit " + human(maxDrop) + ")");
      return;
    }

    var xhr = new XMLHttpRequest();
    xhr.open("POST", "/` + assetPrefix + `/drop?name=" + encodeURIComponent(f.name));
    xhr.upload.onprogress = function (ev) {
      if (ev.lengthComputable) {
        show("uploading " + f.name + " — " + human(ev.loaded) + " of " + human(ev.total), ev.loaded / ev.total);
      }
    };
    xhr.onload = function () {
      if (xhr.status === 200) {
        show("opening " + f.name + " …", 1);
        location.href = xhr.responseText;
      } else {
        fail(f.name + ": " + (xhr.responseText || xhr.status + " " + xhr.statusText).trim());
      }
    };
    xhr.onerror = function () { fail(f.name + ": upload failed"); };
    show("uploading " + f.name + " …", 0);
    xhr.send(f);
  });
})();
</script>
`

// dataTableDefine is the shared "datatable" sub-template rendering one
// sheetView as a coordinate-framed table; parsed into both the directory
// template (for query results) and the table template.
const dataTableDefine = `{{define "datatable"}}{{if .Name}}<h2 class="sheet">{{.Name}}</h2>
{{end}}{{if .Summary}}<p class="summary">{{.Summary}}</p>
{{end}}{{if .Header}}<div class="tablewrap">
<table class="data">
<tr class="coords"><td class="rownum"></td>{{range .ColNums}}<td class="colnum">{{.}}</td>{{end}}</tr>
<tr><th class="corner"></th>{{range .Header}}<th>{{.}}</th>{{end}}</tr>
{{range .Rows}}<tr><td class="rownum">{{.N}}</td>{{range .Cells}}<td>{{.}}</td>{{end}}</tr>
{{end}}</table>
</div>
{{else}}<p class="summary">(empty)</p>
{{end}}{{if .Omitted}}<p class="summary">&hellip; and {{.Omitted}} more rows</p>
{{end}}{{end}}
`

// dataTableCSS styles the coordinate-framed data tables plus the sqlite
// query form; shared by the directory and table templates.
const dataTableCSS = `  p.summary { color: #57606a; font-size: 0.85rem; }
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
  div.schema { margin-top: 0.5rem; font-size: 0.85rem; overflow-x: auto; }
  form.query { margin: 0 0 1rem; }
  form.query input {
    width: min(48em, 100%);
    font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
    font-size: 0.85rem;
    padding: 0.35rem 0.5rem;
    border: 1px solid #d0d7de;
    border-radius: 6px;
    box-sizing: border-box;
  }
  p.queryerror {
    color: #cf222e;
    font-size: 0.85rem;
    font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  }
`

var directoryTemplate = template.Must(template.New("directory").Parse(dataTableDefine + `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<link rel="icon" href="/` + assetPrefix + `/favicon.png">
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
` + dataTableCSS + `</style>
` + dropJS + `</head>
<body>
<nav>{{range $i, $c := .Crumbs}}{{if gt $i 1}}<span class="sep">/</span>{{end}}<a href="{{$c.Href}}">{{$c.Name}}</a>{{end}}{{if .RawHref}}<a class="raw" href="{{.RawHref}}">raw</a>{{end}}</nav>
{{if .Blurb}}<p class="blurb">{{.Blurb}}</p>
{{end}}{{if .QueryForm}}<form class="query" method="get" action="">
<input type="text" name="q" value="{{.Query}}" placeholder="SQL query, e.g. select * from some_table limit 10" spellcheck="false" autocomplete="off">
</form>
{{with .QueryError}}<p class="queryerror">{{.}}</p>
{{end}}{{with .QuerySheet}}{{template "datatable" .}}
{{end}}{{end}}{{if .StatsAsync}}<p class="summary" id="statprog"></p>
{{end}}{{if .SortLinks}}<div class="sort">sort: {{range $i, $l := .SortLinks}}{{if $i}} &middot; {{end}}<a {{if $l.Active}}class="active" {{end}}href="{{$l.Href}}">{{$l.Label}}{{if $l.Active}} {{$l.Arrow}}{{end}}</a>{{end}}</div>
{{end}}
{{define "rows"}}{{range .}}<tr data-name="{{.Name}}">{{if .IsDir}}<td class="dname"><a href="{{.Href}}" title="{{.Name}}">{{.Name}}</a></td><td class="blurb"{{with .Blurb}} title="{{.}}"{{end}}>{{.Blurb}}</td>{{else}}<td class="fname" colspan="2"><a href="{{.Href}}" title="{{.Name}}">{{.Name}}</a></td>{{end}}<td class="meta">{{.Size}}</td><td class="meta">{{.ModTime}}</td></tr>
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
{{if .StatsAsync}}` + statsJS + `{{end}}</body>
</html>
`))

// statsJS fills in per-table stats on sqlite listing pages one table at a
// time, with a progress indicator, so listings of huge databases render
// instantly instead of waiting on every COUNT(*).
const statsJS = `<script>
(function () {
  var prog = document.getElementById("statprog");
  var rows = Array.prototype.slice.call(document.querySelectorAll("table.listing tr[data-name]"))
    .filter(function (tr) { return tr.getAttribute("data-name") !== ".."; });
  if (!rows.length) { if (prog) prog.remove(); return; }
  function next(i) {
    if (i >= rows.length) { prog.remove(); return; }
    var name = rows[i].getAttribute("data-name");
    var pct = Math.round(100 * i / rows.length);
    prog.textContent = "computing table stats — " + (i + 1) + " of " + rows.length + ": " + name + " …";
    prog.style.background = "linear-gradient(to right, #e7f0fa " + pct + "%, transparent " + pct + "%)";
    fetch(location.pathname + "?stat=" + encodeURIComponent(name))
      .then(function (r) { if (!r.ok) throw new Error(r.status); return r.json(); })
      .then(function (st) {
        var metas = rows[i].querySelectorAll("td.meta");
        if (metas[0]) metas[0].textContent = st.detail;
        if (metas[1]) metas[1].textContent = st.bytes;
      })
      .catch(function () {})
      .then(function () { next(i + 1); });
  }
  next(0);
})();
</script>
`

var pandocHeader = `<link rel="icon" href="/` + assetPrefix + `/favicon.png">
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
` + dropJS
