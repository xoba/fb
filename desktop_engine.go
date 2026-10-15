//go:build fbmac

package main

// This adapter keeps the existing HTTP-shaped rendering API without starting
// an HTTP server. WebKit receives the response through a C ABI and an io.Pipe;
// large files and byte ranges are streamed rather than buffered in memory.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

type desktopRequestInput struct {
	URL        string            `json:"url"`
	Method     string            `json:"method"`
	Headers    map[string]string `json:"headers"`
	Document   bool              `json:"document"`
	BodyBase64 string            `json:"bodyBase64,omitempty"`
}

type desktopResponse struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Error   string            `json:"error,omitempty"`
}

type desktopEngine struct {
	root    *os.Root
	handler http.Handler
	mu      sync.Mutex
	closed  bool
	active  map[*desktopRequest]struct{}
}

type desktopRequest struct {
	ctx           context.Context
	cancelContext context.CancelFunc
	body          *io.PipeReader
	writer        *io.PipeWriter
	ready         chan struct{}
	done          chan struct{}
	responseOnce  sync.Once
	response      desktopResponse
}

var desktopHandles = struct {
	sync.Mutex
	next     atomic.Uint64
	engines  map[uint64]*desktopEngine
	requests map[uint64]*desktopRequest
}{engines: make(map[uint64]*desktopEngine), requests: make(map[uint64]*desktopRequest)}

func newDesktopEngine(root, pandoc string) (*desktopEngine, error) {
	resolved, err := resolveRoot(root)
	if err != nil {
		return nil, err
	}
	// Require the bundled executable explicitly. An app never depends on PATH,
	// Homebrew, or a converter that happens to be installed on the user's Mac.
	if !filepath.IsAbs(pandoc) {
		return nil, errors.New("pandoc must be an absolute executable path")
	}
	info, err := os.Stat(pandoc)
	if err != nil {
		return nil, fmt.Errorf("bundled pandoc: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		return nil, errors.New("bundled pandoc is not executable")
	}
	osRoot, err := os.OpenRoot(resolved)
	if err != nil {
		return nil, err
	}
	fsys := osRoot.FS()
	if resolved == string(os.PathSeparator) {
		// The native app browses the whole filesystem with the user's permissions.
		// os.Root rejects absolute symlinks even when rooted at /, so use normal
		// filesystem resolution here. Keep confinement for narrower engine roots.
		fsys = os.DirFS(resolved)
	}
	return &desktopEngine{
		root:    osRoot,
		handler: fileServer{fsys: fsys, pandoc: pandoc, dir: resolved},
		active:  make(map[*desktopRequest]struct{}),
	}, nil
}

func registerDesktopEngine(root, pandoc string) (uint64, error) {
	engine, err := newDesktopEngine(root, pandoc)
	if err != nil {
		return 0, err
	}
	id := desktopHandles.next.Add(1)
	desktopHandles.Lock()
	desktopHandles.engines[id] = engine
	desktopHandles.Unlock()
	return id, nil
}

func destroyDesktopEngine(id uint64) {
	desktopHandles.Lock()
	engine := desktopHandles.engines[id]
	delete(desktopHandles.engines, id)
	desktopHandles.Unlock()
	if engine != nil {
		engine.close()
	}
}

// close is nonblocking: cancel outstanding render work immediately, and keep
// the root descriptor alive until each handler has actually returned.
func (e *desktopEngine) close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return
	}
	e.closed = true
	for request := range e.active {
		request.cancel()
	}
	if len(e.active) == 0 {
		e.root.Close()
	}
}

func (e *desktopEngine) requestDone(request *desktopRequest) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.active, request)
	if e.closed && len(e.active) == 0 {
		e.root.Close()
	}
}

func newDesktopRequest() *desktopRequest {
	ctx, cancel := context.WithCancel(context.Background())
	reader, writer := io.Pipe()
	return &desktopRequest{
		ctx: ctx, cancelContext: cancel, body: reader, writer: writer,
		ready: make(chan struct{}), done: make(chan struct{}),
	}
}

func startDesktopRequest(engineID uint64, inputJSON string) uint64 {
	request := newDesktopRequest()
	id := desktopHandles.next.Add(1)
	desktopHandles.Lock()
	desktopHandles.requests[id] = request
	engine := desktopHandles.engines[engineID]
	desktopHandles.Unlock()
	if engine == nil {
		request.fail(errors.New("unknown or closed engine"))
		return id
	}
	input, err := decodeDesktopRequest(inputJSON)
	if err != nil {
		request.fail(err)
		return id
	}
	engine.start(request, input)
	return id
}

func getDesktopRequest(id uint64) *desktopRequest {
	desktopHandles.Lock()
	defer desktopHandles.Unlock()
	return desktopHandles.requests[id]
}

func finishDesktopRequest(id uint64) {
	desktopHandles.Lock()
	request := desktopHandles.requests[id]
	delete(desktopHandles.requests, id)
	desktopHandles.Unlock()
	if request != nil {
		request.cancel()
	}
}

