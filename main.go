package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const listenAddr = "localhost:3030"

func main() {
	root, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	if _, err := exec.LookPath("pandoc"); err != nil {
		log.Fatal("pandoc is required but was not found in PATH")
	}

	fsys := os.DirFS(root)
	srv := &http.Server{
		Addr:    listenAddr,
		Handler: fileServer{fsys: fsys, root: root, pandoc: "pandoc"},
	}

	log.Printf("serving %s at http://%s/", root, listenAddr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

type fileServer struct {
	fsys   fs.FS
	root   string
	pandoc string
}

func (s fileServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := requestName(r.URL.Path)

	if strings.EqualFold(path.Ext(name), ".md") {
		info, err := fs.Stat(s.fsys, name)
		if err == nil && !info.IsDir() {
			s.serveMarkdown(w, r, name, info)
			return
		}
	}

	http.ServeFileFS(w, r, s.fsys, name)
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

	html, err := s.renderMarkdown(r.Context(), name)
	if err != nil {
		log.Printf("pandoc failed for %s: %v", name, err)
		http.Error(w, "pandoc failed", http.StatusInternalServerError)
		return
	}

	outName := strings.TrimSuffix(path.Base(name), path.Ext(name)) + ".html"
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeContent(w, r, outName, info.ModTime(), bytes.NewReader(html))
}

func (s fileServer) renderMarkdown(ctx context.Context, name string) ([]byte, error) {
	header, cleanup, err := writePandocHeader()
	if err != nil {
		return nil, err
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	args := []string{
		"-f", "markdown+footnotes+lists_without_preceding_blankline+hard_line_breaks+tex_math_single_backslash",
		"--mathjax=https://cdn.jsdelivr.net/npm/mathjax@3/es5/tex-mml-chtml.js",
		"--reference-location=section",
		"--include-in-header", header,
		localPath(s.root, name),
		"-s",
	}

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

func localPath(root, name string) string {
	if name == "." {
		return root
	}
	return filepath.Join(root, filepath.FromSlash(name))
}

func writePandocHeader() (string, func(), error) {
	f, err := os.CreateTemp("", "localmd-pandoc-header-*.html")
	if err != nil {
		return "", func() {}, err
	}

	cleanup := func() {
		if err := os.Remove(f.Name()); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Printf("remove temp pandoc header %s: %v", f.Name(), err)
		}
	}

	if _, err := f.WriteString(pandocHeader); err != nil {
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

const pandocHeader = `<style>
  :root {
    color-scheme: light;
  }

  html, body {
    color: #000 !important;
    background: #fff !important;
  }

  body * {
    color: #000 !important;
    opacity: 1 !important;
    text-shadow: none !important;
  }

  a,
  a:visited,
  a:hover,
  a:active {
    color: #000 !important;
  }

  blockquote {
    color: #000 !important;
    border-left: 0.2rem solid #222 !important;
    margin-left: 0;
    padding-left: 1rem;
  }

  blockquote * {
    color: #000 !important;
    opacity: 1 !important;
    -webkit-text-fill-color: #000 !important;
  }

  img[alt="Book Cover"] {
    display: block;
    margin-top: 2.25rem !important;
  }

  .footnotes,
  .footnotes *,
  .footnote-ref,
  .footnote-ref * {
    color: #000 !important;
    opacity: 1 !important;
    -webkit-text-fill-color: #000 !important;
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
    }
  }
</style>
`
