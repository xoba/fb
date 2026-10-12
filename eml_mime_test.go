package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestEMLMIMEPlainHeadersAndCharsets(t *testing.T) {
	tests := []struct {
		name, charset, encoding, body, want string
	}{
		{"UTF8", "utf-8", "8bit", "Hello, 世界", "Hello, 世界"},
		{"Latin1QP", "ISO-8859-1", "quoted-printable", "caf=E9=20\r\nsecond=\r\nline", "café \r\nsecondline"},
		{"Windows1252Base64", "windows-1252", "base64", base64.StdEncoding.EncodeToString([]byte{0x93, 'H', 'i', 0x94, ' ', 0x80}), "“Hi” €"},
		{"DefaultUTF8", "", "", "café", "café"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			contentType := "text/plain"
			if test.charset != "" {
				contentType += "; charset=" + test.charset
			}
			raw := "From: =?windows-1252?Q?Andr=E9?= <sender@example.com>\r\n" +
				"To: reader@example.com\r\nSubject: =?UTF-8?B?SGVsbG8s?=\r\n\t=?UTF-8?B?IOS4lueVjA==?=\r\n" +
				"X-Trace: first\r\nX-Trace: second\r\nContent-Type: " + contentType + "\r\n" +
				"Content-Transfer-Encoding: " + test.encoding + "\r\n\r\n" + test.body
			message, err := parseEML([]byte(raw))
			if err != nil {
				t.Fatal(err)
			}
			if message.Subject != "Hello, 世界" || message.Plain != test.want || message.HTML != "" || len(message.Parts) != 0 {
				t.Fatalf("unexpected parsed message: %#v", message)
			}
			if message.Root.ID != "1" || message.Root.MediaType != "text/plain" || message.Root.Text != test.want {
				t.Fatalf("incorrect root leaf: %#v", message.Root)
			}
			if len(message.Warnings) != 0 {
				t.Fatalf("unexpected warnings: %v", message.Warnings)
			}
			foundFrom, traces := false, []string{}
			for index, header := range message.Headers {
				if index > 0 && message.Headers[index-1].Name > header.Name {
					t.Fatal("headers are not sorted")
				}
				if header.Name == "From" {
					foundFrom = header.Value == "André <sender@example.com>"
				}
				if header.Name == "X-Trace" {
					traces = append(traces, header.Value)
				}
			}
			if !foundFrom || !reflect.DeepEqual(traces, []string{"first", "second"}) {
				t.Fatalf("headers were lost or not decoded: %#v", message.Headers)
			}
		})
	}
}

