package main

import (
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func emlInboxTestResponse(s fileServer, target, name string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, target, nil)
	w := httptest.NewRecorder()
	s.serveEMLInbox(w, r, name)
	return w
}

func TestEMLInboxDecodedHeadersEscapingAndDateOrder(t *testing.T) {
	files := fstest.MapFS{
		"old.eml":          {Data: []byte("From: Old <old@example.com>\r\nSubject: Older message\r\nDate: Mon, 12 Oct 2026 08:00:00 -0400\r\n\r\nold body")},
		"new.EML~":         {Data: []byte("From: =?ISO-8859-1?Q?Andr=E9?= <new@example.com>\r\nTo: Reader <reader@example.com>\r\nSubject: =?UTF-8?B?UsOpc3Vtw6k=?= <img src=x onerror=alert(1)>\r\nDate: Mon, 12 Oct 2026 09:00:00 -0400\r\n\r\nbody must not be indexed")},
		"broken.eml":       {Data: []byte("this is not an email")},
		"ignored.txt":      {Data: []byte("Subject: SHOULDNOTSHOW\n\n")},
		"nested/child.eml": {Data: []byte("Subject: child\n\n")},
	}
	w := emlInboxTestResponse(fileServer{fsys: files}, "/?view=inbox", ".")
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	html := w.Body.String()
	for _, want := range []string{"André &lt;new@example.com&gt;", "Résumé &lt;img src=x onerror=alert(1)&gt;", "2026-10-12 09:00", "Invalid message headers.", `href="/nested/?view=inbox"`, `href="/new.EML~"`, "3 messages"} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, unwanted := range []string{"<img src=x", "SHOULDNOTSHOW", "body must not be indexed"} {
		if strings.Contains(html, unwanted) {
			t.Errorf("unexpected %q", unwanted)
		}
	}
	if strings.Index(html, "Résumé &lt;img") > strings.Index(html, ">Older message</a>") {
		t.Fatal("messages not ordered newest first")
	}
}

func TestEMLInboxSearchIncludesRecipientsFilenameAndDecodedHeaders(t *testing.T) {
	files := fstest.MapFS{
		"a.eml":          {Data: []byte("From: Person <person@example.com>\nSubject: =?UTF-8?B?UsOpc3Vtw6k=?=\nTo: Special <special@example.com>\n\nbody-only-needle")},
		"UniqueFile.eml": {Data: []byte("Subject: Second message\n\n")},
	}
	s := fileServer{fsys: files}
	for _, tc := range []struct{ query, want, absent string }{
		{"r%C3%A9SUM%C3%A9", ">Résumé</a>", ">Second message</a>"},
		{"SPECIAL%40EXAMPLE.COM", ">Résumé</a>", ">Second message</a>"},
		{"uniquefile", ">Second message</a>", ">Résumé</a>"},
		{"body-only-needle", "No messages on this page match", ">Résumé</a>"},
	} {
		t.Run(tc.query, func(t *testing.T) {
			w := emlInboxTestResponse(s, "/?view=inbox&q="+tc.query, ".")
			if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), tc.want) || strings.Contains(w.Body.String(), tc.absent) {
				t.Fatalf("unexpected filtered inbox for %q: status %d", tc.query, w.Code)
			}
		})
	}
}

func TestEMLInboxArchiveURLsAndFallbackDate(t *testing.T) {
	files := fstest.MapFS{
		"mail 100%/résumé %.eml": {Data: []byte("Subject: Archive message\nDate: invalid\n\n"), ModTime: time.Date(2026, 10, 11, 13, 14, 0, 0, time.UTC)},
		"mail 100%/子/future.eml": {Data: []byte("Subject: future\n\n")},
	}
	s := fileServer{fsys: files, base: "/archive %.zip"}
	w := emlInboxTestResponse(s, "/archive%20%25.zip/mail%20100%25/?view=inbox", "mail 100%")
	html := w.Body.String()
	for _, want := range []string{
		`href="/archive%20%25.zip/mail%20100%25/r%C3%A9sum%C3%A9%20%25.eml"`,
		`href="/archive%20%25.zip/mail%20100%25/%E5%AD%90/?view=inbox"`,
		`href="/archive%20%25.zip/?view=inbox"`,
		`href="/archive%20%25.zip/mail%20100%25/">Files</a>`,
		"2026-10-11 13:14 UTC",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q", want)
		}
	}
}

type emlInboxCountingFS struct {
	fs.FS
	opens int
	bytes int
}

func (f *emlInboxCountingFS) Open(name string) (fs.File, error) {
	file, err := f.FS.Open(name)
	if err != nil {
		return nil, err
	}
	if emlInboxFile(name) {
		f.opens++
		return &emlInboxCountingFile{File: file, fsys: f}, nil
	}
	return file, nil
}

