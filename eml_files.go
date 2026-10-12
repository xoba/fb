package main

import (
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"strings"
	"testing/fstest"
)

// Prefer predictable filenames for unnamed MIME parts, so they reach the same
// viewers as ordinary files. Supplied filenames still take precedence.
var emlMediaExtensions = map[string]string{
	"text/plain": ".txt", "text/html": ".html", "text/markdown": ".md",
	"text/x-markdown": ".md", "application/x-ipynb+json": ".ipynb",
	"application/x-plist": ".plist", "application/x-bplist": ".plist",
	"text/x-rst": ".rst", "text/csv": ".csv", "text/tab-separated-values": ".tsv",
	"text/calendar": ".ics", "text/css": ".css", "text/javascript": ".js",
	"text/xml": ".xml", "application/xml": ".xml", "application/xhtml+xml": ".html",
	"application/json": ".json", "application/javascript": ".js",
	"application/pdf": ".pdf", "application/rtf": ".rtf", "text/rtf": ".rtf",
	"application/msword": ".doc", "application/epub+zip": ".epub",
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": ".docx",
	"application/vnd.oasis.opendocument.text":                                 ".odt",
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":       ".xlsx",
	"application/vnd.apache.parquet":                                          ".parquet", "application/x-parquet": ".parquet",
	"application/vnd.sqlite3": ".sqlite3", "application/x-sqlite3": ".sqlite3",
	"application/zip": ".zip", "application/x-zip-compressed": ".zip",
	"application/java-archive": ".jar", "application/x-tar": ".tar",
	"application/gzip": ".gz", "application/x-gzip": ".gz",
	"message/rfc822": ".eml", "message/global": ".eml",
	"image/png": ".png", "image/jpeg": ".jpg", "image/gif": ".gif",
	"image/webp": ".webp", "image/bmp": ".bmp", "image/tiff": ".tiff",
	"image/heic": ".heic", "image/heif": ".heif", "image/svg+xml": ".svg",
	"video/mp4": ".mp4", "video/quicktime": ".mov", "video/x-m4v": ".m4v",
	"video/webm": ".webm", "audio/mpeg": ".mp3", "audio/mp4": ".m4a",
	"audio/aac": ".aac", "audio/flac": ".flac", "audio/x-flac": ".flac",
	"audio/ogg": ".ogg", "application/ogg": ".ogg", "audio/wav": ".wav",
	"audio/x-wav": ".wav", "audio/wave": ".wav",
}

func emlHasFile(node *emlNode) bool {
	return len(node.Children) == 0 && !strings.HasPrefix(node.MediaType, "multipart/") || emlAttachedMessage(node)
}

func emlUsesFileViewer(node *emlNode) bool {
	if !emlHasFile(node) {
		return false
	}
	// Body alternatives retain the readable, charset-aware email presentation.
	// Named attachments (including text and HTML) use ordinary file viewers.
	return node.Filename != "" || node.MediaType != "text/plain" && node.MediaType != "text/html"
}

func (s fileServer) emlFileURL(name string, node *emlNode) string {
	return (&url.URL{Path: path.Join("/", s.base, name, node.ID, emlFilename(node))}).String()
}

func emlTextView(name string) bool {
	ext := strings.ToLower(path.Ext(viewName(name)))
	return tableDelims[ext] != 0 || highlightable(name) || isGoModFile(name) || isICalendar(name) ||
		ext == ".txt" || ext == ".html" || ext == ".svg" || ext == ".md" || ext == ".rst" || ext == ".typ"
}

func (s fileServer) readEML(name string, info fs.FileInfo) (*emlMessage, []byte, error) {
	if info.Size() > maxEMLBytes {
		return nil, nil, fmt.Errorf("message exceeds the %d MiB browsing limit; open the original source to save a copy", maxEMLBytes>>20)
	}
	file, err := s.fsys.Open(name)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxEMLBytes+1))
	if err != nil {
		return nil, data, err
	}
	message, err := parseEML(data)
	return message, data, err
}

// Mount decoded MIME data exactly as archive members are mounted. Part IDs
// isolate duplicate or hostile filenames; no attachment is written to disk.
// The shared viewers may use their usual bounded conversion scratch space.
func (s fileServer) descendEML(w http.ResponseWriter, r *http.Request, name string, info fs.FileInfo, inner string, depth int) {
	if s.emlDepth >= maxEMLDepth {
		http.NotFound(w, r)
		return
	}
	release, ok := acquireHeavy(r.Context())
	if !ok {
		http.Error(w, "server busy", http.StatusServiceUnavailable)
		return
	}
	message, _, err := s.readEML(name, info)
	release() // A document, table, or nested archive can need its own slot.
	if err != nil {
		http.Error(w, "cannot decode email: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}
	files := fstest.MapFS{}
	preview := r.URL.Query().Get("raw") != "1" && wantsDocument(r.Header)
	var add func(*emlNode)
	add = func(node *emlNode) {
		if emlHasFile(node) {
			filename := emlFilename(node)
			data := node.Data
			// MIME supplies a charset that a stand-alone text file does not.
			// Feed its decoded text to textual viewers, retaining exact original
			// bytes for raw requests, downloads, media and binary containers.
			if preview && strings.HasPrefix(node.MediaType, "text/") && emlTextView(filename) {
				data = []byte(node.Text)
			}
			files[path.Join(node.ID, filename)] = &fstest.MapFile{
				Data: data, Mode: 0o444, ModTime: info.ModTime(),
			}
		}
		for _, child := range node.Children {
			add(child)
		}
	}
	add(message.Root)
	sub := fileServer{fsys: files, pandoc: s.pandoc, base: path.Join("/", s.base, name), emlDepth: s.emlDepth + 1}
	sub.route(w, r, inner, depth)
}
