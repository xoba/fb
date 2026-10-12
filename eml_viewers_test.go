package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

type emlViewerAttachment struct {
	name      string
	mediaType string
	data      []byte
}

// Every message is generated locally, with a readable body followed by the
// supplied attachments. Thus the first attachment is MIME part 1.2.
func emlViewerFixture(t *testing.T, attachments ...emlViewerAttachment) []byte {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	parts := append([]emlViewerAttachment{{mediaType: "text/plain", data: []byte("A synthetic attachment gallery.")}}, attachments...)
	for i, part := range parts {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Type", part.mediaType)
		header.Set("Content-Transfer-Encoding", "base64")
		if i > 0 {
			params := map[string]string{}
			if part.name != "" {
				params["filename"] = part.name
			}
			header.Set("Content-Disposition", mime.FormatMediaType("attachment", params))
		}
		p, err := w.CreatePart(header)
		if err != nil {
			t.Fatal(err)
		}
		encoded := base64.StdEncoding.EncodeToString(part.data)
		for len(encoded) > 0 {
			n := min(76, len(encoded))
			if _, err := fmt.Fprintf(p, "%s\r\n", encoded[:n]); err != nil {
				t.Fatal(err)
			}
			encoded = encoded[n:]
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return append([]byte("From: Example Sender <sender@example.com>\r\nSubject: Attachment gallery\r\nMIME-Version: 1.0\r\nContent-Type: "+mime.FormatMediaType("multipart/mixed", map[string]string{"boundary": w.Boundary()})+"\r\n\r\n"), body.Bytes()...)
}

func emlViewerExample(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("examples/formats/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func emlViewerZip(t *testing.T, name string, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	member, err := w.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := member.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func emlViewerCheckHTML(t *testing.T, w *httptest.ResponseRecorder, contains ...string) {
	t.Helper()
	if w.Code != http.StatusOK || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("viewer: status %d, headers %v, body %.200s", w.Code, w.Header(), w.Body.String())
	}
	for _, want := range contains {
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("viewer is missing %q", want)
		}
	}
	if strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("viewer unexpectedly forced a download: %v", w.Header())
	}
}

func TestEMLPartsUseNormalTextAndTableViewers(t *testing.T) {
	for _, tc := range []struct {
		attachment emlViewerAttachment
		contains   []string
	}{
		{emlViewerAttachment{"cities.csv", "text/csv", []byte("city,population\nPortland,652503\n")}, []string{"<th>city</th><th>population</th>", "<td>Portland</td><td>652503</td>", `name="q"`}},
		{emlViewerAttachment{"cities.tsv", "text/tab-separated-values", []byte("city\tpopulation\nPortland\t652503\n")}, []string{"<th>city</th><th>population</th>", "<td>Portland</td><td>652503</td>"}},
		{emlViewerAttachment{"catalog.json", "application/json", []byte(`{"city":"Portland","population":652503}`)}, []string{`id="L1"`, "Portland", "652503"}},
		{emlViewerAttachment{"hello.go", "application/octet-stream", []byte("package main\n\nfunc main() {}\n")}, []string{`id="L1"`, `id="L3"`, "package"}},
	} {
		t.Run(tc.attachment.name, func(t *testing.T) {
			original := emlViewerFixture(t, tc.attachment)
			s := fileServer{fsys: fstest.MapFS{"message.eml": {Data: original}}}
			target := "/message.eml/1.2/" + tc.attachment.name
			message := emlHTTPRequest(t, s, "GET", "/message.eml", true, nil)
			emlViewerCheckHTML(t, message, `href="`+target+`"`, `href="`+target+`?raw=1" download`)
			emlViewerCheckHTML(t, emlHTTPRequest(t, s, "GET", target, true, nil), tc.contains...)
			for _, rawTarget := range []string{target + "?raw=1", "/message.eml?eml-part=1.2"} {
				w := emlHTTPRequest(t, s, "GET", rawTarget, false, nil)
				if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), tc.attachment.data) {
					t.Fatalf("decoded original at %s: status %d, bytes %q", rawTarget, w.Code, w.Body.Bytes())
				}
			}
			w := emlHTTPRequest(t, s, "GET", "/message.eml?raw=1", true, nil)
			if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), original) {
				t.Fatal("browsing attachments changed the original message")
			}
		})
	}
}

