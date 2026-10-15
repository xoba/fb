//go:build fbmac

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func desktopTestEngine(t *testing.T) (uint64, *desktopEngine, string) {
	t.Helper()
	root := t.TempDir()
	id, err := registerDesktopEngine(root, fakePandoc(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { destroyDesktopEngine(id) })
	desktopHandles.Lock()
	engine := desktopHandles.engines[id]
	desktopHandles.Unlock()
	return id, engine, root
}

func desktopTestStart(t *testing.T, engine uint64, input desktopRequestInput) (uint64, *desktopRequest) {
	t.Helper()
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	id := startDesktopRequest(engine, string(encoded))
	t.Cleanup(func() { finishDesktopRequest(id) })
	return id, getDesktopRequest(id)
}

func desktopTestFetch(t *testing.T, engine uint64, input desktopRequestInput) (desktopResponse, []byte) {
	t.Helper()
	_, request := desktopTestStart(t, engine, input)
	response := request.metadata()
	if response.Error != "" {
		t.Fatal(response.Error)
	}
	body, err := io.ReadAll(request.body)
	if err != nil {
		t.Fatal(err)
	}
	return response, body
}

func waitDesktopDone(t *testing.T, request *desktopRequest) {
	t.Helper()
	select {
	case <-request.done:
	case <-time.After(5 * time.Second):
		t.Fatal("request did not stop after cancellation")
	}
}

func TestDesktopMarkdownAndAssets(t *testing.T) {
	id, _, root := desktopTestEngine(t)
	if err := os.WriteFile(filepath.Join(root, "notes.md"), []byte("# Heading\n"), 0600); err != nil {
		t.Fatal(err)
	}
	response, body := desktopTestFetch(t, id, desktopRequestInput{URL: "fb://local/notes.md", Document: true})
	if response.Status != http.StatusOK || !bytes.Contains(body, []byte("rendered markdown")) {
		t.Fatalf("markdown status=%d body=%.200s", response.Status, body)
	}
	if response.Headers["Content-Security-Policy"] == "" {
		t.Fatal("rendered document lost security headers")
	}
	response, body = desktopTestFetch(t, id, desktopRequestInput{URL: "fb://local/_fb/favicon.png"})
	want, err := fs.ReadFile(embeddedAssets, "assets/favicon.png")
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != http.StatusOK || !bytes.Equal(body, want) {
		t.Fatal("embedded asset did not survive bridge")
	}
}

func TestDesktopPagesHaveNoServerFooter(t *testing.T) {
	id, _, root := desktopTestEngine(t)
	for name, content := range map[string]string{
		"notes.md":  "# Heading\n",
		"notes.rst": "Heading\n=======\n",
		"code.go":   "package main\n",
		"data.csv":  "a,b\n1,2\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{"/", "/notes.md", "/notes.rst", "/code.go", "/data.csv"} {
		response, body := desktopTestFetch(t, id, desktopRequestInput{URL: "fb://local" + path, Document: true})
		if response.Status != http.StatusOK {
			t.Fatalf("%s: status=%d", path, response.Status)
		}
		if bytes.Contains(body, []byte("/_fb/healthz")) || bytes.Contains(body, []byte("/_fb/version")) {
			t.Errorf("%s contains web-server diagnostic links", path)
		}
	}
}

func TestDesktopServerDiagnosticsUnavailable(t *testing.T) {
	id, _, _ := desktopTestEngine(t)
	for _, path := range []string{"/_fb/healthz", "/_fb/version"} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			response, body := desktopTestFetch(t, id, desktopRequestInput{URL: "fb://local" + path, Method: method})
			if response.Status != http.StatusNotFound {
				t.Errorf("%s %s: status=%d, want 404", method, path, response.Status)
			}
			if method == http.MethodHead && len(body) != 0 {
				t.Errorf("HEAD %s returned a response body", path)
			}
		}
	}
}