func TestEMLMIMENestedAlternativesRelatedAndAttachments(t *testing.T) {
	raw := "From: sender@example.com\r\nSubject: Multipart example\r\nMIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=mixed\r\n\r\npreamble\r\n" +
		"--mixed\r\nContent-Type: multipart/alternative; boundary=alternative\r\n\r\n" +
		"--alternative\r\nContent-Type: text/plain\r\n\r\nOld plain rendition\r\n" +
		"--alternative\r\nContent-Type: text/plain\r\n\r\nPreferred plain rendition\r\n" +
		"--alternative\r\nContent-Type: multipart/related; boundary=related; start=\"<body@example.com>\"\r\n\r\n" +
		"--related\r\nContent-Type: image/png; name=logo.png\r\nContent-ID: <logo@example.com>\r\nContent-Transfer-Encoding: base64\r\n\r\niVBORw==\r\n" +
		"--related\r\nContent-Type: text/html; charset=utf-8\r\nContent-ID: <body@example.com>\r\n\r\n<p>HTML body<img src=\"cid:logo@example.com\"></p>\r\n" +
		"--related--\r\n--alternative--\r\n" +
		"--mixed\r\nContent-Type: application/octet-stream\r\nContent-Disposition: attachment; filename*0*=utf-8''caf%C3%A9; filename*1*=.bin\r\nContent-Transfer-Encoding: base64\r\n\r\nAAECA/8=\r\n" +
		"--mixed\r\nContent-Type: text/plain; charset=iso-8859-1\r\nContent-Disposition: attachment; filename=\"=?UTF-8?Q?notes_=E2=9C=93.txt?=\"\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\ncaf=E9\r\n" +
		"--mixed\r\nContent-Type: message/rfc822\r\n\r\nFrom: forwarded@example.com\r\nSubject: Inner subject\r\n\r\nInner body\r\n" +
		"--mixed--\r\nepilogue"
	message, err := parseEML([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if message.Plain != "Preferred plain rendition" || strings.Contains(message.HTML, "plain rendition") || !strings.Contains(message.HTML, "HTML body") {
		t.Fatalf("incorrect body selection: plain %q, HTML %q", message.Plain, message.HTML)
	}
	if len(message.Parts) != 4 {
		t.Fatalf("got %d attachments, want 4", len(message.Parts))
	}
	wantIDs := []string{"1.1.3.1", "1.2", "1.3", "1.4"}
	wantNames := []string{"logo.png", "café.bin", "notes ✓.txt", "attachment-1.4.eml"}
	for index, part := range message.Parts {
		if part.ID != wantIDs[index] || part.Filename != wantNames[index] {
			t.Fatalf("part %d: %#v", index, part)
		}
	}
	if !message.Parts[0].Inline || message.Parts[0].ContentID != "logo@example.com" || !bytes.Equal(message.Parts[0].Data, []byte{0x89, 'P', 'N', 'G'}) {
		t.Fatalf("incorrect CID image: %#v", message.Parts[0])
	}
	if !bytes.Equal(message.Parts[1].Data, []byte{0, 1, 2, 3, 255}) || message.Parts[1].Inline {
		t.Fatalf("incorrect binary attachment: %#v", message.Parts[1])
	}
	if message.Parts[3].MediaType != "message/rfc822" || !strings.Contains(string(message.Parts[3].Data), "Inner body") {
		t.Fatalf("attached message was lost: %#v", message.Parts[3])
	}
	root := message.Root
	if root.ID != "1" || root.MediaType != "multipart/mixed" || len(root.Children) != 4 {
		t.Fatalf("incorrect MIME tree root: %#v", root)
	}
	if root.BodyPlain != message.Plain || root.BodyHTML != message.HTML || !reflect.DeepEqual(root.Headers, message.Headers) {
		t.Fatal("root node should retain the overview body and headers")
	}
	alternative := root.Children[0]
	if len(alternative.Children) != 3 || alternative.Children[0].Text != "Old plain rendition" || alternative.Children[1].Text != "Preferred plain rendition" {
		t.Fatalf("tree did not preserve alternatives: %#v", alternative)
	}
	if alternative.BodyPlain != "Preferred plain rendition" || !strings.Contains(alternative.BodyHTML, "HTML body") {
		t.Fatalf("alternative container did not retain its readable body: %#v", alternative)
	}
	textAttachment := root.Children[2]
	if textAttachment.Text != "café" || !bytes.Equal(textAttachment.Data, []byte{'c', 'a', 'f', 0xe9}) {
		t.Fatalf("text attachment should retain raw decoded bytes and UTF-8 preview: %#v", textAttachment)
	}
	if textAttachment.BodyPlain != "café" {
		t.Fatal("text attachment should have its own readable body")
	}
	if &textAttachment.Data[0] != &message.Parts[2].Data[0] {
		t.Fatal("tree and attachment list unnecessarily duplicate bytes")
	}
	attachedMessage := root.Children[3]
	if len(attachedMessage.Children) != 1 || attachedMessage.Children[0].ID != "1.4.1" || attachedMessage.Children[0].Text != "Inner body" || !strings.Contains(attachedMessage.Text, "Subject: Inner subject") {
		t.Fatalf("attached email should expose source and a nested MIME tree: %#v", attachedMessage)
	}
	if len(message.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", message.Warnings)
	}
}

func TestEMLMIMEEncodedNestedEmail(t *testing.T) {
	nested := "From: nested@example.com\r\nSubject: Inner\r\nContent-Type: multipart/mixed; boundary=n\r\n\r\n" +
		"--n\r\nContent-Type: text/plain\r\n\r\nInner body\r\n" +
		"--n\r\nContent-Type: text/plain; charset=windows-1252\r\nContent-Disposition: attachment; filename=inner.txt\r\nContent-Transfer-Encoding: base64\r\n\r\nY2Fm6Q==\r\n--n--\r\n"
	raw := "Subject: Outer\r\nContent-Type: multipart/mixed; boundary=m\r\n\r\n" +
		"--m\r\nContent-Type: text/plain\r\n\r\nOuter body\r\n" +
		"--m\r\nContent-Type: message/rfc822\r\nContent-Disposition: attachment; filename=forwarded.eml\r\nContent-Transfer-Encoding: base64\r\n\r\n" + base64.StdEncoding.EncodeToString([]byte(nested)) + "\r\n--m--\r\n"
	message, err := parseEML([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if message.Subject != "Outer" || message.Plain != "Outer body" {
		t.Fatalf("nested email overwrote the outer overview: %#v", message)
	}
	attached := message.Root.Children[1]
	if attached.ID != "1.2" || !bytes.Equal(attached.Data, []byte(nested)) || len(attached.Children) != 1 {
		t.Fatalf("incorrect attached message: %#v", attached)
	}
	inner := attached.Children[0]
	if inner.MediaType != "multipart/mixed" || inner.ID != "1.2.1" || len(inner.Children) != 2 {
		t.Fatalf("incorrect nested container: %#v", inner)
	}
	if inner.Children[0].Text != "Inner body" || inner.Children[1].Text != "café" || inner.Children[1].ID != "1.2.1.2" {
		t.Fatalf("nested bodies were not decoded: %#v", inner.Children)
	}
	if attached.BodyPlain != "Inner body" || inner.BodyPlain != "Inner body" || inner.Children[1].BodyPlain != "café" {
		t.Fatal("attached email and nested nodes should retain independently readable bodies")
	}
	foundSubject := false
	for _, header := range inner.Headers {
		if header.Name == "Subject" && header.Value == "Inner" {
			foundSubject = true
		}
	}
	if !foundSubject {
		t.Fatalf("nested email headers missing: %#v", inner.Headers)
	}
	for _, header := range attached.Headers {
		if header.Name == "Subject" {
			t.Fatal("attached wrapper should retain its MIME headers, not the nested email headers")
		}
	}
}

func TestEMLMIMEMixedPreservesPlainSectionsBesideHTML(t *testing.T) {
	raw := "Subject: Mixed\nContent-Type: multipart/mixed; boundary=m\n\n" +
		"--m\nContent-Type: text/html\n\n<p>Main body</p>\n" +
		"--m\nContent-Type: text/plain\n\nPostscript <script> is plain text.\n--m--\n"
	message, err := parseEML([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(message.HTML, "<p>Main body</p>") || !strings.Contains(message.HTML, "<pre>Postscript &lt;script&gt; is plain text.</pre>") {
		t.Fatalf("plain section is missing or unsafe in combined body: %s", message.HTML)
	}
}

func TestEMLMIMEAttachedMultipartRetainsOwnBody(t *testing.T) {
	raw := "Subject: Containers\nContent-Type: multipart/mixed; boundary=o\n\n" +
		"--o\nContent-Type: text/plain\n\nOuter body\n" +
		"--o\nContent-Type: multipart/alternative; boundary=i\nContent-Disposition: attachment\n\n" +
		"--i\nContent-Type: text/plain\n\nAttached plain body\n" +
		"--i\nContent-Type: text/html\n\n<p>Attached HTML body</p>\n--i--\n--o--\n"
	message, err := parseEML([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if message.Plain != "Outer body" || message.HTML != "" {
		t.Fatal("attached multipart body leaked into overview")
	}
	attached := message.Root.Children[1]
	if attached.BodyPlain != "Attached plain body" || attached.BodyHTML != "<p>Attached HTML body</p>" {
		t.Fatalf("attached multipart lost its own readable body: %#v", attached)
	}
}

func TestEMLMIMEWarnings(t *testing.T) {
	tests := []struct {
		name, raw, text string
	}{
		{"unknownCharset", "Subject: Charset\nContent-Type: text/plain; charset=x-unknown\n\nHello", "Hello"},
		{"invalidUTF8", "Subject: Charset\nContent-Type: text/plain; charset=utf-8\n\nHello \xff", "Hello �"},
		{"unknownHeaderCharset", "Subject: =?x-unknown?Q?Hello?=\n\nBody", "Body"},
		{"malformedHeader", "Subject: =?UTF-8?B?%%%?=\n\nBody", "Body"},
		{"truncatedUTF16", "Subject: Charset\nContent-Type: text/plain; charset=utf-16le\n\na", "�"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			message, err := parseEML([]byte(test.raw))
			if err != nil {
				t.Fatal(err)
			}
			if len(message.Warnings) == 0 || message.Plain != test.text {
				t.Fatalf("expected visible warning and %q body, got %#v", test.text, message)
			}
		})
	}
}

func TestEMLMIMEMalformedInput(t *testing.T) {
	tests := map[string]string{
		"notEmail":             "some arbitrary document\nwith no headers",
		"noHeaders":            "\njust a body",
		"unrelatedHeaders":     "Random: thing\n\nnot an email",
		"badContentType":       "Subject: Broken\nContent-Type: text/plain; charset=\"missing\n\nBody",
		"badMediaType":         "Subject: Broken\nContent-Type: invalid\n\nBody",
		"badDisposition":       "Subject: Broken\nContent-Disposition: attachment; filename=\"missing\n\nBody",
		"badTransfer":          "Subject: Broken\nContent-Transfer-Encoding: x-unknown\n\nBody",
		"badBase64":            "Subject: Broken\nContent-Transfer-Encoding: base64\n\nSGVsbG8=%%%",
		"truncatedBase64":      "Subject: Broken\nContent-Transfer-Encoding: base64\n\nSGVsbG8",
		"badQuotedPrintable":   "Subject: Broken\nContent-Transfer-Encoding: quoted-printable\n\nHello =QZ",
		"truncatedQP":          "Subject: Broken\nContent-Transfer-Encoding: quoted-printable\n\nHello =",
		"truncatedHexQP":       "Subject: Broken\nContent-Transfer-Encoding: quoted-printable\n\nHello =A",
		"badSoftBreakQP":       "Subject: Broken\nContent-Transfer-Encoding: quoted-printable\n\nHello =\rbroken",
		"missingBoundary":      "Subject: Broken\nContent-Type: multipart/mixed\n\nBody",
		"emptyMultipart":       "Subject: Broken\nContent-Type: multipart/mixed; boundary=m\n\n--m--\n",
		"missingBoundaryBody":  "Subject: Broken\nContent-Type: multipart/mixed; boundary=m\n\nNo delimiter",
		"missingClosing":       "Subject: Broken\nContent-Type: multipart/mixed; boundary=m\n\n--m\nContent-Type: text/plain\n\nBody",
		"missingRelatedRoot":   "Subject: Broken\nContent-Type: multipart/related; boundary=m; start=\"<missing@example.com>\"\n\n--m\nContent-Type: text/html\n\n<p>Body</p>\n--m--\n",
		"malformedPartHeaders": "Subject: Broken\nContent-Type: multipart/mixed; boundary=m\n\n--m\nnot a header\n\nBody\n--m--\n",
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			if message, err := parseEML([]byte(raw)); err == nil || message != nil {
				t.Fatalf("malformed email unexpectedly parsed: %#v, %v", message, err)
			}
		})
	}
}

func TestEMLMIMEDigestAndTextResource(t *testing.T) {
	digest := "Subject: Digest\nContent-Type: multipart/digest; boundary=d\n\n--d\n\nFrom: nested@example.com\nSubject: Nested\n\nNested body\n--d--\n"
	message, err := parseEML([]byte(digest))
	if err != nil {
		t.Fatal(err)
	}
	if len(message.Parts) != 1 || message.Parts[0].MediaType != "message/rfc822" || message.Plain != "" {
		t.Fatalf("digest member should be attached message: %#v", message)
	}
	related := "Subject: Related\nContent-Type: multipart/related; boundary=r\n\n--r\nContent-Type: text/html\n\n<p>Body</p>\n--r\nContent-Type: text/plain\nContent-ID: <resource@example.com>\n\nResource content\n--r--\n"
	message, err = parseEML([]byte(related))
	if err != nil {
		t.Fatal(err)
	}
	if message.Plain != "" || len(message.Parts) != 1 || !message.Parts[0].Inline || message.Root.Children[1].Text != "Resource content" {
		t.Fatalf("related resource should not become body: %#v", message)
	}
}

func TestEMLMIMELimits(t *testing.T) {
	t.Run("rawBytes", func(t *testing.T) {
		data := append([]byte("Subject: Large\n\n"), bytes.Repeat([]byte{'a'}, maxEMLBytes)...)
		if _, err := parseEML(data); err == nil || !strings.Contains(err.Error(), "preview limit") {
			t.Fatalf("expected raw size rejection, got %v", err)
		}
	})
	t.Run("decodedBytes", func(t *testing.T) {
		data := append([]byte("Subject: Large\nContent-Type: text/plain; charset=iso-8859-1\n\n"), bytes.Repeat([]byte{0xe9}, maxEMLBytes/2+1)...)
		if _, err := parseEML(data); err == nil || !strings.Contains(err.Error(), "preview limit") {
			t.Fatalf("expected decoded size rejection, got %v", err)
		}
	})
	t.Run("partCount", func(t *testing.T) {
		var raw strings.Builder
		raw.WriteString("Subject: Many parts\nContent-Type: multipart/mixed; boundary=m\n\n")
		for range maxEMLParts {
			raw.WriteString("--m\nContent-Type: text/plain\n\nBody\n")
		}
		raw.WriteString("--m--\n")
		if _, err := parseEML([]byte(raw.String())); err == nil || !strings.Contains(err.Error(), "part limit") {
			t.Fatalf("expected part count rejection, got %v", err)
		}
	})
	t.Run("depth", func(t *testing.T) {
		var raw strings.Builder
		raw.WriteString("Subject: Deep message\n")
		for depth := range maxEMLDepth + 1 {
			fmt.Fprintf(&raw, "Content-Type: multipart/mixed; boundary=b%d\n\n--b%d\n", depth, depth)
		}
		raw.WriteString("Content-Type: text/plain\n\nBody\n")
		for depth := maxEMLDepth; depth >= 0; depth-- {
			fmt.Fprintf(&raw, "--b%d--\n", depth)
		}
		if _, err := parseEML([]byte(raw.String())); err == nil || !strings.Contains(err.Error(), "nesting limit") {
			t.Fatalf("expected depth rejection, got %v", err)
		}
	})
}