func TestEMLTextPartViewersHonorCharsetWithoutChangingDownloads(t *testing.T) {
	for _, tc := range []struct {
		name, mediaType, encoded, preview string
		table                             bool
	}{
		{"menu.csv", "text/csv; charset=iso-8859-1", "drink,price\ncaf\xe9,3\n", "<td>café</td><td>3</td>", true},
		{"menu.txt", "text/plain; charset=iso-8859-1", "A caf\xe9 menu.\n", "A café menu.\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := []byte(tc.encoded)
			s := fileServer{fsys: fstest.MapFS{"message.eml": {Data: emlViewerFixture(t, emlViewerAttachment{tc.name, tc.mediaType, original})}}}
			target := "/message.eml/1.2/" + tc.name
			w := emlHTTPRequest(t, s, "GET", target, true, nil)
			if tc.table {
				emlViewerCheckHTML(t, w, tc.preview)
			} else if w.Code != http.StatusOK || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/plain") || w.Body.String() != tc.preview {
				t.Fatalf("charset-aware text preview: status %d, headers %v, body %q", w.Code, w.Header(), w.Body.String())
			}
			for _, request := range []struct {
				target   string
				document bool
			}{
				{target + "?raw=1", true},
				{"/message.eml?eml-part=1.2", false},
				{target, false},
			} {
				w = emlHTTPRequest(t, s, "GET", request.target, request.document, nil)
				if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), original) {
					t.Errorf("download %s changed the Latin-1 original: status %d, bytes %q", request.target, w.Code, w.Body.Bytes())
				}
			}
		})
	}
}

func TestEMLPartPDFSupportsInlineHeadAndRange(t *testing.T) {
	pdf := emlViewerExample(t, "sample.pdf")
	s := fileServer{fsys: fstest.MapFS{"message.eml": {Data: emlViewerFixture(t, emlViewerAttachment{"report.pdf", "application/pdf", pdf})}}}
	const target = "/message.eml/1.2/report.pdf"
	for _, query := range []string{"", "?raw=1"} {
		w := emlHTTPRequest(t, s, "GET", target+query, true, nil)
		if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "application/pdf" || !bytes.Equal(w.Body.Bytes(), pdf) {
			t.Fatalf("inline PDF%s: status %d, headers %v, length %d", query, w.Code, w.Header(), w.Body.Len())
		}
		if strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment") {
			t.Fatal("PDF navigation unexpectedly forced a download")
		}
	}
	w := emlHTTPRequest(t, s, "HEAD", target, false, nil)
	if w.Code != http.StatusOK || w.Body.Len() != 0 || w.Header().Get("Content-Length") != strconv.Itoa(len(pdf)) {
		t.Fatalf("PDF HEAD: status %d, headers %v, length %d", w.Code, w.Header(), w.Body.Len())
	}
	w = emlHTTPRequest(t, s, "GET", target, false, http.Header{"Range": {"bytes=0-15"}})
	if w.Code != http.StatusPartialContent || !bytes.Equal(w.Body.Bytes(), pdf[:16]) || w.Header().Get("Content-Range") != fmt.Sprintf("bytes 0-15/%d", len(pdf)) {
		t.Fatalf("PDF range: status %d, headers %v, bytes %q", w.Code, w.Header(), w.Body.Bytes())
	}
}

func TestEMLPartImageViewerAndSubresources(t *testing.T) {
	png := emlViewerExample(t, "color-grid.png")
	s := fileServer{fsys: fstest.MapFS{"message.eml": {Data: emlViewerFixture(t, emlViewerAttachment{"picture.png", "image/png", png})}}}
	const target = "/message.eml/1.2/picture.png"
	emlViewerCheckHTML(t, emlHTTPRequest(t, s, "GET", target, true, nil), `<img class="subject" src="picture.png?raw=1"`, "image/png")
	for _, query := range []string{"", "?raw=1"} {
		w := emlHTTPRequest(t, s, "GET", target+query, false, http.Header{"Sec-Fetch-Dest": {"image"}})
		if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "image/png" || !bytes.Equal(w.Body.Bytes(), png) {
			t.Fatalf("image subresource%s: status %d, headers %v, bytes %d", query, w.Code, w.Header(), w.Body.Len())
		}
	}
}