func decodeDesktopRequest(inputJSON string) (desktopRequestInput, error) {
	var input desktopRequestInput
	if len(inputJSON) > 1<<20 {
		return input, errors.New("request metadata is too large")
	}
	if err := json.Unmarshal([]byte(inputJSON), &input); err != nil {
		return input, fmt.Errorf("invalid request JSON: %w", err)
	}
	u, err := url.Parse(input.URL)
	if err != nil || u.Scheme != "fb" || u.Host != "local" || u.User != nil || u.Opaque != "" {
		return input, errors.New("only fb://local/ URLs are allowed")
	}
	if strings.ContainsRune(u.Path, '\x00') || (u.Path != "" && !strings.HasPrefix(u.Path, "/")) {
		return input, errors.New("invalid resource path")
	}
	if input.Method == "" {
		input.Method = http.MethodGet
	}
	if input.Method != http.MethodGet && input.Method != http.MethodHead {
		return input, errors.New("only GET and HEAD requests are allowed")
	}
	if input.BodyBase64 != "" {
		body, err := base64.StdEncoding.DecodeString(input.BodyBase64)
		if err != nil || len(body) != 0 {
			return input, errors.New("GET and HEAD requests must not have a body")
		}
	}
	for key, value := range input.Headers {
		if !validDesktopHeaderName(key) || strings.ContainsAny(value, "\r\n\x00") {
			return input, errors.New("invalid request header")
		}
	}
	return input, nil
}

func validDesktopHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || strings.ContainsRune("!#$%&'*+-.^_`|~", c) {
			continue
		}
		return false
	}
	return true
}

func (e *desktopEngine) start(request *desktopRequest, input desktopRequestInput) {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		request.fail(errors.New("engine is closed"))
		return
	}
	e.active[request] = struct{}{}
	e.mu.Unlock()
	go e.serve(request, input)
}

func (e *desktopEngine) serve(request *desktopRequest, input desktopRequestInput) {
	writer := &desktopResponseWriter{request: request, header: make(http.Header), head: input.Method == http.MethodHead}
	defer func() {
		if recovered := recover(); recovered != nil {
			err := fmt.Errorf("rendering failed: %v", recovered)
			request.publish(desktopResponse{Error: err.Error(), Headers: map[string]string{}})
			request.writer.CloseWithError(err)
		} else {
			writer.WriteHeader(http.StatusOK)
			request.writer.Close()
		}
		request.cancelContext()
		e.requestDone(request)
		close(request.done)
	}()
	u, _ := url.Parse(input.URL) // Already validated by decodeDesktopRequest.
	// net/http is a request/response API as well as a transport. These values
	// are synthetic and never reach a network listener or client.
	u.Scheme, u.Host, u.Fragment = "http", "localhost", ""
	if u.Path == "" {
		u.Path = "/"
	}
	r, err := http.NewRequestWithContext(request.ctx, input.Method, u.String(), nil)
	if err != nil {
		request.publish(desktopResponse{Error: err.Error(), Headers: map[string]string{}})
		request.writer.CloseWithError(err)
		return
	}
	// Server-side requests normally carry a path-only URL and a separate Host.
	// Match that shape so existing directory redirects stay scheme-relative.
	r.URL.Scheme, r.URL.Host = "", ""
	for key, value := range input.Headers {
		r.Header.Set(key, value)
	}
	// WebKit does not consistently provide Sec-Fetch-Dest for custom schemes.
	// Native navigation policy supplies this bit explicitly for every request.
	if input.Document {
		r.Header.Set("Sec-Fetch-Dest", "document")
	} else {
		r.Header.Set("Sec-Fetch-Dest", "empty")
	}
	name := requestName(r.URL.Path)
	if name == assetPrefix+"/healthz" || name == assetPrefix+"/version" {
		http.NotFound(writer, r)
		return
	}
	if name == assetPrefix+"/drop" || name == assetPrefix+"/cd" || name == assetPrefix+"/drops" || strings.HasPrefix(name, assetPrefix+"/drops/") {
		http.Error(writer, "use native File > Open or Finder drag-and-drop", http.StatusNotFound)
		return
	}
	e.handler.ServeHTTP(writer, r)
}

func (r *desktopRequest) publish(response desktopResponse) {
	r.responseOnce.Do(func() {
		r.response = response
		close(r.ready)
	})
}

func (r *desktopRequest) metadata() desktopResponse {
	<-r.ready
	return r.response
}

func (r *desktopRequest) cancel() {
	r.cancelContext()
	r.publish(desktopResponse{Error: context.Canceled.Error(), Headers: map[string]string{}})
	r.body.CloseWithError(context.Canceled)
	r.writer.CloseWithError(context.Canceled)
}

func (r *desktopRequest) fail(err error) {
	r.publish(desktopResponse{Error: err.Error(), Headers: map[string]string{}})
	r.writer.CloseWithError(err)
	r.cancelContext()
	close(r.done)
}

type desktopResponseWriter struct {
	request *desktopRequest
	header  http.Header
	status  int
	head    bool
}

func (w *desktopResponseWriter) Header() http.Header { return w.header }

func (w *desktopResponseWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	if status < 100 || status > 999 {
		panic(fmt.Sprintf("invalid response status: %d", status))
	}
	if status < 200 {
		return // The bridge sends one final response, never interim headers.
	}
	w.status = status
	headers := make(map[string]string, len(w.header))
	for key, values := range w.header {
		headers[key] = strings.Join(values, ", ")
	}
	w.request.publish(desktopResponse{Status: status, Headers: headers})
}

func (w *desktopResponseWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		if _, exists := w.header["Content-Type"]; !exists && len(data) != 0 {
			w.header.Set("Content-Type", http.DetectContentType(data))
		}
		w.WriteHeader(http.StatusOK)
	}
	if w.head || w.status == http.StatusNoContent || w.status == http.StatusNotModified {
		return len(data), nil
	}
	return w.request.writer.Write(data)
}

func (w *desktopResponseWriter) Flush() { w.WriteHeader(http.StatusOK) }
