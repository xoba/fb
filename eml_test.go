package main

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"fmt"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"

	nethtml "golang.org/x/net/html"
)

const emlHTTPPixel = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+j53sAAAAASUVORK5CYII="

func emlHTTPFixture() []byte {
	var b strings.Builder
	b.WriteString("From: Example Sender <sender@example.com>\r\nSubject: Mail <unsafe-header>\r\nMIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=parts\r\n\r\n")
	parts := []struct{ header, body string }{
		{"Content-Type: text/plain; charset=utf-8", "Plain <b>alternative</b> & original"},
		{"Content-Type: text/html; charset=utf-8", `<p>HTML <strong>message</strong></p><img src="cid:pixel@example.com"><script>unsafeEmail()</script>`},
		{"Content-Type: image/png\r\nContent-ID: <pixel@example.com>\r\nContent-Transfer-Encoding: base64", emlHTTPPixel},
		{"Content-Type: application/octet-stream\r\nContent-Disposition: attachment; filename=\"../../evil.bin\"\r\nContent-Transfer-Encoding: base64", "AAECf4D/"},
		{"Content-Type: text/html\r\nContent-Disposition: attachment; filename=\"danger.html\"", `<p>Attachment preview</p><script>badAttachment()</script>`},
	}
	for _, part := range parts {
		fmt.Fprintf(&b, "--parts\r\n%s\r\n\r\n%s\r\n", part.header, part.body)
	}
	b.WriteString("--parts--\r\n")
	return []byte(b.String())
}

func emlHTTPRequest(t *testing.T, s fileServer, method, target string, document bool, headers http.Header) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, target, nil)
	if document {
		r.Header.Set("Sec-Fetch-Dest", "document")
	}
	for k, values := range headers {
		r.Header[k] = values
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func emlHTTPDocument(t *testing.T, body string) *nethtml.Node {
	t.Helper()
	doc, err := nethtml.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func emlHTTPWalk(node *nethtml.Node, visit func(*nethtml.Node)) {
	visit(node)
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		emlHTTPWalk(child, visit)
	}
}

func emlHTTPAttr(node *nethtml.Node, name string) string {
	for _, attr := range node.Attr {
		if attr.Key == name {
			return attr.Val
		}
	}
	return ""
}

func emlHTTPElement(t *testing.T, doc *nethtml.Node, id string) *nethtml.Node {
	t.Helper()
	var found *nethtml.Node
	emlHTTPWalk(doc, func(node *nethtml.Node) {
		if emlHTTPAttr(node, "id") == id {
			found = node
		}
	})
	if found == nil {
		t.Fatalf("missing element #%s", id)
	}
	return found
}

func emlHTTPText(node *nethtml.Node) string {
	var text strings.Builder
	emlHTTPWalk(node, func(node *nethtml.Node) {
		if node.Type == nethtml.TextNode {
			text.WriteString(node.Data)
		}
	})
	return text.String()
}

func TestEMLHTTPNavigationAndRaw(t *testing.T) {
	data := emlHTTPFixture()
	s := fileServer{fsys: fstest.MapFS{"message.eml": {Data: data}, "COPY.EML~": {Data: data}}}
	for _, name := range []string{"message.eml", "COPY.EML~"} {
		w := emlHTTPRequest(t, s, "GET", "/"+name, true, nil)
		if w.Code != http.StatusOK || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/html") {
			t.Fatalf("navigation %s: status %d, headers %v", name, w.Code, w.Header())
		}
		doc := emlHTTPDocument(t, w.Body.String())
		body := emlHTTPElement(t, doc, "eml-body")
		if !strings.Contains(emlHTTPText(body), "HTML message") {
			t.Fatal("missing readable HTML body")
		}
		emlHTTPWalk(doc, func(node *nethtml.Node) {
			if node.Data == "unsafe-header" || node.Data == "script" {
				t.Errorf("email content escaped into markup: %s", node.Data)
			}
		})
		if !strings.Contains(w.Header().Get("Content-Security-Policy"), "default-src 'none'") {
			t.Fatal("email response lacks restrictive CSP")
		}
	}
	for _, tc := range []struct {
		query    string
		document bool
	}{
		{"", false},
		{"?eml-view=1.1", false},
		{"?raw=1", true},
		{"?raw=1&eml-part=1.4&inline=1", true},
	} {
		w := emlHTTPRequest(t, s, "GET", "/message.eml"+tc.query, tc.document, nil)
		if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), data) {
			t.Fatalf("raw request %q (document=%v) changed original bytes; status %d", tc.query, tc.document, w.Code)
		}
	}
}