func TestDesktopRealPandoc(t *testing.T) {
	pandoc := os.Getenv("FB_TEST_PANDOC")
	if pandoc == "" {
		var err error
		pandoc, err = exec.LookPath("pandoc")
		if err != nil {
			t.Skip("set FB_TEST_PANDOC to test the packaged converter")
		}
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.md"), []byte("# Desktop heading\n\n**Embedded renderer**\n"), 0600); err != nil {
		t.Fatal(err)
	}
	id, err := registerDesktopEngine(root, pandoc)
	if err != nil {
		t.Fatal(err)
	}
	defer destroyDesktopEngine(id)
	response, body := desktopTestFetch(t, id, desktopRequestInput{URL: "fb://local/notes.md", Document: true})
	if response.Status != http.StatusOK || !bytes.Contains(body, []byte("<strong>Embedded renderer</strong>")) || !bytes.Contains(body, []byte("desktop-heading")) {
		t.Fatalf("real pandoc render status=%d body=%.200s", response.Status, body)
	}
}

func TestDesktopRawRangeAndHead(t *testing.T) {
	id, _, root := desktopTestEngine(t)
	content := []byte("0123456789abcdef")
	if err := os.WriteFile(filepath.Join(root, "data.bin"), content, 0600); err != nil {
		t.Fatal(err)
	}
	response, body := desktopTestFetch(t, id, desktopRequestInput{URL: "fb://local/data.bin?raw=1", Headers: map[string]string{"Range": "bytes=3-7"}})
	if response.Status != http.StatusPartialContent || string(body) != "34567" || response.Headers["Content-Range"] != "bytes 3-7/16" {
		t.Fatalf("range response=%+v body=%q", response, body)
	}
	response, body = desktopTestFetch(t, id, desktopRequestInput{URL: "fb://local/data.bin?raw=1", Method: http.MethodHead})
	if response.Status != http.StatusOK || len(body) != 0 || response.Headers["Content-Length"] != "16" {
		t.Fatalf("head response=%+v body=%q", response, body)
	}
}

