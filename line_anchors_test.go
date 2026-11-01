package main

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestLineTargetIsTheFirstLineNamed(t *testing.T) {
	for fragment, want := range map[string]int{"L42": 42, "L10-L20": 10, "L20-L10": 10, "L10-20": 10} {
		target, ok := lineTargetOf(withLineTarget(context.Background(), fragment))
		if !ok || target.line != want {
			t.Errorf("%q: got %v, %v; want line %d", fragment, target, ok, want)
		}
	}
	for _, fragment := range []string{"L99999999999999999999999", "42", "heading"} {
		if hasLineTarget(withLineTarget(context.Background(), fragment)) {
			t.Errorf("%q should not produce a target", fragment)
		}
	}
}

func TestLineFragmentForms(t *testing.T) {
	for _, fragment := range []string{"L1", "L42", "L10-L20", "L10-20", "L120"} {
		if !lineFragment.MatchString(fragment) {
			t.Errorf("%q should name a line", fragment)
		}
	}
	// A bare number can be a heading's id, so it must not switch views.
	for _, fragment := range []string{"", "42", "L", "L0", "l42", "L42x", "L1-", "second-heading", "cell=1,2"} {
		if lineFragment.MatchString(fragment) {
			t.Errorf("%q should not name a line", fragment)
		}
	}
}

// lineRequest is a navigation carrying a line target, as the desktop bridge
// builds it from a URL with a #L fragment.
func lineRequest(target string, targeted bool) *http.Request {
	r := httptest.NewRequest(http.MethodGet, target, nil)
	r.Header.Set("Sec-Fetch-Dest", "document")
	if targeted {
		r = r.WithContext(withLineTarget(context.Background(), "L2"))
	}
	return r
}

func writeTree(t *testing.T, files map[string][]byte) string {
	t.Helper()
	root := t.TempDir()
	for name, data := range files {
		full := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestLineTargetSwitchesTextFilesToNumberedView(t *testing.T) {
	root := writeTree(t, map[string][]byte{
		"notes.txt":     []byte("one\ntwo\nthree\n"),
		"server.log":    []byte("one\ntwo\n\x1b[31mred\x1b[0m\n"),
		"README.md":     []byte("# Title\n\ntext\n"),
		"data.csv":      []byte("a,b\n1,2\n"),
		"page.html":     []byte("<script>window.bad = 1</script>\n<p>two</p>\n"),
		"go.sum":        []byte("example.com/a v1.0.0 h1:abc=\nexample.com/a v1.0.0/go.mod h1:def=\n"),
		"extensionless": []byte("one\ntwo\n"),
	})
	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}

	for _, name := range []string{"notes.txt", "server.log", "README.md", "data.csv", "page.html", "go.sum", "extensionless"} {
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, lineRequest("/"+name, true))
		body := rec.Body.String()
		if !strings.Contains(body, `id="L2"`) || !strings.Contains(body, "line-target") {
			t.Errorf("%s: want a numbered view with line anchors, got %.200q", name, body)
		}
		if rec.Header().Get("Content-Security-Policy") == "" {
			t.Errorf("%s: numbered view lost security headers", name)
		}
	}

	// Source text is escaped into the page, never live.
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, lineRequest("/page.html", true))
	if strings.Contains(rec.Body.String(), "<script>window.bad") {
		t.Fatal("numbered view of HTML must not embed the file's markup")
	}
}

func TestNumberedViewLinksBackOnlyWhenARenderedViewExists(t *testing.T) {
	root := writeTree(t, map[string][]byte{
		"README.md": []byte("# Title\n\ntext\n"),
		"notes.txt": []byte("one\ntwo\n"),
	})
	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}

	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, lineRequest("/README.md", true))
	if !strings.Contains(rec.Body.String(), `<a class="raw alt" href="README.md?view=rendered">rendered</a>`) {
		t.Fatalf("Markdown line view has no link back: %.300q", rec.Body.String())
	}

	// The link does not name a line, so it opens the file's normal view.
	if !strings.Contains(rec.Body.String(), `id="L1"`) {
		t.Fatal("the line view should be numbered")
	}
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, lineRequest("/notes.txt?view=rendered", true))
	if strings.Contains(rec.Body.String(), `id="L1"`) {
		t.Fatal("a query should keep the normal view, whatever the fragment")
	}

	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, lineRequest("/notes.txt", true))
	if strings.Contains(rec.Body.String(), ">rendered</a>") {
		t.Fatal("plain text has no rendered view to link back to")
	}
}