func TestEMLHTTPAttachmentResponses(t *testing.T) {
	s := fileServer{fsys: fstest.MapFS{"message.eml": {Data: emlHTTPFixture()}}}
	w := emlHTTPRequest(t, s, "GET", "/message.eml?eml-part=1.4", false, nil)
	want := []byte{0, 1, 2, 127, 128, 255}
	if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), want) {
		t.Fatalf("decoded attachment: %d %v", w.Code, w.Body.Bytes())
	}
	disposition, params, err := mime.ParseMediaType(w.Header().Get("Content-Disposition"))
	if err != nil || disposition != "attachment" || params["filename"] != "evil.bin" {
		t.Fatalf("unsafe attachment filename: %q (%v)", w.Header().Get("Content-Disposition"), err)
	}
	if w.Header().Get("Content-Type") != "application/octet-stream" || w.Header().Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("attachment safety headers missing: %v", w.Header())
	}
	w = emlHTTPRequest(t, s, "GET", "/message.eml?eml-part=1.4", false, http.Header{"Range": {"bytes=1-3"}})
	if w.Code != http.StatusPartialContent || !bytes.Equal(w.Body.Bytes(), want[1:4]) || w.Header().Get("Content-Range") != "bytes 1-3/6" {
		t.Fatalf("attachment range: %d %v %v", w.Code, w.Body.Bytes(), w.Header())
	}
	w = emlHTTPRequest(t, s, "HEAD", "/message.eml?eml-part=1.4", false, nil)
	if w.Code != http.StatusOK || w.Body.Len() != 0 || w.Header().Get("Content-Length") != "6" {
		t.Fatalf("attachment HEAD: %d %d %v", w.Code, w.Body.Len(), w.Header())
	}
	for _, target := range []string{"?eml-part=missing", "?eml-part=1", "?eml-view=9", "?eml-part=../../private"} {
		w = emlHTTPRequest(t, s, "GET", "/message.eml"+target, true, nil)
		if w.Code != http.StatusNotFound {
			t.Errorf("%s status = %d, want 404", target, w.Code)
		}
	}
	w = emlHTTPRequest(t, s, "GET", "/message.eml?eml-part=1.5&inline=1", false, nil)
	if w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("HTML attachment inline status = %d", w.Code)
	}
	w = emlHTTPRequest(t, s, "GET", "/message.eml?eml-part=1.3&inline=1", false, nil)
	pixel, _ := base64.StdEncoding.DecodeString(emlHTTPPixel)
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "image/png" || !strings.HasPrefix(w.Header().Get("Content-Disposition"), "inline;") || !bytes.Equal(w.Body.Bytes(), pixel) {
		t.Fatalf("inline CID response: status %d, headers %v", w.Code, w.Header())
	}
}

func TestEMLHTTPSelectedPartsAndEscapedSource(t *testing.T) {
	s := fileServer{fsys: fstest.MapFS{"message.eml": {Data: emlHTTPFixture()}}}
	w := emlHTTPRequest(t, s, "GET", "/message.eml?eml-view=1.1", true, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("selected plain part: %d", w.Code)
	}
	doc := emlHTTPDocument(t, w.Body.String())
	body := emlHTTPElement(t, doc, "eml-body")
	if !strings.Contains(emlHTTPText(body), "Plain <b>alternative</b> & original") {
		t.Fatalf("plain part lost original text: %s", emlHTTPText(body))
	}
	emlHTTPWalk(body, func(node *nethtml.Node) {
		if node.Data == "b" {
			t.Error("plain body became HTML")
		}
	})
	w = emlHTTPRequest(t, s, "GET", "/message.eml?eml-view=1.5", true, nil)
	if w.Code != http.StatusFound || w.Header().Get("Location") != "/message.eml/1.5/danger.html" {
		t.Fatalf("HTML attachment should open its ordinary viewer: %d %v", w.Code, w.Header())
	}
	w = emlHTTPRequest(t, s, "GET", w.Header().Get("Location"), true, nil)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Attachment preview") || !strings.Contains(w.Header().Get("Content-Security-Policy"), "sandbox") {
		t.Fatal("HTML attachment lacks the standard sandboxed inline view")
	}
}