func TestEMLPartsBrowseDatabasesAndSpreadsheets(t *testing.T) {
	for _, tc := range []struct {
		name, mediaType, member string
	}{
		{"library.sqlite", "application/vnd.sqlite3", "books"},
		{"library.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "Books"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := emlViewerExample(t, tc.name)
			s := fileServer{fsys: fstest.MapFS{"message.eml": {Data: emlViewerFixture(t, emlViewerAttachment{tc.name, tc.mediaType, data})}}}
			base := "/message.eml/1.2/" + tc.name
			w := emlHTTPRequest(t, s, "GET", base, true, nil)
			if w.Code != http.StatusMovedPermanently || w.Header().Get("Location") != base+"/" {
				t.Fatalf("container navigation: status %d, location %q", w.Code, w.Header().Get("Location"))
			}
			emlViewerCheckHTML(t, emlHTTPRequest(t, s, "GET", base+"/", true, nil), `href="`+tc.member+`"`)
			emlViewerCheckHTML(t, emlHTTPRequest(t, s, "GET", base+"/"+tc.member, true, nil), "<th>title</th>", "<td>A Journey Through Files</td>")
			w = emlHTTPRequest(t, s, "GET", base+"/"+tc.member+"?raw=1", false, nil)
			if w.Code != http.StatusOK || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/csv") || !strings.Contains(w.Body.String(), "A Journey Through Files") {
				t.Fatalf("container member export: status %d, headers %v, body %.200s", w.Code, w.Header(), w.Body.String())
			}
			w = emlHTTPRequest(t, s, "GET", base+"?raw=1", false, nil)
			if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), data) {
				t.Fatal("container's raw response did not preserve decoded attachment")
			}
		})
	}
}

func TestEMLPartsBrowseNestedArchivesAndMessages(t *testing.T) {
	csv := []byte("city,population\nPortland,652503\n")
	inner := emlViewerFixture(t, emlViewerAttachment{"cities.csv", "text/csv", csv})
	zipData := emlViewerZip(t, "inside.eml", inner)
	original := emlViewerFixture(t, emlViewerAttachment{"files.zip", "application/zip", zipData}, emlViewerAttachment{"forwarded.eml", "message/rfc822", inner})
	s := fileServer{fsys: fstest.MapFS{
		"message.eml": {Data: original},
		"outer.zip":   {Data: emlViewerZip(t, "message.eml", original)},
	}}
	for _, base := range []string{"/message.eml", "/outer.zip/message.eml"} {
		emlViewerCheckHTML(t, emlHTTPRequest(t, s, "GET", base+"/1.2/files.zip/", true, nil), `href="inside.eml"`)
		for _, target := range []string{
			base + "/1.2/files.zip/inside.eml/1.2/cities.csv",
			base + "/1.3/forwarded.eml/1.2/cities.csv",
		} {
			t.Run(target, func(t *testing.T) {
				emlViewerCheckHTML(t, emlHTTPRequest(t, s, "GET", target, true, nil), "<th>city</th>", "<td>Portland</td>")
				w := emlHTTPRequest(t, s, "GET", target+"?raw=1", false, nil)
				if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), csv) {
					t.Fatalf("nested raw attachment at %s: status %d, body %q", target, w.Code, w.Body.String())
				}
			})
		}
	}
}