func TestLineViewOnlyWhenRequested(t *testing.T) {
	root := writeTree(t, map[string][]byte{"notes.txt": []byte("one\ntwo\n")})
	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}

	// Without a target the view is unchanged.
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, lineRequest("/notes.txt", false))
	if strings.Contains(rec.Body.String(), `id="L1"`) || rec.Body.String() != "one\ntwo\n" {
		t.Fatalf("plain text changed without a line target: %q", rec.Body.String())
	}

	// ?raw=1 still returns the original bytes.
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, lineRequest("/notes.txt?raw=1", true))
	if rec.Body.String() != "one\ntwo\n" {
		t.Fatalf("raw request was reformatted: %q", rec.Body.String())
	}

	// Subresource loads are never reformatted.
	r := lineRequest("/notes.txt", true)
	r.Header.Set("Sec-Fetch-Dest", "empty")
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, r)
	if rec.Body.String() != "one\ntwo\n" {
		t.Fatalf("subresource request was reformatted: %q", rec.Body.String())
	}

	// A query names a specific view, which keeps priority.
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, lineRequest("/notes.txt?other=1", true))
	if strings.Contains(rec.Body.String(), `id="L1"`) {
		t.Fatal("a query string should keep the requested view")
	}
}

func TestLineTargetIgnoredForBinaryContent(t *testing.T) {
	root := writeTree(t, map[string][]byte{
		"blob.bin":   {0x00, 0x01, 0x02, '\n', 0x03},
		"latin1.txt": {'c', 'a', 'f', 0xe9, '\n'},
		"image.png":  []byte("not really an image, but the extension decides\n"),
		"paper.pdf":  []byte("%PDF-1.4 ascii only\n"),
	})
	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}
	for _, name := range []string{"blob.bin", "latin1.txt", "image.png", "paper.pdf"} {
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, lineRequest("/"+name, true))
		if strings.Contains(rec.Body.String(), `id="L1"`) {
			t.Errorf("%s: binary content should ignore a line target", name)
		}
	}
}

func numberedLines(n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "line %d of the file\n", i)
	}
	return b.String()
}

func lineTargetRequest(target, fragment string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, target, nil)
	r.Header.Set("Sec-Fetch-Dest", "document")
	return r.WithContext(withLineTarget(context.Background(), fragment))
}

func TestLongFilesShowAWindowAroundTheTarget(t *testing.T) {
	long := numberedLines(maxWholeNumberedLines + 20000)
	if len(long) <= maxHighlightBytes {
		long += strings.Repeat("padding to pass the highlighting cap\n", maxHighlightBytes/36)
	}
	root := writeTree(t, map[string][]byte{
		"big.log":   []byte(long),
		"big.go":    []byte(long),
		"short.txt": []byte(numberedLines(maxWholeNumberedLines - 1)),
		"long.txt":  []byte(numberedLines(maxWholeNumberedLines + 100)),
		"small.go":  []byte("package main\n"),
	})
	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}

	for _, name := range []string{"big.log", "big.go"} {
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, lineTargetRequest("/"+name, "L9000"))
		body := rec.Body.String()
		for _, want := range []string{
			`id="L9000"`, `id="L7000"`, `id="L10999"`, `class="source" data-windowed`,
			"Showing lines 7000&ndash;10999", `href="#L6999"`, `href="#L11000"`, "line 9000 of the file",
		} {
			if !strings.Contains(body, want) {
				t.Errorf("%s: window is missing %q", name, want)
			}
		}
		for _, absent := range []string{`id="L6999"`, `id="L11000"`, `id="L1"`} {
			if strings.Contains(body, absent) {
				t.Errorf("%s: window should not contain %q", name, absent)
			}
		}
	}

	// A file short enough to lay out quickly is shown whole, with no window.
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, lineTargetRequest("/short.txt", "L9000"))
	if body := rec.Body.String(); !strings.Contains(body, `id="L1"`) || strings.Contains(body, `class="source" data-windowed`) {
		t.Fatal("short.txt should be shown whole")
	}
	// Within the highlighting cap but too long to lay out quickly.
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, lineTargetRequest("/long.txt", "L5"))
	if body := rec.Body.String(); !strings.Contains(body, `class="source" data-windowed`) || !strings.Contains(body, `id="L1"`) {
		t.Fatal("long.txt should show a window that starts at the top")
	}
	if strings.Contains(rec.Body.String(), "Earlier lines") {
		t.Fatal("a window at the top of the file has nothing earlier")
	}

	// Without a target the highlighting cap behaves as before.
	rec = httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/big.log", nil)
	r.Header.Set("Sec-Fetch-Dest", "document")
	server.ServeHTTP(rec, r)
	if strings.Contains(rec.Body.String(), `id="L1"`) {
		t.Fatal("big.log without a line target should stay plain")
	}
	// Files that already have a numbered view keep their normal viewer.
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, lineRequest("/small.go", true))
	if body := rec.Body.String(); !strings.Contains(body, `id="L1"`) || strings.Contains(body, ">rendered</a>") {
		t.Fatal("small.go should keep its normal numbered viewer")
	}
}