func TestEMLHTMLRemovesFetchesAndActiveContent(t *testing.T) {
	pixel, _ := base64.StdEncoding.DecodeString(emlHTTPPixel)
	root := &emlNode{ID: "1", Children: []*emlNode{
		{ID: "1.1", MediaType: "image/png", ContentID: "pixel@example.com", Data: pixel},
		{ID: "1.2", MediaType: "image/svg+xml", ContentID: "svg@example.com", Data: []byte(`<svg onload="evil()"></svg>`)},
		{ID: "1.3", MediaType: "image/png", ContentID: "fake@example.com", Data: []byte(`<html>not a PNG</html>`)},
	}}
	input := `<base href="https://example.com/"><meta http-equiv="refresh" content="0;url=https://example.com/">` +
		`<p id="attack" class="attack" style="background:url(https://example.com/css)">Readable text</p>` +
		`<img src="cid:pixel%40example.com" srcset="https://example.com/track 2x" onerror="evil()" style="display:none">` +
		`<img src="https://example.com/tracker"><img src="//example.com/tracker"><img src="/local.png">` +
		`<img src="file:///private/pixel.png"><img src="data:image/png;base64,` + emlHTTPPixel + `">` +
		`<img src="cid:svg@example.com"><img src="cid:fake@example.com">` +
		`<script>evil()</script><style>@import 'https://example.com/style';</style><iframe src="https://example.com/frame"></iframe>` +
		`<object data="https://example.com/object"></object><video poster="https://example.com/poster"><source src="https://example.com/video"></video>` +
		`<form action="https://example.com/form"><input name="password"><button formaction="https://example.com/button">Submit</button></form>` +
		`<svg><image href="https://example.com/svg"/><a xlink:href="javascript:evil()">SVG link</a></svg>` +
		`<a href="relative.md">relative</a><a href="fb://local/private">internal</a><a href="file:///private">file</a>` +
		`<a href="javascript:evil()">js</a><a href="data:text/html,evil">data</a>` +
		`<a href="https://example.com/message" ping="https://example.com/ping">web</a><a href="mailto:reader@example.com">mail</a>`
	s := fileServer{base: "/folder/archive #é.zip"}
	output := string(s.emlHTML(input, "message & 100%.eml", root))
	doc := emlHTTPDocument(t, output)
	images, links := 0, 0
	emlHTTPWalk(doc, func(node *nethtml.Node) {
		if node.Type != nethtml.ElementNode {
			return
		}
		switch node.Data {
		case "script", "style", "form", "input", "button", "iframe", "object", "video", "source", "svg", "image", "base", "meta":
			t.Errorf("active or fetch-capable element survived: %s", node.Data)
		}
		for _, attr := range node.Attr {
			if strings.HasPrefix(attr.Key, "on") || attr.Key == "style" || attr.Key == "srcset" || attr.Key == "ping" || attr.Key == "id" || attr.Key == "class" {
				t.Errorf("unsafe attribute survived: %s=%q", attr.Key, attr.Val)
			}
		}
		if node.Data == "img" {
			images++
			u, err := url.Parse(emlHTTPAttr(node, "src"))
			if err != nil || u.Scheme != "" || u.Host != "" || u.Path != "/folder/archive #é.zip/message & 100%.eml" || u.Query().Get("eml-part") != "1.1" || u.Query().Get("inline") != "1" {
				t.Errorf("unexpected CID URL: %s (%v)", emlHTTPAttr(node, "src"), err)
			}
		}
		if node.Data == "a" && emlHTTPAttr(node, "href") != "" {
			links++
			u, err := url.Parse(emlHTTPAttr(node, "href"))
			if err != nil || u.Scheme != "https" && u.Scheme != "mailto" {
				t.Errorf("unsafe email link survived: %s", emlHTTPAttr(node, "href"))
			}
		}
	})
	if images != 1 || links != 2 || !strings.Contains(emlHTTPText(doc), "Readable text") {
		t.Fatalf("sanitized content: images=%d links=%d, output=%s", images, links, output)
	}
}

