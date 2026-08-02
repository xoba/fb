package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/csv"
	"encoding/json"
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
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing/fstest"
	"time"
	"unicode"
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

	// Converted image previews are subresources, also ahead of the gating.
	if r.URL.Query().Get("jpeg") == "1" && needsJPEGPreview(name) {
		s.serveHEICPreview(w, r, name, info)
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
		case videoExts[ext]:
			s.serveVideo(w, r, name, info)
			return
		case ext == ".plist":
			s.servePlist(w, r, name, info)
			return
		case ext == ".json":
			s.serveJSON(w, r, name, info)
			return
		case ext == ".typ":
			s.serveTypst(w, r, name, info)
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
	switch strings.ToLower(path.Ext(viewName(name))) {
	case ".zip", ".jar": // jars are zips
		return true
	}
	return isTarName(name)
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

// serveVersion reports which build is running: the git revision stamped into
// the binary by go build (absent under go run and go test), the commit time,
// and whether the working tree was dirty. service.sh compares the revision
// against git HEAD to confirm a redeploy took effect.
func serveVersion(w http.ResponseWriter) {
	revision, vcsTime, modified := "unknown", "unknown", "unknown"
	goVersion := runtime.Version()
	if bi, ok := debug.ReadBuildInfo(); ok {
		goVersion = bi.GoVersion
		for _, kv := range bi.Settings {
			switch kv.Key {
			case "vcs.revision":
				revision = kv.Value
			case "vcs.time":
				vcsTime = kv.Value
			case "vcs.modified":
				modified = kv.Value
			}
		}
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "revision: %s\nvcs.time: %s\nmodified: %s\ngo: %s\n", revision, vcsTime, modified, goVersion)
}

// serveInternal handles the reserved /_localmd/ namespace: embedded assets,
// the drag-and-drop upload endpoint, browsing of dropped files, and the
// health and version probes.
func (s fileServer) serveInternal(w http.ResponseWriter, r *http.Request, name string) {
	if name == assetPrefix+"/healthz" {
		preventCaching(w.Header(), r.Header)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintln(w, "ok")
		return
	}

	if name == assetPrefix+"/version" {
		preventCaching(w.Header(), r.Header)
		serveVersion(w)
		return
	}

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
	".rst":   "rst",
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
			// Markdown pages render clean, without the healthz/version
			// footer that closes every other page.
			if format != pandocFormats[".md"] {
				if p, err := pandocFooterFile(); err == nil {
					args = append(args, "--include-after-body", p)
				}
			}
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
	footerOnce sync.Once
	footerPath string
	footerErr  error
)

// pandocFooterFile writes the shared page footer to a temp file, once per
// process, for pandoc --include-after-body (which only accepts files).
func pandocFooterFile() (string, error) {
	footerOnce.Do(func() {
		f, err := os.CreateTemp("", "localmd-pandoc-footer-*.html")
		if err != nil {
			footerErr = err
			return
		}
		if _, err := f.WriteString(pageFooter); err != nil {
			f.Close()
			footerErr = err
			return
		}
		footerErr = f.Close()
		footerPath = f.Name()
	})
	return footerPath, footerErr
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

// videoExts lists video types shown on a player page with file details; the
// browser's decoder reports duration and dimensions client-side.
var videoExts = map[string]bool{
	".m4v":  true,
	".mov":  true,
	".mp4":  true,
	".webm": true,
}

func (s fileServer) serveVideo(w http.ResponseWriter, r *http.Request, name string, info fs.FileInfo) {
	rawHref := (&url.URL{Path: path.Base(name), RawQuery: "raw=1"}).String()
	page := imagePage{
		Title:   path.Base(name),
		Crumbs:  s.breadcrumbs(path.Dir(name)),
		RawHref: rawHref,
		Src:     rawHref,
	}
	page.Rows = append(page.Rows,
		kvRow{K: "file size", V: fmt.Sprintf("%s (%d bytes)", humanSize(info.Size()), info.Size())})
	if !info.ModTime().IsZero() {
		page.Rows = append(page.Rows, kvRow{K: "modified", V: info.ModTime().Format("2006-01-02 15:04:05")})
	}
	if mt := mime.TypeByExtension(strings.ToLower(path.Ext(viewName(name)))); mt != "" {
		page.Rows = append(page.Rows, kvRow{K: "format", V: mt})
	}

	var buf bytes.Buffer
	if err := videoTemplate.Execute(&buf, page); err != nil {
		log.Printf("render video %s: %v", name, err)
		http.Error(w, "cannot render video page", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeContent(w, r, "index.html", info.ModTime(), bytes.NewReader(buf.Bytes()))
}

var videoTemplate = template.Must(template.New("video").Parse(`<!DOCTYPE html>
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
    margin: 0;
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
  /* The window sets the display size, not the movie's native dimensions:
     the player spans the window below the nav and the picture scales (up
     or down, aspect preserved) to fit inside it. */
  video.subject {
    display: block;
    width: 100%;
    height: calc(100vh - 9rem);
    height: calc(100dvh - 9rem);
    object-fit: contain;
    border: 1px solid #d0d7de;
    border-radius: 6px;
    background: #000;
  }
  table.kv { border-collapse: collapse; font-size: 0.85rem; margin-top: 1rem; }
  table.kv td { padding: 0.25rem 1.25rem 0.25rem 0; vertical-align: top; }
  table.kv td.k { color: #57606a; white-space: nowrap; }
</style>
` + dropJS + `</head>
<body>
<nav>{{range $i, $c := .Crumbs}}{{if gt $i 1}}<span class="sep">/</span>{{end}}<a href="{{$c.Href}}">{{$c.Name}}</a>{{end}}{{if gt (len .Crumbs) 1}}<span class="sep">/</span>{{end}}<span class="file">{{.Title}}</span><a class="raw" href="{{.RawHref}}">raw</a></nav>
<video class="subject" controls preload="metadata" src="{{.Src}}"></video>
<table class="kv" id="vmeta">
{{range .Rows}}<tr><td class="k">{{.K}}</td><td>{{.V}}</td></tr>
{{end}}</table>
<script>
document.querySelector("video").addEventListener("loadedmetadata", function () {
  var v = this, t = document.getElementById("vmeta");
  function row(k, val) {
    var tr = t.insertRow();
    var td = tr.insertCell(); td.className = "k"; td.textContent = k;
    tr.insertCell().textContent = val;
  }
  if (isFinite(v.duration)) {
    var s = Math.round(v.duration);
    row("duration", Math.floor(s / 60) + ":" + ("0" + (s % 60)).slice(-2) + " (" + v.duration.toFixed(2) + " s)");
  }
  if (v.videoWidth) row("dimensions", v.videoWidth + " × " + v.videoHeight);
});
</script>
` + pageFooter + `</body>
</html>
`))

// imageExts lists image types shown on a viewer page with a technical
// readout (dimensions, file size, EXIF) beneath the image. The image itself
// is fetched as a subresource, which gets the raw bytes as usual.
var imageExts = map[string]bool{
	".bmp":  true,
	".gif":  true,
	".heic": true,
	".heif": true,
	".jpeg": true,
	".jpg":  true,
	".png":  true,
	".tif":  true,
	".tiff": true,
	".webp": true,
}

// isHEIC reports whether name is a HEIC/HEIF image, which browsers other
// than Safari cannot display and Go cannot decode: both the page's preview
// and its metadata come from a JPEG conversion via macOS's sips.
func isHEIC(name string) bool {
	switch strings.ToLower(path.Ext(viewName(name))) {
	case ".heic", ".heif":
		return true
	}
	return false
}

// needsJPEGPreview reports whether an image must be converted for display:
// browsers cannot show HEIC or TIFF inline.
func needsJPEGPreview(name string) bool {
	switch strings.ToLower(path.Ext(viewName(name))) {
	case ".heic", ".heif", ".tif", ".tiff":
		return true
	}
	return false
}

var (
	heicCacheOnce sync.Once
	heicCachePath string
	heicCacheErr  error
)

// heicJPEG converts a HEIC file to JPEG with sips (which preserves EXIF,
// including GPS) and returns the path of the converted file, cached per
// (path, size, mtime) so repeat views are instant.
func (s fileServer) heicJPEG(ctx context.Context, name string, info fs.FileInfo) (string, error) {
	heicCacheOnce.Do(func() {
		heicCachePath, heicCacheErr = os.MkdirTemp("", "localmd-heic-")
	})
	if heicCacheErr != nil {
		return "", heicCacheErr
	}

	key := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%d",
		s.displayPath(name), info.Size(), info.ModTime().UnixNano())))
	out := filepath.Join(heicCachePath, fmt.Sprintf("%x.jpg", key[:12]))
	if _, err := os.Stat(out); err == nil {
		return out, nil
	}

	sips, err := exec.LookPath("sips")
	if err != nil {
		return "", fmt.Errorf("heic conversion needs sips: %w", err)
	}

	in := filepath.Join(s.dir, filepath.FromSlash(name))
	if s.dir == "" { // archive-backed: sips needs a real file
		data, err := fs.ReadFile(s.fsys, name)
		if err != nil {
			return "", err
		}
		tmp, err := os.CreateTemp(heicCachePath, "in-*.heic")
		if err != nil {
			return "", err
		}
		_, err = tmp.Write(data)
		if closeErr := tmp.Close(); err == nil {
			err = closeErr
		}
		defer os.Remove(tmp.Name())
		if err != nil {
			return "", err
		}
		in = tmp.Name()
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, sips, "-s", "format", "jpeg", in, "--out", out)
	if outBytes, err := cmd.CombinedOutput(); err != nil {
		os.Remove(out)
		return "", fmt.Errorf("sips: %s: %w", strings.TrimSpace(string(outBytes)), err)
	}
	return out, nil
}

// serveHEICPreview streams the JPEG conversion, used as the <img> source on
// HEIC viewer pages.
func (s fileServer) serveHEICPreview(w http.ResponseWriter, r *http.Request, name string, info fs.FileInfo) {
	out, err := s.heicJPEG(r.Context(), name, info)
	if err != nil {
		log.Printf("heic preview %s: %v", name, err)
		s.serveRaw(w, r, name, info)
		return
	}
	f, err := os.Open(out)
	if err != nil {
		http.Error(w, "cannot read preview", http.StatusInternalServerError)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "image/jpeg")
	http.ServeContent(w, r, "preview.jpg", info.ModTime(), f)
}

// maxImageMetaBytes bounds how much of an image is read for metadata;
// dimensions and EXIF live near the start of the file.
const maxImageMetaBytes = 32 << 20

func (s fileServer) serveImage(w http.ResponseWriter, r *http.Request, name string, info fs.FileInfo) {
	heic := isHEIC(name)
	preview := needsJPEGPreview(name)

	var data []byte
	if heic {
		// Metadata (dimensions, EXIF, GPS) comes from the JPEG conversion,
		// which sips carries over from the original. (TIFFs keep their
		// original bytes here: Go reads TIFF metadata natively.)
		out, err := s.heicJPEG(r.Context(), name, info)
		if err != nil {
			log.Printf("heic %s: %v", name, err)
			s.serveRaw(w, r, name, info)
			return
		}
		if data, err = os.ReadFile(out); err != nil {
			http.Error(w, "cannot read preview", http.StatusInternalServerError)
			return
		}
	} else {
		f, err := s.fsys.Open(name)
		if err != nil {
			http.Error(w, "cannot read file", http.StatusInternalServerError)
			return
		}
		data, err = io.ReadAll(io.LimitReader(f, maxImageMetaBytes))
		f.Close()
		if err != nil {
			http.Error(w, "cannot read file", http.StatusInternalServerError)
			return
		}
	}

	rawHref := (&url.URL{Path: path.Base(name), RawQuery: "raw=1"}).String()
	page := imagePage{
		Title:   path.Base(name),
		Crumbs:  s.breadcrumbs(path.Dir(name)),
		RawHref: rawHref,
		Src:     rawHref,
	}
	if preview {
		page.Src = (&url.URL{Path: path.Base(name), RawQuery: "jpeg=1"}).String()
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
		if !heic {
			mimeType = "image/" + format // from actual content, not the extension
		}
		add("dimensions", fmt.Sprintf("%d × %d (%.1f MP)", cfg.Width, cfg.Height,
			float64(cfg.Width)*float64(cfg.Height)/1e6))
	}
	if heic {
		mimeType = "image/heic"
	}
	if preview {
		add("preview", "converted to JPEG with sips")
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
    margin: 0;
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
  /* The window sets the display size, not the image's native dimensions:
     the frame spans the window below the nav and the picture scales (up or
     down, aspect preserved) to fit inside it. */
  img.subject {
    display: block;
    width: 100%;
    height: calc(100vh - 9rem);
    height: calc(100dvh - 9rem);
    object-fit: contain;
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
{{end}}` + pageFooter + `</body>
</html>
`))

// highlightExts lists source-file extensions served as syntax-highlighted HTML
// pages when a browser navigates to them. Subresource fetches and non-browser
// clients (see wantsDocument), plus anything requested with ?raw=1, get the
// file verbatim — so .localmd.css and stylesheets referenced by served HTML
// keep working as real stylesheets. Deliberately absent: .md (pandoc), .html
// and .svg (the browser renders those), and .txt (prose reads better plain).
var highlightExts = map[string]bool{
	".ada":        true,
	".adb":        true,
	".ads":        true,
	".awk":        true,
	".bash":       true,
	".bat":        true,
	".c":          true,
	".coffee":     true,
	".erb":        true,
	".f":          true,
	".f90":        true,
	".feature":    true,
	".properties": true,
	".s":          true,
	".cc":         true,
	".clj":        true,
	".cpp":        true,
	".cs":         true,
	".css":        true,
	".dart":       true,
	".diff":       true,
	".el":         true,
	".erl":        true,
	".ex":         true,
	".exs":        true,
	".fish":       true,
	".go":         true,
	".gradle":     true,
	".graphql":    true,
	".groovy":     true,
	".h":          true,
	".hcl":        true,
	".hpp":        true,
	".hs":         true,
	".ini":        true,
	".java":       true,
	".jl":         true,
	".js":         true,
	".jsx":        true,
	".kt":         true,
	".lisp":       true,
	".lua":        true,
	".mf":         true,
	".mjs":        true,
	".nix":        true,
	".patch":      true,
	".php":        true,
	".pl":         true,
	".proto":      true,
	".ps1":        true,
	".py":         true,
	".r":          true,
	".rb":         true,
	".rs":         true,
	".scala":      true,
	".scss":       true,
	".sh":         true,
	".sql":        true,
	".svelte":     true,
	".swift":      true,
	".tex":        true,
	".tf":         true,
	".toml":       true,
	".ts":         true,
	".tsx":        true,
	".vue":        true,
	".xml":        true,
	".yaml":       true,
	".yml":        true,
	".zig":        true,
	".zsh":        true,
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

// serveJSON shows a JSON file pretty-printed and highlighted. Content that
// does not parse as a single JSON value (syntax errors, concatenated values)
// shows as-is; files too large to highlight display as plain text rather
// than downloading under their application/json type.
func (s fileServer) serveJSON(w http.ResponseWriter, r *http.Request, name string, info fs.FileInfo) {
	if info.Size() > maxHighlightBytes {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		s.serveRaw(w, r, name, info)
		return
	}

	data, err := fs.ReadFile(s.fsys, name)
	if err != nil {
		http.Error(w, "cannot read file", http.StatusInternalServerError)
		return
	}

	var pretty bytes.Buffer
	if err := json.Indent(&pretty, data, "", "  "); err == nil {
		data = pretty.Bytes()
	}

	s.renderSourcePage(w, r, name, info, path.Base(viewName(name)), string(data))
}

// serveTypst shows a Typst file reformatted by typstyle and highlighted.
// Without typstyle installed the file shows as written; erroneous sources
// need no fallback because typstyle passes them through unchanged.
func (s fileServer) serveTypst(w http.ResponseWriter, r *http.Request, name string, info fs.FileInfo) {
	if info.Size() > maxHighlightBytes {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		s.serveRaw(w, r, name, info)
		return
	}

	data, err := fs.ReadFile(s.fsys, name)
	if err != nil {
		http.Error(w, "cannot read file", http.StatusInternalServerError)
		return
	}

	if formatted, err := typstyleFormat(r.Context(), data); err == nil {
		data = formatted
	}

	s.renderSourcePage(w, r, name, info, path.Base(viewName(name)), string(data))
}

func typstyleFormat(ctx context.Context, data []byte) ([]byte, error) {
	typstyle, err := exec.LookPath("typstyle")
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, typstyle)
	cmd.Stdin = bytes.NewReader(data)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("typstyle: %s: %w", msg, err)
	}
	return out, nil
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

// codeStyle is the github style with Error tokens neutralized to plain
// text: files a lexer only partially understands (unusual dialects like
// plan9 assembly, embedded DSLs) should degrade to uncolored text rather
// than fill the page with red boxes.
var codeStyle = func() *chroma.Style {
	builder := styles.Get("github").Builder()
	builder.Add(chroma.Error, "#1f2328")
	s, err := builder.Build()
	if err != nil {
		return styles.Get("github")
	}
	return s
}()

// forcedLexers overrides chroma's filename matching where its default guess
// fits poorly (.s spans many assembler dialects, and the generic GAS lexer
// degrades better than ArmAsm) or where chroma has no match at all (.mf jar
// manifests are key: value pairs the properties lexer colors well).
var forcedLexers = map[string]string{
	".mf": "properties",
	".s":  "gas",
}

func highlightSource(filename, src string) (template.HTML, error) {
	lexer := lexers.Match(filename)
	if alias, ok := forcedLexers[strings.ToLower(path.Ext(filename))]; ok {
		if forced := lexers.Get(alias); forced != nil {
			lexer = forced
		}
	}
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
	if err := formatter.Format(&buf, codeStyle, iterator); err != nil {
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
` + pageFooter + `</body>
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

	if ref := r.URL.Query().Get("cell"); ref != "" {
		row, col, ok := parseCellCoord(ref)
		if !ok || row > len(rows) {
			http.NotFound(w, r)
			return
		}
		s.renderCellView(w, r, info, s.breadcrumbs(path.Dir(name)), path.Base(name),
			header, rows[row-1], row, col, (&url.URL{Path: path.Base(name)}).String())
		return
	}

	page := s.tablePageFor(name)
	page.Summary = fmt.Sprintf("%d rows × %d columns", len(rows), len(header))
	page.Sheets = []sheetView{makeSheet("", header, displayCells(rows), cellSelfHref)}
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
// meaningful even when rows are ragged. cellHref returns the href of the
// full-text view for a Cut cell's 1-based display coordinates; without one,
// Cut cells fall back to a plain ellipsis.
func makeSheet(name string, header []string, rows [][]tableCell, cellHref func(row, col int) string) sheetView {
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
		for j := range cells {
			if !cells[j].Cut {
				continue
			}
			if cellHref != nil {
				cells[j].More = cellHref(i+1, j+1)
			} else {
				cells[j].Text += "…"
			}
		}
		sheet.Rows[i] = tableRow{N: i + 1, Cells: cells}
	}
	return sheet
}

// cellSelfHref addresses a cell's full-text view on the current page URL.
func cellSelfHref(row, col int) string {
	return fmt.Sprintf("?cell=%d,%d", row, col)
}

// parseCellCoord parses a 1-based "row,col" reference from a table view's
// ?cell= parameter, bounded by the rows a table view can display.
func parseCellCoord(ref string) (row, col int, ok bool) {
	rs, cs, found := strings.Cut(ref, ",")
	if !found {
		return 0, 0, false
	}
	row, rErr := strconv.Atoi(rs)
	col, cErr := strconv.Atoi(cs)
	if rErr != nil || cErr != nil || row < 1 || col < 1 || row > maxTableRows {
		return 0, 0, false
	}
	return row, col, true
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
	Cells []tableCell
}

// tableCell is one displayed table cell. Cut marks text cut short at
// maxCellChars whose full content is plain text; makeSheet turns Cut cells
// into "… more" links by filling More with the full-text view's href. Link
// is the external URL the cell's full text names, when it names one.
type tableCell struct {
	Text string
	Cut  bool
	More string
	Link string
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
		members, page.Summary, err = s.xlsxMembers(name, info)
	} else {
		db, cleanup, dbErr := s.openSQLite(name, info)
		if dbErr == nil {
			defer cleanup()
			// On-disk databases can be huge: serve the listing instantly
			// with names only and let statsJS fill in per-table stats with
			// a progress indicator. Archive-backed ones are small; compute
			// synchronously.
			page.StatsAsync = s.dir != ""
			members, page.Summary, err = sqliteMemberList(db, !page.StatsAsync)

			page.QueryForm = true
			page.Query = strings.TrimSpace(r.URL.Query().Get("q"))
			if page.Query != "" {
				if ref := r.URL.Query().Get("cell"); ref != "" {
					s.serveQueryCell(w, r, db, name, info, page.Query, ref)
					return
				}
				query := page.Query
				sheet, qErr := sqliteQuerySheet(r.Context(), db, query, func(row, col int) string {
					return fmt.Sprintf("?q=%s&cell=%d,%d", url.QueryEscape(query), row, col)
				})
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
func sqliteQuerySheet(ctx context.Context, db *sql.DB, query string, cellHref func(row, col int) string) (sheetView, error) {
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

	var body [][]tableCell
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

	sheet := makeSheet("", cols, body, cellHref)
	sheet.Summary = fmt.Sprintf("%d row%s × %d column%s", len(body), plural(len(body)), len(cols), plural(len(cols)))
	if truncated {
		sheet.Summary = fmt.Sprintf("first %d rows × %d column%s", maxTableRows, len(cols), plural(len(cols)))
	}
	return sheet, nil
}

// serveQueryCell serves the full-text view of one cell of a query result,
// addressed by the 1-based display coordinates of the result sheet.
func (s fileServer) serveQueryCell(w http.ResponseWriter, r *http.Request, db *sql.DB, name string, info fs.FileInfo, query, ref string) {
	row, col, ok := parseCellCoord(ref)
	if !ok {
		http.NotFound(w, r)
		return
	}
	header, cells, err := sqliteQueryRow(r.Context(), db, query, row)
	if err != nil {
		log.Printf("query cell of %s: %v", name, err)
		http.NotFound(w, r)
		return
	}
	if cells == nil {
		http.NotFound(w, r)
		return
	}
	s.renderCellView(w, r, info, s.breadcrumbs(name), "query",
		header, cells, row, col, (&url.URL{RawQuery: "q=" + url.QueryEscape(query)}).String())
}

// sqliteQueryRow re-runs a query and returns the full text of its row'th
// result row (1-based); a nil row means the result has fewer rows.
func sqliteQueryRow(ctx context.Context, db *sql.DB, query string, row int) (header, cells []string, err error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	header, err = rows.Columns()
	if err != nil {
		return nil, nil, err
	}
	for i := 0; i < row; i++ {
		if !rows.Next() {
			return header, nil, rows.Err()
		}
	}
	cells, err = scanSQLiteRow(rows, len(header), csvSQLiteValue)
	return header, cells, err
}

// serveContainerMember serves one sheet or table: a table page for browser
// navigations, CSV bytes otherwise.
func (s fileServer) serveContainerMember(w http.ResponseWriter, r *http.Request, name string, info fs.FileInfo, member string) {
	if r.URL.Query().Get("raw") != "1" && wantsDocument(r.Header) {
		if ref := r.URL.Query().Get("cell"); ref != "" {
			s.serveMemberCell(w, r, name, info, member, ref)
			return
		}
		s.serveMemberTable(w, r, name, info, member)
		return
	}
	s.serveMemberCSV(w, r, name, info, member)
}

func (s fileServer) serveMemberTable(w http.ResponseWriter, r *http.Request, name string, info fs.FileInfo, member string) {
	var (
		header []string
		body   [][]tableCell
		total  int64
		schema string
		err    error
	)
	if isXLSX(name) {
		var rows [][]string
		header, rows, total, err = s.xlsxMemberRows(name, info, member)
		body = displayCells(rows)
	} else {
		header, body, total, schema, err = s.sqliteMemberData(name, info, member, maxTableRows)
	}
	if err != nil {
		log.Printf("member %s of %s: %v", member, name, err)
		http.NotFound(w, r)
		return
	}

	sheet := makeSheet("", header, body, cellSelfHref)
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
	rows = dropLeadingEmptyRows(rows)
	if len(rows) > 0 {
		header, body = rows[0], rows[1:]
	}
	return header, body, int64(len(body)), nil
}

// serveMemberCell serves the full-text view of one cell of a sheet or table,
// addressed by the 1-based display coordinates of the member table view.
func (s fileServer) serveMemberCell(w http.ResponseWriter, r *http.Request, name string, info fs.FileInfo, member, ref string) {
	row, col, ok := parseCellCoord(ref)
	if !ok {
		http.NotFound(w, r)
		return
	}

	var header, cells []string
	var err error
	if isXLSX(name) {
		var body [][]string
		header, body, _, err = s.xlsxMemberRows(name, info, member)
		if err == nil && row <= len(body) {
			cells = body[row-1]
		}
	} else {
		header, cells, err = s.sqliteMemberRow(name, info, member, row)
	}
	if err != nil {
		log.Printf("cell %s of %s: %v", member, name, err)
		http.NotFound(w, r)
		return
	}
	if cells == nil {
		http.NotFound(w, r)
		return
	}
	s.renderCellView(w, r, info, s.breadcrumbs(name), member,
		header, cells, row, col, (&url.URL{Path: member}).String())
}

// sqliteMemberRow fetches one row of a table, full text, by its 1-based
// position in the member table view. A nil row means the table is shorter.
func (s fileServer) sqliteMemberRow(name string, info fs.FileInfo, member string, row int) (header, cells []string, err error) {
	db, cleanup, err := s.openSQLite(name, info)
	if err != nil {
		return nil, nil, err
	}
	defer cleanup()

	rows, err := db.Query(fmt.Sprintf("SELECT * FROM %s LIMIT 1 OFFSET %d", quoteSQLiteIdent(member), row-1))
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	header, err = rows.Columns()
	if err != nil {
		return nil, nil, err
	}
	if !rows.Next() {
		return header, nil, rows.Err()
	}
	cells, err = scanSQLiteRow(rows, len(header), csvSQLiteValue)
	return header, cells, err
}

// sqliteMemberData fetches one table's display rows, total count, and its
// schema (the CREATE statements for the table and everything attached to it).
func (s fileServer) sqliteMemberData(name string, info fs.FileInfo, member string, limit int) (header []string, body [][]tableCell, total int64, schema string, err error) {
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
	if err := chromahtml.New().Format(&buf, codeStyle, iterator); err != nil {
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
			for _, row := range dropLeadingEmptyRows(rows) {
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

func (s fileServer) xlsxMembers(name string, info fs.FileInfo) ([]containerMember, string, error) {
	wb, err := s.openXLSX(name, info)
	if err != nil {
		return nil, "", err
	}
	defer wb.Close()

	var members []containerMember
	var total int
	for _, sheet := range wb.GetSheetList() {
		rows, err := wb.GetRows(sheet)
		if err != nil {
			return nil, "", err
		}
		n := max(len(dropLeadingEmptyRows(rows))-1, 0)
		total += n
		members = append(members, containerMember{Name: sheet, Detail: fmt.Sprintf("%d row%s", n, plural(n))})
	}
	summary := fmt.Sprintf("%d sheet%s · %d row%s", len(members), plural(len(members)), total, plural(total))
	return members, summary, nil
}

// dropLeadingEmptyRows discards blank rows above a sheet's data, so a sheet
// whose contents start below row 1 (a common layout when a chart or title
// occupies the top) yields its first populated row as the header rather than
// a blank one.
func dropLeadingEmptyRows(rows [][]string) [][]string {
	for len(rows) > 0 {
		for _, cell := range rows[0] {
			if cell != "" {
				return rows
			}
		}
		rows = rows[1:]
	}
	return rows
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

// sqliteMemberList lists tables and views, and formats the summary line for
// the listing header. With withStats it also computes each member's
// row/column counts and footprint synchronously — appropriate only for small
// (archive-backed) databases; large on-disk ones get their stats (and the
// summary's row/byte totals) filled in asynchronously by statsJS instead.
func sqliteMemberList(db *sql.DB, withStats bool) ([]containerMember, string, error) {
	rows, err := db.Query(`SELECT name, type FROM sqlite_master
		WHERE type IN ('table', 'view') AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	var members []containerMember
	var kinds []string
	for rows.Next() {
		var m containerMember
		var kind string
		if err := rows.Scan(&m.Name, &kind); err != nil {
			return nil, "", err
		}
		members = append(members, m)
		kinds = append(kinds, kind)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	var tables, views int
	for _, kind := range kinds {
		if kind == "view" {
			views++
		} else {
			tables++
		}
	}
	summary := fmt.Sprintf("%d table%s", tables, plural(tables))
	if views > 0 {
		summary += fmt.Sprintf(" · %d view%s", views, plural(views))
	}

	var totalRows, totalSize int64
	for i := range members {
		if withStats {
			st := sqliteTableStat(db, members[i].Name, kinds[i])
			members[i].Detail, members[i].Bytes = st.Detail, st.Bytes
			totalRows += st.Rows
			totalSize += st.Size
		} else {
			members[i].Detail = kinds[i]
		}
	}
	if withStats {
		summary += fmt.Sprintf(" · %d row%s", totalRows, plural(int(totalRows)))
		if totalSize > 0 {
			summary += " · " + humanSize(totalSize)
		}
	}
	return members, summary, nil
}

// sqliteStat is one member's listing metadata: formatted detail and
// footprint strings for the listing row, plus the raw numbers so statsJS
// can total them across tables for the summary line.
type sqliteStat struct {
	Detail string `json:"detail"`
	Bytes  string `json:"bytes"`
	Rows   int64  `json:"rows"`
	Size   int64  `json:"size"`
}

// sqliteTableStat computes one member's listing metadata: "N rows × M
// columns" (prefixed for views) and its on-disk footprint including indexes
// via dbstat (empty without the sqlite_dbstat build tag).
func sqliteTableStat(db *sql.DB, member, kind string) sqliteStat {
	st := sqliteStat{Detail: kind}

	var size int64
	if err := db.QueryRow(`SELECT COALESCE(SUM(s.pgsize), 0)
		FROM dbstat('main', 1) s JOIN sqlite_master m ON s.name = m.name
		WHERE m.tbl_name = ?`, member).Scan(&size); err == nil && size > 0 {
		st.Size = size
		st.Bytes = humanSize(size)
	}

	var count int64
	if err := db.QueryRow("SELECT COUNT(*) FROM " + quoteSQLiteIdent(member)).Scan(&count); err != nil {
		return st // e.g. a view over a missing table
	}
	var cols int
	if err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info(?)", member).Scan(&cols); err != nil {
		return st
	}

	st.Rows = count
	st.Detail = fmt.Sprintf("%d row%s × %d column%s", count, plural(int(count)), cols, plural(cols))
	if kind == "view" {
		st.Detail = "view · " + st.Detail
	}
	return st
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sqliteTableStat(db, member, kind))
}

func quoteSQLiteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func scanSQLiteRows[T any](rows *sql.Rows, cols int, format func(any) T) ([][]T, error) {
	var body [][]T
	for rows.Next() {
		cells, err := scanSQLiteRow(rows, cols, format)
		if err != nil {
			return nil, err
		}
		body = append(body, cells)
	}
	return body, rows.Err()
}

func scanSQLiteRow[T any](rows *sql.Rows, cols int, format func(any) T) ([]T, error) {
	holders := make([]any, cols)
	for i := range holders {
		holders[i] = new(any)
	}
	if err := rows.Scan(holders...); err != nil {
		return nil, err
	}
	cells := make([]T, cols)
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

// maxCellChars keeps giant text or blob columns from wrecking the table.
const maxCellChars = 200

// displayCell shapes one cell for table display, cutting text longer than
// maxCellChars. Cells cut from plain text are marked Cut so the table can
// link to the full content; text with binary junk just keeps the ellipsis.
// Cells whose text is an http(s) URL link out to it (a cut cell's visible
// prefix still links to the full URL).
func displayCell(text string) tableCell {
	link := cellURL(text)
	runes := []rune(text)
	if len(runes) <= maxCellChars {
		return tableCell{Text: text, Link: link}
	}
	if !isPlainText([]byte(text)) {
		return tableCell{Text: string(runes[:maxCellChars]) + "…"}
	}
	return tableCell{Text: string(runes[:maxCellChars]), Cut: true, Link: link}
}

// cellURL returns the URL a cell's text names — the text, trimmed, when it
// is a single well-formed absolute http(s) URL — or "" otherwise.
func cellURL(text string) string {
	trimmed := strings.TrimSpace(text)
	if strings.IndexFunc(trimmed, unicode.IsSpace) >= 0 {
		return ""
	}
	u, err := url.Parse(trimmed)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	return trimmed
}

// displayCells applies displayCell across raw string rows.
func displayCells(rows [][]string) [][]tableCell {
	out := make([][]tableCell, len(rows))
	for i, cells := range rows {
		out[i] = make([]tableCell, len(cells))
		for j, text := range cells {
			out[i][j] = displayCell(text)
		}
	}
	return out
}

func formatSQLiteValue(v any) tableCell {
	switch val := v.(type) {
	case nil:
		return tableCell{Text: "NULL"}
	case []byte:
		if !isPlainText(val) {
			return tableCell{Text: fmt.Sprintf("(%d-byte blob)", len(val))}
		}
		return displayCell(string(val))
	default:
		return displayCell(fmt.Sprint(val))
	}
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
{{end}}` + cellFocusJS + pageFooter + `</body>
</html>
`))

// renderCellView renders the full text of one table cell: header and cells
// are the column names and the complete (untruncated) row, row and col the
// cell's 1-based display coordinates, and backHref the table view to return
// to. Only plain-text cells are served.
func (s fileServer) renderCellView(w http.ResponseWriter, r *http.Request, info fs.FileInfo, crumbs []crumb, title string, header, cells []string, row, col int, backHref string) {
	if col > len(cells) {
		http.NotFound(w, r)
		return
	}
	text := cells[col-1]
	if !isPlainText([]byte(text)) {
		http.NotFound(w, r)
		return
	}

	where := fmt.Sprintf("row %d, column %d", row, col)
	if col <= len(header) && header[col-1] != "" {
		where += fmt.Sprintf(" (%s)", header[col-1])
	}
	chars := len([]rune(text))
	page := cellPage{
		Title:  title,
		Crumbs: crumbs,
		Where:  where,
		Detail: fmt.Sprintf("%d character%s", chars, plural(chars)),
		// The fragment carries the coordinates back so the table view can
		// scroll the cell into view instead of resetting to the top left.
		BackHref: fmt.Sprintf("%s#cell=%d,%d", backHref, row, col),
		Text:     text,
		Link:     cellURL(text),
	}

	var buf bytes.Buffer
	if err := cellTemplate.Execute(&buf, page); err != nil {
		log.Printf("render cell %s: %v", title, err)
		http.Error(w, "cannot render cell", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeContent(w, r, "cell.html", info.ModTime(), bytes.NewReader(buf.Bytes()))
}

type cellPage struct {
	Title    string // the table's name in the nav, linking back to it
	Crumbs   []crumb
	Where    string // cell coordinates, e.g. `row 3, column 2 (email)`
	Detail   string
	BackHref string
	Text     string
	Link     string // external URL the text names, when it names one
}

var cellTemplate = template.Must(template.New("cell").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<link rel="icon" href="/` + assetPrefix + `/favicon.png">
<title>{{.Where}} — {{.Title}}</title>
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
  p.summary { color: #57606a; font-size: 0.85rem; }
  pre.cell {
    border: 1px solid #d0d7de;
    background-color: #fff;
    padding: 0.75rem 1rem;
    font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
    font-size: 0.85rem;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
</style>
` + dropJS + `</head>
<body>
<nav>{{range $i, $c := .Crumbs}}{{if gt $i 1}}<span class="sep">/</span>{{end}}<a href="{{$c.Href}}">{{$c.Name}}</a>{{end}}{{if gt (len .Crumbs) 1}}<span class="sep">/</span>{{end}}<a href="{{.BackHref}}">{{.Title}}</a><span class="sep">/</span><span class="file">{{.Where}}</span></nav>
<p class="summary">{{.Detail}} — <a href="{{.BackHref}}">&larr; back to table</a></p>
<pre class="cell">{{if .Link}}<a href="{{.Link}}" target="_blank" rel="noopener">{{.Text}}</a>{{else}}{{.Text}}{{end}}</pre>
` + pageFooter + `</body>
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

	var worktree *worktreeStatus
	if s.dir != "" {
		if worktree = s.worktreeStatusFor(r.Context(), name); worktree != nil {
			page.GitLine = worktree.rootLine()
		}
	}

	var dirs, files int
	var total int64
	for _, item := range items {
		entry := item.entry
		view := dirEntryView{Name: entry.Name(), Href: (&url.URL{Path: entry.Name()}).String()}
		if worktree != nil {
			view.Git = worktree.entryStatus(entry.Name(), entry.IsDir())
		}
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
			continue
		}
		page.Entries = append(page.Entries, view)
		if entry.IsDir() {
			dirs++
		} else {
			files++
			if item.info != nil {
				total += item.info.Size()
			}
		}
	}
	page.Summary = listingSummary(dirs, files, total)

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

	if s.dir != "" && isGitDirListing(entries) {
		page.Git = gitSection(r.Context(), filepath.Join(s.dir, filepath.FromSlash(name)))
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

// isGitDirListing reports whether a directory listing looks like a git
// repository directory — a .git directory or a bare repository — by the
// presence of a HEAD file plus objects and refs directories.
func isGitDirListing(entries []fs.DirEntry) bool {
	var head, objects, refs bool
	for _, e := range entries {
		switch e.Name() {
		case "HEAD":
			head = !e.IsDir()
		case "objects":
			objects = e.IsDir()
		case "refs":
			refs = e.IsDir()
		}
	}
	return head && objects && refs
}

type gitCommitView struct {
	Hash    string
	Date    string
	Author  string
	Subject string
}

type gitView struct {
	Head    string   // "branch main", "detached HEAD at abc1234", ...
	Facts   []string // "12 commits", "3 branches", "2 tags", "4.5 MB of objects"
	Remotes []string // "origin: ssh://..."
	Commits []gitCommitView
}

// gitOutput runs one git command against a repository directory and returns
// its trimmed stdout.
func gitOutput(ctx context.Context, gitDir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"--git-dir", gitDir}, args...)...)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}

func countNoun(n int64, singular, plural string) string {
	if n == 1 {
		return "1 " + singular
	}
	return fmt.Sprintf("%d %s", n, plural)
}

// gitSection summarizes a git repository directory for the bottom of its
// listing page: where HEAD points, repository-level counts, remotes, and
// the most recent commits. Each probe degrades independently, so a bare,
// empty, or odd repository still shows whatever applies.
func gitSection(ctx context.Context, gitDir string) *gitView {
	if _, err := exec.LookPath("git"); err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	view := &gitView{}

	head, err := gitOutput(ctx, gitDir, "rev-parse", "--abbrev-ref", "HEAD")
	switch {
	case err != nil:
		view.Head = "unborn HEAD"
	case head == "HEAD":
		view.Head = "detached HEAD"
		if short, err := gitOutput(ctx, gitDir, "rev-parse", "--short", "HEAD"); err == nil {
			view.Head += " at " + short
		}
	default:
		view.Head = "branch " + head
	}

	if out, err := gitOutput(ctx, gitDir, "rev-list", "--count", "HEAD"); err == nil {
		if n, err := strconv.ParseInt(out, 10, 64); err == nil {
			view.Facts = append(view.Facts, countNoun(n, "commit", "commits"))
		}
	} else {
		view.Facts = append(view.Facts, "no commits yet")
	}
	countRefs := func(space, singular, plural string) {
		out, err := gitOutput(ctx, gitDir, "for-each-ref", "--format=%(refname)", space)
		if err != nil {
			return
		}
		var n int64
		if out != "" {
			n = int64(strings.Count(out, "\n") + 1)
		}
		view.Facts = append(view.Facts, countNoun(n, singular, plural))
	}
	countRefs("refs/heads", "branch", "branches")
	countRefs("refs/tags", "tag", "tags")
	if out, err := gitOutput(ctx, gitDir, "count-objects", "-v"); err == nil {
		var kib int64
		for _, line := range strings.Split(out, "\n") {
			key, val, ok := strings.Cut(line, ": ")
			if ok && (key == "size" || key == "size-pack") {
				if n, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64); err == nil {
					kib += n
				}
			}
		}
		view.Facts = append(view.Facts, humanSize(kib*1024)+" of objects")
	}

	if out, err := gitOutput(ctx, gitDir, "config", "--get-regexp", `^remote\..*\.url$`); err == nil {
		for _, line := range strings.Split(out, "\n") {
			key, u, ok := strings.Cut(line, " ")
			if !ok {
				continue
			}
			name := strings.TrimSuffix(strings.TrimPrefix(key, "remote."), ".url")
			view.Remotes = append(view.Remotes, name+": "+u)
		}
	}

	const logFormat = "%h%x09%ad%x09%an%x09%s"
	if out, err := gitOutput(ctx, gitDir, "log", "-n", "8", "--format="+logFormat, "--date=format:%Y-%m-%d %H:%M"); err == nil {
		for _, line := range strings.Split(out, "\n") {
			parts := strings.SplitN(line, "\t", 4)
			if len(parts) == 4 {
				view.Commits = append(view.Commits, gitCommitView{Hash: parts[0], Date: parts[1], Author: parts[2], Subject: parts[3]})
			}
		}
	}

	return view
}

// gitFileStatus is one listing entry's relationship to its git worktree,
// ready for the template: a short colored badge after the name with a
// hover description, or a dimmed name for ignored files. The zero value
// renders nothing (clean tracked files, non-repo listings).
type gitFileStatus struct {
	Badge string // "M", "?", "●", ... ; "" for none
	Class string // CSS color class: conflict, modified, staged, untracked
	Title string // hover description of the state
	Dim   bool   // gitignored: fade the name instead of badging it
}

// gitStatusItem is one fragment of the repo-root "git:" line; Bad marks
// the states worth noticing (dirty counts, ahead/behind).
type gitStatusItem struct {
	Text string
	Bad  bool
}

// gitDirCounts aggregates the states found beneath one subdirectory of a
// listing (and, summed, beneath the listing itself).
type gitDirCounts struct {
	conflicts, staged, modified, deleted, untracked int
}

func (c *gitDirCounts) any() bool {
	return c.conflicts+c.staged+c.modified+c.deleted+c.untracked > 0
}

// facts renders the nonzero counts as "2 modified, 1 untracked" fragments.
func (c *gitDirCounts) facts() []string {
	var out []string
	add := func(n int, noun string) {
		if n > 0 {
			out = append(out, fmt.Sprintf("%d %s", n, noun))
		}
	}
	add(c.conflicts, "unmerged")
	add(c.modified, "modified")
	add(c.deleted, "deleted")
	add(c.staged, "staged")
	add(c.untracked, "untracked")
	return out
}

// badge summarizes a subdirectory's contents as a small dot whose color
// follows the most urgent state within, described fully on hover.
func (c *gitDirCounts) badge() gitFileStatus {
	class := "untracked"
	switch {
	case c.conflicts > 0:
		class = "conflict"
	case c.modified+c.deleted+c.staged > 0:
		class = "modified"
	}
	return gitFileStatus{Badge: "●", Class: class, Title: strings.Join(c.facts(), ", ") + " within"}
}

// worktreeStatus is the parsed `git status` of one listing directory:
// per-entry states for its direct children, aggregated counts for its
// subdirectories, branch/upstream headers, and totals for the repo-root
// summary line.
type worktreeStatus struct {
	branch   string // branch.head: "main", "(detached)", ...
	oid      string // branch.oid, abbreviated
	upstream string // branch.upstream: "origin/main", "" when unset
	hasAB    bool   // branch.ab header present
	ahead    int
	behind   int
	files    map[string]gitFileStatus // direct children by entry name
	dirs     map[string]*gitDirCounts // subdirs with changes deeper down
	counts   gitDirCounts             // totals across the whole listing subtree
	all      gitFileStatus            // listing dir itself untracked/ignored: applies to every entry
	isRoot   bool                     // listing dir is the worktree root
}

// findWorktreeRoot walks up from an OS directory looking for a .git entry
// (directory, or file for linked worktrees and submodules) and returns the
// directory holding it. Paths inside a .git directory itself return "" —
// object and ref listings are not worktree files.
func findWorktreeRoot(dir string) string {
	for cur := dir; ; {
		if filepath.Base(cur) == ".git" {
			return ""
		}
		if fi, err := os.Stat(filepath.Join(cur, ".git")); err == nil && (fi.IsDir() || fi.Mode().IsRegular()) {
			return cur
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return ""
		}
		cur = parent
	}
}

// worktreeStatusFor runs git status scoped to one listing directory and
// parses it, or returns nil when the directory is not in a worktree (or
// git is missing, slow, or fails — annotations just don't appear).
// --no-optional-locks keeps browsing from ever writing to the repository.
func (s fileServer) worktreeStatusFor(ctx context.Context, name string) *worktreeStatus {
	osDir := filepath.Join(s.dir, filepath.FromSlash(name))
	root := findWorktreeRoot(osDir)
	if root == "" {
		return nil
	}
	if _, err := exec.LookPath("git"); err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", osDir,
		"--no-optional-locks", "status", "--porcelain=v2", "-z", "--branch", "--ignored=matching", "--", ".")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		log.Printf("git status in %s: %v", osDir, err)
		return nil
	}
	relDir := ""
	if rel, err := filepath.Rel(root, osDir); err == nil && rel != "." {
		relDir = filepath.ToSlash(rel)
	}
	ws := parseWorktreeStatus(out.String(), relDir)
	ws.isRoot = relDir == ""
	return ws
}

// parseWorktreeStatus decodes NUL-separated `git status --porcelain=v2`
// output, whose paths -z pins relative to the worktree root; relDir (the
// listing directory relative to that root, "" at the root itself) is
// stripped so entries key by listing name. Anything deeper than one level
// is aggregated into its top-level subdirectory's counts.
func parseWorktreeStatus(out, relDir string) *worktreeStatus {
	ws := &worktreeStatus{
		files: make(map[string]gitFileStatus),
		dirs:  make(map[string]*gitDirCounts),
	}
	tokens := strings.Split(out, "\x00")
	for i := 0; i < len(tokens); i++ {
		line := tokens[i]
		if line == "" {
			continue
		}
		if rest, ok := strings.CutPrefix(line, "# "); ok {
			key, val, _ := strings.Cut(rest, " ")
			switch key {
			case "branch.head":
				ws.branch = val
			case "branch.oid":
				if len(val) > 7 {
					val = val[:7]
				}
				ws.oid = val
			case "branch.upstream":
				ws.upstream = val
			case "branch.ab":
				if _, err := fmt.Sscanf(val, "+%d -%d", &ws.ahead, &ws.behind); err == nil {
					ws.hasAB = true
				}
			}
			continue
		}
		var xy, path string
		switch line[0] {
		case '1':
			fields := strings.SplitN(line, " ", 9)
			if len(fields) < 9 {
				continue
			}
			xy, path = fields[1], fields[8]
		case '2':
			fields := strings.SplitN(line, " ", 10)
			if len(fields) < 10 {
				continue
			}
			xy, path = fields[1], fields[9]
			i++ // the following token is the rename's original path
		case 'u':
			fields := strings.SplitN(line, " ", 11)
			if len(fields) < 11 {
				continue
			}
			xy, path = fields[1], fields[10]
		case '?', '!':
			xy, path = string(line[0]), line[2:]
		default:
			continue
		}
		if relDir != "" {
			if path == relDir+"/" {
				// The listing directory itself is untracked or ignored:
				// the state applies to everything in it.
				ws.all = fileStatusFor(xy)
				continue
			}
			rest, ok := strings.CutPrefix(path, relDir+"/")
			if !ok {
				continue
			}
			path = rest
		}
		ws.record(xy, path)
	}
	return ws
}

// record files one status entry under the direct child it belongs to,
// keeping both the per-entry state and the listing-wide totals.
func (ws *worktreeStatus) record(xy, path string) {
	name, isDir := strings.CutSuffix(path, "/")
	if child, _, nested := strings.Cut(name, "/"); nested {
		// Deeper than the listing: fold into the subdirectory's counts.
		// Ignored files inside tracked subdirectories are noise, not news.
		if xy != "!" {
			c := ws.dirs[child]
			if c == nil {
				c = &gitDirCounts{}
				ws.dirs[child] = c
			}
			c.count(xy)
			ws.counts.count(xy)
		}
		return
	}
	_ = isDir // untracked/ignored directories annotate like files
	ws.files[name] = fileStatusFor(xy)
	if xy != "!" {
		ws.counts.count(xy)
	}
}

func (c *gitDirCounts) count(xy string) {
	switch {
	case xy == "?":
		c.untracked++
	case len(xy) != 2: // conflict codes like "UU", "AA" have both set
	case xy[0] == 'U' || xy[1] == 'U' || xy == "AA" || xy == "DD":
		c.conflicts++
	case xy[1] == 'D':
		c.deleted++
	case xy[1] != '.':
		c.modified++
	default:
		c.staged++
	}
}

// stagedNouns names the staged states by their index letter for badge
// tooltips: "A" reads better as "new file" than as bare "added".
var stagedNouns = map[byte]string{
	'M': "modified", 'A': "new file", 'D': "deleted", 'R': "renamed", 'C': "copied", 'T': "type changed",
}

// fileStatusFor maps one porcelain XY code (or "?"/"!") to its badge.
func fileStatusFor(xy string) gitFileStatus {
	switch xy {
	case "?":
		return gitFileStatus{Badge: "?", Class: "untracked", Title: "untracked — not added to git"}
	case "!":
		return gitFileStatus{Dim: true, Title: "gitignored"}
	}
	if len(xy) != 2 {
		return gitFileStatus{}
	}
	x, y := xy[0], xy[1]
	if x == 'U' || y == 'U' || xy == "AA" || xy == "DD" {
		return gitFileStatus{Badge: "!", Class: "conflict", Title: "merge conflict — unmerged"}
	}
	switch {
	case x != '.' && y != '.':
		return gitFileStatus{Badge: string(x) + string(y), Class: "modified", Title: "staged, with further unstaged changes"}
	case y != '.':
		noun := stagedNouns[y]
		if noun == "" {
			noun = "changed"
		}
		return gitFileStatus{Badge: string(y), Class: "modified", Title: noun + " — not staged"}
	case x != '.':
		noun := stagedNouns[x]
		if noun == "" {
			noun = "changed"
		}
		return gitFileStatus{Badge: string(x), Class: "staged", Title: noun + " — staged for commit"}
	}
	return gitFileStatus{}
}

// entryStatus resolves the annotation for one listing entry: a whole-dir
// state if the listing itself is untracked/ignored, the entry's own state,
// or (for subdirectories) the summary of changes within.
func (ws *worktreeStatus) entryStatus(name string, isDir bool) gitFileStatus {
	if ws.all.Badge != "" || ws.all.Dim {
		return ws.all
	}
	if st, ok := ws.files[name]; ok {
		return st
	}
	if isDir {
		if c := ws.dirs[name]; c != nil && c.any() {
			return c.badge()
		}
	}
	return gitFileStatus{}
}

// rootLine builds the repo-root "git:" summary — branch, sync against the
// upstream as of the last fetch, and dirty counts — shown only on the
// worktree's top-level listing.
func (ws *worktreeStatus) rootLine() []gitStatusItem {
	if !ws.isRoot {
		return nil
	}
	var items []gitStatusItem
	switch {
	case ws.branch == "(detached)" && ws.oid != "":
		items = append(items, gitStatusItem{Text: "detached HEAD at " + ws.oid})
	case ws.branch != "":
		items = append(items, gitStatusItem{Text: "branch " + ws.branch})
	}
	switch {
	case ws.hasAB && ws.ahead == 0 && ws.behind == 0:
		items = append(items, gitStatusItem{Text: "in sync with " + ws.upstream})
	case ws.hasAB:
		var sync []string
		if ws.ahead > 0 {
			sync = append(sync, fmt.Sprintf("ahead %d", ws.ahead))
		}
		if ws.behind > 0 {
			sync = append(sync, fmt.Sprintf("behind %d", ws.behind))
		}
		items = append(items, gitStatusItem{Text: strings.Join(sync, ", ") + " of " + ws.upstream, Bad: true})
	default:
		items = append(items, gitStatusItem{Text: "no upstream"})
	}
	if ws.counts.any() {
		for _, f := range ws.counts.facts() {
			items = append(items, gitStatusItem{Text: f, Bad: true})
		}
	} else {
		items = append(items, gitStatusItem{Text: "clean"})
	}
	return items
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

// listingSummary formats the headline atop directory and archive listings:
// "5 dirs · 23 files · 1.4 GB". The byte total covers the listed files
// only (dot-files and subdirectory contents excluded).
func listingSummary(dirs, files int, size int64) string {
	summary := fmt.Sprintf("%d dir%s · %d file%s", dirs, plural(dirs), files, plural(files))
	if size > 0 {
		summary += " · " + humanSize(size)
	}
	return summary
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
	Summary    string
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
	Git        *gitView
	GitLine    []gitStatusItem
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
	Git     gitFileStatus
}

// dropJS lets any rendered page accept a drag-and-dropped file: the file is
// uploaded to the drop endpoint (with a floating progress monitor, since
// large files take a while) and the browser navigates to the stored copy,
// which renders through the ordinary pipeline like any other file.
// pageFooter links the health and version probes from the bottom of every
// rendered page except markdown documents, which stay clean. Inline styles
// keep it self-contained, so templates and pandoc output can append it
// without touching their own CSS.
const pageFooter = `<footer style="margin-top:3rem;padding-top:0.5rem;border-top:1px solid #eaeef0;font-size:0.75rem;color:#8c959f"><a style="color:#8c959f" href="/` + assetPrefix + `/healthz">healthz</a> &middot; <a style="color:#8c959f" href="/` + assetPrefix + `/version">version</a></footer>
`

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
{{end}}{{if or .Header .Rows}}<div class="tablewrap">
<table class="data">
<tr class="coords"><td class="rownum"></td>{{range .ColNums}}<td class="colnum">{{.}}</td>{{end}}</tr>
{{if .Header}}<tr><th class="corner"></th>{{range .Header}}<th>{{.}}</th>{{end}}</tr>
{{end}}{{range .Rows}}<tr><td class="rownum">{{.N}}</td>{{range .Cells}}<td>{{if .Link}}<a href="{{.Link}}" target="_blank" rel="noopener">{{.Text}}</a>{{else}}{{.Text}}{{end}}{{if .More}}<a class="more" href="{{.More}}">&hellip;&nbsp;more</a>{{end}}</td>{{end}}</tr>
{{end}}</table>
</div>
{{else}}<p class="summary">(empty)</p>
{{end}}{{if .Omitted}}<p class="summary">&hellip; and {{.Omitted}} more rows</p>
{{end}}{{end}}
`

// cellFocusJS scrolls the table cell named by a #cell=row,col fragment into
// view and flashes it, so "back to table" from a full-cell page returns to
// the spot the reader left rather than the top-left corner. Shared by the
// directory and table templates; runs at the end of body.
const cellFocusJS = `<script>
(function () {
  var m = /^#cell=(\d+),(\d+)$/.exec(location.hash);
  if (!m) return;
  var table = document.querySelector("table.data");
  if (!table) return;
  var tr = table.rows[+m[1] + 1]; // rows 0 and 1 are the coords and header
  var td = tr && tr.cells[+m[2]]; // cell 0 is the row number
  if (!td) return;
  td.scrollIntoView({ block: "center", inline: "center" });
  td.style.outline = "2px solid #0969da";
  td.style.outlineOffset = "-2px";
  setTimeout(function () { td.style.outline = ""; }, 2000);
})();
</script>
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
  table.data a.more { white-space: nowrap; }
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
  /* Git worktree annotations: small colored badges after names, hover
     titles carry the words; ignored files fade instead of badging. */
  span.gitb {
    margin-left: 0.5em;
    font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
    font-size: 0.7rem;
    font-weight: 600;
    cursor: default;
  }
  span.gitb.modified { color: #9a6700; }
  span.gitb.staged { color: #1a7f37; }
  span.gitb.untracked { color: #6e7781; }
  span.gitb.conflict { color: #cf222e; }
  a.gitdim { opacity: 0.55; }
  p.gitline { margin: -0.3rem 0 0.6rem; font-size: 0.85rem; color: #57606a; }
  p.gitline span.bad { color: #9a6700; font-weight: 600; }
  section.readme { margin-top: 2rem; border-top: 1px solid #d0d7de; }
  section.readme h1.readme-title { font-size: 1rem; color: #57606a; font-weight: 600; }
  section.readme pre.readme-text {
    font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
    font-size: 0.85rem;
    white-space: pre-wrap;
  }
  section.git { margin-top: 2rem; border-top: 1px solid #d0d7de; }
  section.git h1.git-title { font-size: 1rem; color: #57606a; font-weight: 600; }
  section.git p { margin: 0.3rem 0; font-size: 0.85rem; color: #57606a; }
  table.commits { border-collapse: collapse; font-size: 0.85rem; margin-top: 0.75rem; }
  table.commits td { padding: 0.25rem 1rem 0.25rem 0; vertical-align: baseline; }
  table.commits td.hash {
    font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
    color: #57606a;
    white-space: nowrap;
  }
  table.commits td.cmeta {
    color: #57606a;
    white-space: nowrap;
    font-variant-numeric: tabular-nums;
  }
` + dataTableCSS + `</style>
` + dropJS + `</head>
<body>
<nav>{{range $i, $c := .Crumbs}}{{if gt $i 1}}<span class="sep">/</span>{{end}}<a href="{{$c.Href}}">{{$c.Name}}</a>{{end}}{{if .RawHref}}<a class="raw" href="{{.RawHref}}">raw</a>{{end}}</nav>
{{if .Blurb}}<p class="blurb">{{.Blurb}}</p>
{{end}}{{if .Summary}}<p class="summary" id="listsum">{{.Summary}}</p>
{{end}}{{with .GitLine}}<p class="gitline">git: {{range $i, $it := .}}{{if $i}}<span class="sep"> &middot; </span>{{end}}<span{{if $it.Bad}} class="bad"{{end}}>{{$it.Text}}</span>{{end}}</p>
{{end}}{{if .QueryForm}}<form class="query" method="get" action="">
<input type="text" name="q" value="{{.Query}}" placeholder="SQL query, e.g. select * from some_table limit 10" spellcheck="false" autocomplete="off">
</form>
{{with .QueryError}}<p class="queryerror">{{.}}</p>
{{end}}{{with .QuerySheet}}{{template "datatable" .}}
{{end}}{{end}}{{if .StatsAsync}}<p class="summary" id="statprog"></p>
{{end}}{{if .SortLinks}}<div class="sort">sort: {{range $i, $l := .SortLinks}}{{if $i}} &middot; {{end}}<a {{if $l.Active}}class="active" {{end}}href="{{$l.Href}}">{{$l.Label}}{{if $l.Active}} {{$l.Arrow}}{{end}}</a>{{end}}</div>
{{end}}
{{define "gitb"}}{{with .Git}}{{if .Badge}}<span class="gitb {{.Class}}" title="{{.Title}}">{{.Badge}}</span>{{end}}{{end}}{{end}}{{define "rows"}}{{range .}}<tr data-name="{{.Name}}">{{if .IsDir}}<td class="dname"><a {{if .Git.Dim}}class="gitdim" {{end}}href="{{.Href}}" title="{{.Name}}{{if .Git.Dim}} — {{.Git.Title}}{{end}}">{{.Name}}</a>{{template "gitb" .}}</td><td class="blurb"{{with .Blurb}} title="{{.}}"{{end}}>{{.Blurb}}</td>{{else}}<td class="fname" colspan="2"><a {{if .Git.Dim}}class="gitdim" {{end}}href="{{.Href}}" title="{{.Name}}{{if .Git.Dim}} — {{.Git.Title}}{{end}}">{{.Name}}</a>{{template "gitb" .}}</td>{{end}}<td class="meta">{{.Size}}</td><td class="meta">{{.ModTime}}</td></tr>
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
{{with .Git}}<section class="git">
<h1 class="git-title">git repository</h1>
<p>{{.Head}}{{range .Facts}} &middot; {{.}}{{end}}</p>
{{range .Remotes}}<p>{{.}}</p>
{{end}}{{if .Commits}}<table class="commits">
{{range .Commits}}<tr><td class="hash">{{.Hash}}</td><td class="cmeta">{{.Date}}</td><td class="cmeta">{{.Author}}</td><td>{{.Subject}}</td></tr>
{{end}}</table>{{end}}
</section>{{end}}
{{if .StatsAsync}}` + statsJS + `{{end}}` + cellFocusJS + pageFooter + `</body>
</html>
`))

// statsJS fills in per-table stats on sqlite listing pages one table at a
// time, with a progress indicator, so listings of huge databases render
// instantly instead of waiting on every COUNT(*). As it goes it totals rows
// and bytes, and appends them to the summary line when the sweep finishes.
const statsJS = `<script>
(function () {
  var prog = document.getElementById("statprog");
  var sum = document.getElementById("listsum");
  var rows = Array.prototype.slice.call(document.querySelectorAll("table.listing tr[data-name]"))
    .filter(function (tr) { return tr.getAttribute("data-name") !== ".."; });
  if (!rows.length) { if (prog) prog.remove(); return; }
  var totalRows = 0, totalBytes = 0;
  function human(n) {
    var units = ["B", "KB", "MB", "GB", "TB"], i = 0;
    while (n >= 1024 && i < units.length - 1) { n /= 1024; i++; }
    return (i ? n.toFixed(1) : n) + " " + units[i];
  }
  function finish() {
    prog.remove();
    if (!sum) return;
    sum.textContent += " · " + totalRows + (totalRows === 1 ? " row" : " rows");
    if (totalBytes > 0) sum.textContent += " · " + human(totalBytes);
  }
  function next(i) {
    if (i >= rows.length) { finish(); return; }
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
        totalRows += st.rows || 0;
        totalBytes += st.size || 0;
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