func openString(content string) func() (io.ReadCloser, error) {
	return func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(content)), nil }
}

func TestReadLineWindow(t *testing.T) {
	ctx := context.Background()
	read := func(t *testing.T, content string, target int) lineWindow {
		t.Helper()
		window, ok, err := findLineWindow(ctx, openString(content), target)
		if err != nil || !ok {
			t.Fatalf("findLineWindow: ok=%v err=%v", ok, err)
		}
		return window
	}

	t.Run("target past the end shows the end of the file", func(t *testing.T) {
		total := lineWindowContext*3 + 7
		window := read(t, numberedLines(total), total+50000)
		last := window.lines[len(window.lines)-1]
		if last != fmt.Sprintf("line %d of the file", total) || window.more {
			t.Fatalf("last line %q, more=%v", last, window.more)
		}
		if window.start != total-lineWindowContext {
			t.Fatalf("start = %d", window.start)
		}
	})
	t.Run("numbers stay true at the start of the file", func(t *testing.T) {
		window := read(t, numberedLines(50), 10)
		if window.start != 1 || len(window.lines) != 50 || window.more || window.lines[9] != "line 10 of the file" {
			t.Fatalf("start=%d lines=%d more=%v", window.start, len(window.lines), window.more)
		}
	})
	t.Run("the target stays in the window", func(t *testing.T) {
		target := lineWindowContext + 500
		window := read(t, numberedLines(lineWindowContext*4), target)
		if got := window.lines[target-window.start]; got != fmt.Sprintf("line %d of the file", target) {
			t.Fatalf("line at target = %q", got)
		}
		if !window.more || window.start != target-lineWindowContext || len(window.lines) != lineWindowContext*2 {
			t.Fatalf("start=%d lines=%d more=%v", window.start, len(window.lines), window.more)
		}
	})
	t.Run("a huge line is truncated without splitting a character", func(t *testing.T) {
		huge := strings.Repeat("é", maxWindowLineBytes) + "\nnext\n"
		window := read(t, huge, 1)
		if len(window.lines) != 2 || window.lines[1] != "next" || !strings.HasSuffix(window.lines[0], "[line truncated]") {
			t.Fatalf("lines=%d", len(window.lines))
		}
		if !utf8.ValidString(window.lines[0]) || len(window.lines[0]) > maxWindowLineBytes+32 {
			t.Fatalf("truncated line is invalid or too long: %d bytes", len(window.lines[0]))
		}
	})
	t.Run("the total held is bounded", func(t *testing.T) {
		line := strings.Repeat("x", maxWindowLineBytes-1) + "\n"
		count := (maxWindowBytes/len(line) + 1) * 3
		for _, target := range []int{1, count / 2, count + 10} {
			window := read(t, strings.Repeat(line, count), target)
			held := 0
			for _, l := range window.lines {
				held += len(l)
			}
			if held > maxWindowBytes+maxWindowLineBytes {
				t.Fatalf("target %d: held %d bytes", target, held)
			}
		}
	})
	t.Run("a final line with no newline and CRLF endings", func(t *testing.T) {
		window := read(t, "one\r\ntwo\r\nthree", 2)
		if strings.Join(window.lines, "|") != "one|two|three" {
			t.Fatalf("lines = %q", window.lines)
		}
	})
	t.Run("binary content is refused", func(t *testing.T) {
		if _, ok, err := findLineWindow(ctx, openString("one\ntw\x00o\n"), 1); ok || err != nil {
			t.Fatalf("NUL byte: ok=%v err=%v", ok, err)
		}
		if _, ok, err := findLineWindow(ctx, openString("caf\xe9\n"), 1); ok || err != nil {
			t.Fatalf("invalid UTF-8 in the window: ok=%v err=%v", ok, err)
		}
	})
	t.Run("invalid UTF-8 far from the target is tolerated", func(t *testing.T) {
		content := "caf\xe9\n" + numberedLines(lineWindowContext*3)
		window := read(t, content, lineWindowContext*2)
		if window.start <= 1 {
			t.Fatalf("start = %d", window.start)
		}
	})
	t.Run("cancellation stops a long scan", func(t *testing.T) {
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		if _, ok, err := findLineWindow(cancelled, openString(numberedLines(10000)), 9000); ok || err == nil {
			t.Fatalf("ok=%v err=%v", ok, err)
		}
	})
}