func TestEMLHTTPArchiveAndEncodedPaths(t *testing.T) {
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	name := "café & 100% #1.eml"
	entry, err := writer.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = entry.Write(emlHTTPFixture()); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	s := fileServer{fsys: fstest.MapFS{"mail #1.zip": {Data: archive.Bytes()}}}
	target := (&url.URL{Path: "/mail #1.zip/" + name}).String()
	w := emlHTTPRequest(t, s, "GET", target, true, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("archive message: %d %s", w.Code, w.Body.String())
	}
	doc := emlHTTPDocument(t, w.Body.String())
	var imageURL string
	emlHTTPWalk(emlHTTPElement(t, doc, "eml-body"), func(node *nethtml.Node) {
		if node.Data == "img" {
			imageURL = emlHTTPAttr(node, "src")
		}
	})
	if imageURL == "" {
		t.Fatal("archive email lost embedded CID URL")
	}
	w = emlHTTPRequest(t, s, "GET", imageURL, false, nil)
	pixel, _ := base64.StdEncoding.DecodeString(emlHTTPPixel)
	if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), pixel) {
		t.Fatalf("archive CID URL failed: %q status=%d", imageURL, w.Code)
	}
}

func TestEMLHTMLCIDScopesExcludeAttachedMessages(t *testing.T) {
	pixel, _ := base64.StdEncoding.DecodeString(emlHTTPPixel)
	innerImage := &emlNode{ID: "1.2.1.2", MediaType: "image/png", ContentID: "same@example.com", Data: pixel}
	inner := &emlNode{ID: "1.2.1", MediaType: "multipart/related", Children: []*emlNode{
		{ID: "1.2.1.1", MediaType: "text/html"}, innerImage,
	}}
	attached := &emlNode{ID: "1.2", MediaType: "message/rfc822", Children: []*emlNode{inner}}
	outerImage := &emlNode{ID: "1.3", MediaType: "image/png", ContentID: "same@example.com", Data: pixel}
	root := &emlNode{ID: "1", MediaType: "multipart/mixed", Children: []*emlNode{
		{ID: "1.1", MediaType: "text/html"}, attached, outerImage,
	}}
	for _, tc := range []struct {
		selected, wantPart string
		wantScope          *emlNode
	}{
		{"1", "1.3", root},
		{"1.1", "1.3", root},
		{"1.2", "1.2.1.2", attached},
		{"1.2.1", "1.2.1.2", inner},
		{"1.2.1.1", "1.2.1.2", inner},
	} {
		t.Run(tc.selected, func(t *testing.T) {
			scope := emlCIDScope(root, tc.selected)
			if scope != tc.wantScope {
				t.Fatalf("CID scope = %v, want %s", scope, tc.wantScope.ID)
			}
			body := string((fileServer{}).emlHTML(`<img src="cid:same@example.com">`, "message.eml", scope))
			var selected string
			emlHTTPWalk(emlHTTPDocument(t, body), func(node *nethtml.Node) {
				if node.Data == "img" {
					u, err := url.Parse(emlHTTPAttr(node, "src"))
					if err != nil {
						t.Error(err)
						return
					}
					selected = u.Query().Get("eml-part")
				}
			})
			if selected != tc.wantPart {
				t.Fatalf("CID selected %q, want %q", selected, tc.wantPart)
			}
		})
	}
}