type emlInboxCountingFile struct {
	fs.File
	fsys *emlInboxCountingFS
}

func (f *emlInboxCountingFile) Read(p []byte) (int, error) {
	n, err := f.File.Read(p)
	f.fsys.bytes += n
	return n, err
}

func TestEMLInboxPaginationBoundsHeaderReads(t *testing.T) {
	files := fstest.MapFS{}
	for i := 0; i < emlInboxPageSize+1; i++ {
		files[fmt.Sprintf("message-%04d.eml", i)] = &fstest.MapFile{Data: []byte(fmt.Sprintf("Subject: Message %04d\n\n%s", i, strings.Repeat("body", 10000)))}
	}
	counted := &emlInboxCountingFS{FS: files}
	s := fileServer{fsys: counted}
	w := emlInboxTestResponse(s, "/?view=inbox", ".")
	if w.Code != http.StatusOK || counted.opens != emlInboxPageSize {
		t.Fatalf("status %d, opened %d messages", w.Code, counted.opens)
	}
	if counted.bytes > emlInboxPageSize*4096 {
		t.Fatalf("unexpected full message reads: %d bytes", counted.bytes)
	}
	for _, want := range []string{"Page 1 of 2", "this page only", `href="/?page=2&amp;view=inbox"`} {
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("missing %q", want)
		}
	}
	counted.opens = 0
	w = emlInboxTestResponse(s, "/?view=inbox&page=2", ".")
	if counted.opens != 1 || !strings.Contains(w.Body.String(), ">Message 0500</a>") || !strings.Contains(w.Body.String(), "Page 2 of 2") {
		t.Fatalf("wrong second page; opened %d messages", counted.opens)
	}
}

func TestEMLInboxHeaderLimitAndUnreadableFile(t *testing.T) {
	files := fstest.MapFS{
		"oversized.eml": {Data: []byte("Subject: " + strings.Repeat("x", emlInboxHeaderMax*2) + "\n\nbody")},
	}
	counted := &emlInboxCountingFS{FS: files}
	w := emlInboxTestResponse(fileServer{fsys: counted}, "/?view=inbox", ".")
	if counted.bytes > emlInboxHeaderMax+1 {
		t.Fatalf("read %d bytes past header cap", counted.bytes)
	}
	if !strings.Contains(w.Body.String(), "Headers exceed 64.0 KB.") || !strings.Contains(w.Body.String(), "oversized.eml") {
		t.Fatal("oversized email did not retain navigable row and warning")
	}
	entry, err := fs.ReadDir(files, ".")
	if err != nil {
		t.Fatal(err)
	}
	row := (fileServer{fsys: fstest.MapFS{}}).emlInboxMessage("oversized.eml", entry[0])
	if row.Warning != "Cannot read message headers." {
		t.Fatalf("unreadable warning = %q", row.Warning)
	}
}

func TestEMLInboxHeaderReaderAcceptsNoBodyAndRejectsInvalid(t *testing.T) {
	for _, tc := range []struct {
		input string
		valid bool
	}{
		{"Subject: Headers only\n", true},
		{"Subject: Final newline absent", true},
		{"Subject: Folded\r\n continuation\r\n\r\n", true},
		{"", false},
		{"not an email\n\n", false},
	} {
		_, err := readEMLInboxHeaders(strings.NewReader(tc.input))
		if (err == nil) != tc.valid {
			t.Errorf("valid=%t for %q, error %v", tc.valid, tc.input, err)
		}
	}
	if _, err := readEMLInboxHeaders(emlInboxErrorReader{}); err == nil {
		t.Fatal("read failure ignored")
	}
}

type emlInboxErrorReader struct{}

func (emlInboxErrorReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestEMLInboxRedirectAndBadParameters(t *testing.T) {
	s := fileServer{fsys: fstest.MapFS{"mail/a.eml": {Data: []byte("Subject: one\n\n")}}}
	w := emlInboxTestResponse(s, "/mail?view=inbox&q=hello", "mail")
	if w.Code != http.StatusMovedPermanently || w.Header().Get("Location") != "/mail/?view=inbox&q=hello" {
		t.Fatalf("redirect: %d %q", w.Code, w.Header().Get("Location"))
	}
	for _, raw := range []string{"page=-1", "page=0", "page=abc", "page=999999999999999999999", "q=" + strings.Repeat("x", 1025)} {
		w := emlInboxTestResponse(s, "/mail/?view=inbox&"+raw, "mail")
		if w.Code != http.StatusBadRequest {
			t.Errorf("query %q: status %d", raw, w.Code)
		}
	}
}