func TestDesktopNavigationVersusResource(t *testing.T) {
	id, _, root := desktopTestEngine(t)
	content := "body { color: red; }"
	if err := os.WriteFile(filepath.Join(root, "style.css"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	_, resource := desktopTestFetch(t, id, desktopRequestInput{URL: "fb://local/style.css", Headers: map[string]string{"Accept": "text/html"}})
	_, document := desktopTestFetch(t, id, desktopRequestInput{URL: "fb://local/style.css", Document: true})
	if string(resource) != content || bytes.Equal(resource, document) || !bytes.Contains(document, []byte("<html")) {
		t.Fatal("document bit did not distinguish navigation from raw resource")
	}
}

func TestDesktopRedirectAndRawSecurity(t *testing.T) {
	id, _, root := desktopTestEngine(t)
	if err := os.Mkdir(filepath.Join(root, "folder"), 0700); err != nil {
		t.Fatal(err)
	}
	response, _ := desktopTestFetch(t, id, desktopRequestInput{URL: "fb://local/folder?q=one", Document: true})
	if response.Status != http.StatusMovedPermanently || response.Headers["Location"] != "/folder/?q=one" {
		t.Fatalf("directory redirect=%+v", response)
	}
	for _, name := range []string{"unsafe.html", "unsafe.svg"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("<script>alert(1)</script>"), 0600); err != nil {
			t.Fatal(err)
		}
		response, _ := desktopTestFetch(t, id, desktopRequestInput{URL: "fb://local/" + name, Document: true})
		if response.Headers["Content-Security-Policy"] != "sandbox; frame-ancestors 'none'" {
			t.Fatalf("%s lost raw content sandbox: %+v", name, response)
		}
	}
}

func TestDesktopSymlinkConfinement(t *testing.T) {
	id, _, root := desktopTestEngine(t)
	outside := filepath.Join(t.TempDir(), "private.txt")
	if err := os.WriteFile(outside, []byte("outside-root-secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape.txt")); err != nil {
		t.Fatal(err)
	}
	response, body := desktopTestFetch(t, id, desktopRequestInput{URL: "fb://local/escape.txt?raw=1"})
	if response.Status == http.StatusOK || bytes.Contains(body, []byte("outside-root-secret")) {
		t.Fatalf("symlink escaped root: response=%+v body=%q", response, body)
	}
}

func TestDesktopFilesystemRootSymlinks(t *testing.T) {
	// The native app browses from /, not from the folder currently on screen.
	// Absolute symlinks must work there just as they do in Finder.
	id, err := registerDesktopEngine(string(os.PathSeparator), fakePandoc(t))
	if err != nil {
		t.Fatal(err)
	}
	defer destroyDesktopEngine(id)
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"notes.md":  "# Linked document\n",
		"style.css": "body { color: red; }",
	} {
		if err := os.WriteFile(filepath.Join(target, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for name, destination := range map[string]string{
		"absolute & café #1": target,
		"relative":           "target",
		"chain":              "absolute & café #1",
		"file.md":            filepath.Join(target, "notes.md"),
		"broken":             filepath.Join(root, "missing"),
		"loop":               "loop",
	} {
		if err := os.Symlink(destination, filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}
	address := func(name string) string {
		return (&url.URL{Scheme: "fb", Host: "local", Path: root + "/" + name}).String()
	}
	for _, alias := range []string{"absolute & café #1", "relative", "chain"} {
		t.Run(alias, func(t *testing.T) {
			response, _ := desktopTestFetch(t, id, desktopRequestInput{URL: address(alias) + "?sort=name", Document: true})
			wantLocation := (&url.URL{Path: root + "/" + alias + "/", RawQuery: "sort=name"}).String()
			if response.Status != http.StatusMovedPermanently || response.Headers["Location"] != wantLocation {
				t.Fatalf("symlink directory redirect=%+v, want %q", response, wantLocation)
			}
			for _, test := range []struct {
				path, want string
				document   bool
			}{
				{alias + "/", "notes.md", true},
				{alias + "/notes.md", "rendered markdown", true},
				{alias + "/style.css", "body { color: red; }", false},
			} {
				response, body := desktopTestFetch(t, id, desktopRequestInput{URL: address(test.path), Document: test.document})
				if response.Status != http.StatusOK || !bytes.Contains(body, []byte(test.want)) {
					t.Errorf("%s: status=%d body=%.200s", test.path, response.Status, body)
				}
			}
		})
	}
	for _, raw := range []bool{false, true} {
		requestURL, want := address("file.md"), "rendered markdown"
		if raw {
			requestURL += "?raw=1"
			want = "# Linked document\n"
		}
		response, body := desktopTestFetch(t, id, desktopRequestInput{URL: requestURL, Document: true})
		if response.Status != http.StatusOK || !bytes.Contains(body, []byte(want)) {
			t.Errorf("file symlink raw=%v: status=%d body=%.200s", raw, response.Status, body)
		}
	}
	for _, name := range []string{"broken", "loop"} {
		response, _ := desktopTestFetch(t, id, desktopRequestInput{URL: address(name), Document: true})
		if response.Status != http.StatusNotFound {
			t.Errorf("%s symlink: status=%d, want 404", name, response.Status)
		}
	}
}

func TestDesktopRawBackupContentStaysInert(t *testing.T) {
	id, _, root := desktopTestEngine(t)
	content := []byte("<!doctype html><script>window.fbUnsafeScriptExecuted = true</script>")
	for _, name := range []string{"payload.txt~", "payload.md~"} {
		if err := os.WriteFile(filepath.Join(root, name), content, 0600); err != nil {
			t.Fatal(err)
		}
		response, body := desktopTestFetch(t, id, desktopRequestInput{URL: "fb://local/" + name + "?raw=1", Document: true})
		if response.Status != http.StatusOK || !bytes.Equal(body, content) {
			t.Fatalf("%s raw backup response=%+v body=%q", name, response, body)
		}
		if ctype := response.Headers["Content-Type"]; ctype == "" || activeContentType(ctype) {
			t.Errorf("%s raw backup has unsafe Content-Type %q", name, ctype)
		}
	}
}

func TestDesktopDisabledWebMutations(t *testing.T) {
	id, _, _ := desktopTestEngine(t)
	for _, path := range []string{"/_fb/drop", "/_fb/cd", "/_fb/drops/old/file.md"} {
		response, _ := desktopTestFetch(t, id, desktopRequestInput{URL: "fb://local" + path})
		if response.Status != http.StatusNotFound {
			t.Fatalf("%s status=%d, want 404", path, response.Status)
		}
	}
}

func TestDesktopMalformedRequests(t *testing.T) {
	for _, input := range []string{
		`not json`, `null`, `{}`, `{"url":"http://localhost/file"}`,
		`{"url":"fb://foreign/file"}`, `{"url":"fb://local:80/file"}`,
		`{"url":"fb://user@local/file"}`, `{"url":"fb://local/%00"}`,
		`{"url":"fb://local/%xx"}`, `{"url":"fb://local/","method":"POST"}`,
		`{"url":"fb://local/","bodyBase64":"YQ=="}`,
		`{"url":"fb://local/","headers":{"Invalid Header":"one"}}`,
		`{"url":"fb://local/","headers":{"X-Test":"one\r\ntwo"}}`,
	} {
		if _, err := decodeDesktopRequest(input); err == nil {
			t.Errorf("accepted invalid request %s", input)
		}
	}
	if _, err := newDesktopEngine(t.TempDir(), "pandoc"); err == nil {
		t.Fatal("accepted PATH-dependent pandoc")
	}
	requestID := startDesktopRequest(^uint64(0), `{"url":"fb://local/"}`)
	defer finishDesktopRequest(requestID)
	if getDesktopRequest(requestID).metadata().Error == "" {
		t.Fatal("unknown engine did not return an error")
	}
}

func TestDesktopCancelBeforeHeaders(t *testing.T) {
	id, engine, _ := desktopTestEngine(t)
	entered := make(chan struct{})
	engine.handler = http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
	})
	_, request := desktopTestStart(t, id, desktopRequestInput{URL: "fb://local/slow"})
	<-entered
	request.cancel()
	if request.metadata().Error != context.Canceled.Error() {
		t.Fatalf("cancel metadata=%+v", request.metadata())
	}
	waitDesktopDone(t, request)
}

func TestDesktopStreamingCancelAndDestroy(t *testing.T) {
	id, engine, root := desktopTestEngine(t)
	file, err := os.Create(filepath.Join(root, "large.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if err = file.Truncate(1 << 30); err != nil {
		t.Fatal(err)
	}
	file.Close()
	_, request := desktopTestStart(t, id, desktopRequestInput{URL: "fb://local/large.bin"})
	if request.metadata().Status != http.StatusOK {
		t.Fatalf("large file metadata=%+v", request.metadata())
	}
	// No consumer has read yet. The handler must be blocked in the pipe,
	// rather than having buffered the entire gigabyte or completed silently.
	select {
	case <-request.done:
		t.Fatal("large file handler finished before the consumer read")
	default:
	}
	destroyDesktopEngine(id)
	waitDesktopDone(t, request)
	if _, err := request.body.Read(make([]byte, 1)); err == nil {
		t.Fatal("destroy did not interrupt body reader")
	}
	if _, err := engine.root.Stat("."); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("root descriptor not closed after handlers stopped: %v", err)
	}
}

func TestDesktopPanicUnblocksReader(t *testing.T) {
	id, engine, _ := desktopTestEngine(t)
	engine.handler = http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("test panic") })
	_, request := desktopTestStart(t, id, desktopRequestInput{URL: "fb://local/"})
	if !strings.Contains(request.metadata().Error, "test panic") {
		t.Fatalf("panic metadata=%+v", request.metadata())
	}
	if _, err := io.ReadAll(request.body); err == nil {
		t.Fatal("panic did not report stream error")
	}
	waitDesktopDone(t, request)
}

func TestDesktopConcurrentEngineDestroy(t *testing.T) {
	id, _, _ := desktopTestEngine(t)
	var workers sync.WaitGroup
	for range 32 {
		workers.Go(func() {
			requestID := startDesktopRequest(id, `{"url":"fb://local/_fb/favicon.png"}`)
			defer finishDesktopRequest(requestID)
			request := getDesktopRequest(requestID)
			request.metadata()
			io.Copy(io.Discard, request.body)
			<-request.done
		})
	}
	destroyDesktopEngine(id)
	workers.Wait()
}