func TestEMLHTTPSelectedMultipartAndAttachedEmail(t *testing.T) {
	data := []byte("Subject: Outer email\r\nMIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=outer\r\n\r\n" +
		"--outer\r\nContent-Type: multipart/alternative; boundary=alternative\r\n\r\n" +
		"--alternative\r\nContent-Type: text/plain\r\n\r\nPlain alternative body\r\n" +
		"--alternative\r\nContent-Type: text/html\r\n\r\n<p>Multipart <strong>readable body</strong></p>\r\n" +
		"--alternative--\r\n--outer\r\nContent-Type: message/rfc822\r\nContent-Disposition: attachment; filename=nested.eml\r\n\r\n" +
		string(emlHTTPFixture()) + "\r\n--outer--\r\n")
	s := fileServer{fsys: fstest.MapFS{"message.eml": {Data: data}}}
	for _, tc := range []struct{ part, text string }{
		{"1.1", "Multipart readable body"},
		{"1.2", "HTML message"},
	} {
		w := emlHTTPRequest(t, s, "GET", "/message.eml?eml-view="+tc.part, true, nil)
		if tc.part == "1.2" {
			if w.Code != http.StatusFound || w.Header().Get("Location") != "/message.eml/1.2/nested.eml" {
				t.Fatalf("forwarded message should open its ordinary viewer: %d %v", w.Code, w.Header())
			}
			w = emlHTTPRequest(t, s, "GET", w.Header().Get("Location"), true, nil)
		}
		if w.Code != http.StatusOK {
			t.Fatalf("part %s response: %d %s", tc.part, w.Code, w.Body.String())
		}
		doc := emlHTTPDocument(t, w.Body.String())
		if !strings.Contains(emlHTTPText(emlHTTPElement(t, doc, "eml-body")), tc.text) {
			t.Fatalf("part %s lacks readable aggregate body: %s", tc.part, w.Body.String())
		}
	}
	w := emlHTTPRequest(t, s, "GET", "/message.eml?eml-part=1.2", false, nil)
	if w.Code != http.StatusOK || !bytes.Equal(bytes.TrimSpace(w.Body.Bytes()), bytes.TrimSpace(emlHTTPFixture())) {
		t.Fatalf("attached original email changed: %d %s", w.Code, w.Body.String())
	}
}

func TestEMLHTTPMalformedAndOversizedFallback(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"malformed", []byte("not email\n<script>badSource()</script>\n")},
		{"oversized", make([]byte, maxEMLBytes+1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := fileServer{fsys: fstest.MapFS{"message.eml": {Data: tc.data}}}
			w := emlHTTPRequest(t, s, "GET", "/message.eml", true, nil)
			if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Cannot preview this email:") || !strings.Contains(w.Body.String(), "raw=1") {
				t.Fatalf("fallback is not visible: %d %s", w.Code, w.Body.String())
			}
			doc := emlHTTPDocument(t, w.Body.String())
			emlHTTPWalk(doc, func(node *nethtml.Node) {
				if node.Data == "script" {
					t.Error("malformed source was executed")
				}
			})
			if tc.name == "malformed" && !strings.Contains(emlHTTPText(doc), "<script>badSource()</script>") {
				t.Fatal("malformed original source missing")
			}
		})
	}
}

func TestEMLHTTPDirectoryInboxLink(t *testing.T) {
	s := fileServer{fsys: fstest.MapFS{"message.eml": {Data: emlHTTPFixture()}, "notes.txt": {Data: []byte("notes")}}}
	w := emlHTTPRequest(t, s, "GET", "/", true, nil)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "view=inbox") {
		t.Fatalf("directory lacks inbox navigation: %d %s", w.Code, w.Body.String())
	}
	w = emlHTTPRequest(t, s, "GET", "/?view=inbox", true, nil)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "sender@example.com") || !strings.Contains(w.Body.String(), "Mail &lt;unsafe-header&gt;") {
		t.Fatalf("inbox route failed: %d %s", w.Code, w.Body.String())
	}
}