func TestLineTargetInsideArchive(t *testing.T) {
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	w, err := zw.Create("docs/notes.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("one\ntwo\nthree\n")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	root := writeTree(t, map[string][]byte{"bundle.zip": archive.Bytes()})
	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}

	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, lineRequest("/bundle.zip/docs/notes.txt", true))
	if !strings.Contains(rec.Body.String(), `id="L2"`) {
		t.Fatalf("archive member should get a numbered view: %.200q", rec.Body.String())
	}
}

func TestEveryNumberedPageCarriesLineAnchorSupport(t *testing.T) {
	root := writeTree(t, map[string][]byte{
		"main.go":  []byte("package main\n"),
		"go.mod":   []byte("module example.com/m\n\ngo 1.22\n"),
		"cal.ics":  []byte("BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nSUMMARY:Meeting\r\nDTSTART:20260101T100000Z\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"),
		"not.ics":  []byte("no calendar here\n"),
		"bad.mod":  []byte("not a module file\n"),
		"data.typ": []byte("= Title\n"),
	})
	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}
	for _, name := range []string{"main.go", "go.mod", "cal.ics", "not.ics", "data.typ"} {
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, lineRequest("/"+name, false))
		body := rec.Body.String()
		for _, want := range []string{`id="L1"`, "scroll-margin-top", "line-target", `addEventListener("hashchange"`} {
			if !strings.Contains(body, want) {
				t.Errorf("%s: page is missing %q", name, want)
			}
		}
	}
}

func TestJSONKeepsItsLinesUnlessMinified(t *testing.T) {
	formatted := "{\n  \"a\": 1, \"b\": 2,\n\n  \"z\": 9\n}\n"
	root := writeTree(t, map[string][]byte{"hand.json": []byte(formatted)})
	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, lineRequest("/hand.json", false))
	body := rec.Body.String()
	if !strings.Contains(body, `id="L5"`) || strings.Contains(body, `id="L6"`) {
		t.Fatalf("formatted JSON should keep its five lines: %.400q", body)
	}

	for data, want := range map[string]bool{
		`{"a":1}`:              true,
		"{\"a\":1}\n":          true,
		"  {\"a\":1}  \n\n":    true,
		"{\n\"a\":1\n}":        false,
		"{\"a\":1}\n{\"b\":2}": false,
	} {
		if got := isMinifiedJSON([]byte(data)); got != want {
			t.Errorf("isMinifiedJSON(%q) = %v, want %v", data, got, want)
		}
	}
}