func TestEMLPartFilenamesAreSafeDistinctAndInferred(t *testing.T) {
	pdf := emlViewerExample(t, "sample.pdf")
	attachments := []emlViewerAttachment{
		{"../../same.csv", "text/csv", []byte("value\nfirst\n")},
		{`C:\\folder\\same.csv`, "text/csv", []byte("value\nsecond\n")},
		{"", "application/pdf", pdf},
		{"café # 100%.csv", "text/csv", []byte("value\nthird\n")},
	}
	s := fileServer{fsys: fstest.MapFS{"message.eml": {Data: emlViewerFixture(t, attachments...)}, "private.csv": {Data: []byte("private")}}}
	for i, filename := range []string{"same.csv", "same.csv", "attachment-1.4.pdf", "café # 100%.csv"} {
		id := fmt.Sprintf("1.%d", i+2)
		target := (&url.URL{Path: "/message.eml/" + id + "/" + filename, RawQuery: "raw=1"}).String()
		w := emlHTTPRequest(t, s, "GET", target, false, nil)
		if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), attachments[i].data) {
			t.Errorf("part %s, filename %q: status %d, body %.100s", id, filename, w.Code, w.Body.String())
		}
	}
	for _, target := range []string{
		"/message.eml/1.999/private.csv",
		"/message.eml/1.2/private.csv",
		"/message.eml/1.2/same.csv/private.csv",
	} {
		w := emlHTTPRequest(t, s, "GET", target, true, nil)
		if w.Code != http.StatusNotFound {
			t.Errorf("nonexistent part path %s: status %d, want 404", target, w.Code)
		}
	}
}

func TestEMLUnnamedMediaUseSupportedExtensions(t *testing.T) {
	// System MIME registries can prefer .jpe or .f4a, neither of which selects
	// fb's image/audio viewer. Generated names must use the supported suffix.
	for _, tc := range []struct {
		example, mediaType, filename, tag string
	}{
		{"color-grid.jpg", "image/jpeg", "attachment-1.2.jpg", `<img class="subject"`},
		{"quiet-tone.m4a", "audio/mp4", "attachment-1.2.m4a", "<audio"},
	} {
		t.Run(tc.mediaType, func(t *testing.T) {
			data := emlViewerExample(t, tc.example)
			s := fileServer{fsys: fstest.MapFS{"message.eml": {Data: emlViewerFixture(t, emlViewerAttachment{mediaType: tc.mediaType, data: data})}}}
			target := "/message.eml/1.2/" + tc.filename
			emlViewerCheckHTML(t, emlHTTPRequest(t, s, "GET", "/message.eml", true, nil), `href="`+target+`"`)
			emlViewerCheckHTML(t, emlHTTPRequest(t, s, "GET", target, true, nil), tc.tag)
			w := emlHTTPRequest(t, s, "GET", target+"?raw=1", false, nil)
			if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), data) {
				t.Fatalf("unnamed %s raw response: status %d, bytes %d", tc.mediaType, w.Code, w.Body.Len())
			}
		})
	}
}

func TestEMLPartViewerReleasesParsingSlotBeforeDispatch(t *testing.T) {
	// Leave exactly one heavy-work slot available. The CSV SQL viewer needs
	// that slot after MIME parsing, including through a nested message.
	for i := 0; i < heavyWorkSlots-1; i++ {
		release, ok := tryAcquireHeavy()
		if !ok {
			t.Fatal("cannot reserve a heavy-work slot")
		}
		defer release()
	}
	csv := []byte("city,population\nPortland,652503\n")
	inner := emlViewerFixture(t, emlViewerAttachment{"cities.csv", "text/csv", csv})
	outer := emlViewerFixture(t, emlViewerAttachment{"forwarded.eml", "message/rfc822", inner})
	s := fileServer{fsys: fstest.MapFS{"message.eml": {Data: outer}}}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	r := httptest.NewRequest("GET", "/message.eml/1.2/forwarded.eml/1.2/cities.csv?q="+url.QueryEscape("select city from t"), nil).WithContext(ctx)
	r.Header.Set("Sec-Fetch-Dest", "document")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if ctx.Err() != nil {
		t.Fatalf("nested MIME viewer exhausted the heavy-work slots: %v", ctx.Err())
	}
	emlViewerCheckHTML(t, w, "<td>Portland</td>")
	if strings.Contains(w.Body.String(), `class="queryerror"`) {
		t.Fatal("CSV query could not acquire its heavy-work slot")
	}
}
