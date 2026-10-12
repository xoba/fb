package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html/charset"
)

const (
	maxEMLBytes = 32 << 20
	maxEMLDepth = 16
	maxEMLParts = 256
)

type emlHeader struct {
	Name, Value string
}

type emlPart struct {
	ID, Filename, MediaType, ContentID string
	Data                               []byte
	Inline                             bool
}

type emlMessage struct {
	Root     *emlNode
	Headers  []emlHeader
	Subject  string
	Plain    string
	HTML     string
	Parts    []emlPart
	Warnings []string
}

type emlNode struct {
	Headers                            []emlHeader
	BodyPlain, BodyHTML                string
	ID, MediaType, Filename, ContentID string
	Children                           []*emlNode
	Data                               []byte
	Text                               string
	Inline                             bool
}

type emlBody struct {
	plain, html string
}

type emlParser struct {
	message      emlMessage
	partCount    int
	decodedBytes int64
}

// parseEML parses one RFC 5322 message without following links or extracting any
// attachments to disk. Transfer decoding is explicit, including for multipart
// children, so malformed data cannot silently turn into a successful preview.
func parseEML(data []byte) (*emlMessage, error) {
	if len(data) > maxEMLBytes {
		return nil, fmt.Errorf("email exceeds the %d MiB preview limit", maxEMLBytes>>20)
	}
	message, err := mail.ReadMessage(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("read email: %w", err)
	}
	if !hasEMLHeaders(message.Header) {
		return nil, fmt.Errorf("file has no recognizable email headers")
	}
	p := &emlParser{}
	body, err := p.parsePart(textproto.MIMEHeader(message.Header), message.Body, "1", 0, true, "text/plain", nil)
	if err != nil {
		return nil, err
	}
	p.message.Headers = p.message.Root.Headers
	for _, header := range p.message.Headers {
		if header.Name == "Subject" {
			p.message.Subject = header.Value
			break
		}
	}
	p.message.Plain, p.message.HTML = body.plain, body.html
	return &p.message, nil
}

func hasEMLHeaders(header mail.Header) bool {
	for _, name := range []string{"From", "To", "Cc", "Bcc", "Subject", "Date", "Sender", "Reply-To", "Message-ID", "Received", "Return-Path", "MIME-Version", "Content-Type", "Content-Transfer-Encoding"} {
		if _, ok := header[textproto.CanonicalMIMEHeaderKey(name)]; ok {
			return true
		}
	}
	return false
}

func (p *emlParser) decodeHeaders(header textproto.MIMEHeader) []emlHeader {
	names := make([]string, 0, len(header))
	for name := range header {
		names = append(names, name)
	}
	sort.Strings(names)
	var decoded []emlHeader
	for _, name := range names {
		for _, value := range header[name] {
			decoded = append(decoded, emlHeader{
				Name: textproto.CanonicalMIMEHeaderKey(name), Value: p.decodeHeader(value),
			})
		}
	}
	return decoded
}

func (p *emlParser) warn(message string) {
	for _, previous := range p.message.Warnings {
		if previous == message {
			return
		}
	}
	p.message.Warnings = append(p.message.Warnings, message)
}

func (p *emlParser) decodeHeader(value string) string {
	decoder := &mime.WordDecoder{CharsetReader: func(label string, input io.Reader) (io.Reader, error) {
		reader, err := charset.NewReaderLabel(label, input)
		if err != nil {
			p.warn(fmt.Sprintf("Unknown header charset %q; displayed as UTF-8 where possible.", label))
			return input, nil
		}
		return reader, nil
	}}
	decoded, err := decoder.DecodeHeader(value)
	if err != nil {
		p.warn("An encoded email header could not be decoded; its original value is shown.")
		decoded = value
	} else if strings.Contains(value, "=?") && value == decoded {
		p.warn("An encoded email header could not be decoded; its original value is shown.")
	}
	if !utf8.ValidString(decoded) {
		p.warn("An email header contains invalid text bytes; replacement characters are shown.")
		decoded = strings.ToValidUTF8(decoded, "\uFFFD")
	}
	return decoded
}

func (p *emlParser) parsePart(header textproto.MIMEHeader, input io.Reader, id string, depth int, allowBody bool, defaultType string, parent *emlNode) (body emlBody, err error) {
	if depth > maxEMLDepth {
		return emlBody{}, fmt.Errorf("email exceeds the MIME nesting limit (%d)", maxEMLDepth)
	}
	p.partCount++
	if p.partCount > maxEMLParts {
		return emlBody{}, fmt.Errorf("email exceeds the MIME part limit (%d)", maxEMLParts)
	}
	mediaType, params := defaultType, map[string]string{}
	if contentType := header.Get("Content-Type"); contentType != "" {
		var err error
		mediaType, params, err = mime.ParseMediaType(contentType)
		if err != nil {
			return emlBody{}, fmt.Errorf("email part %s has an invalid Content-Type: %w", id, err)
		}
		if strings.Count(mediaType, "/") != 1 {
			return emlBody{}, fmt.Errorf("email part %s has an invalid MIME media type", id)
		}
	}
	disposition, dispositionParams := "", map[string]string{}
	if raw := header.Get("Content-Disposition"); raw != "" {
		var err error
		disposition, dispositionParams, err = mime.ParseMediaType(raw)
		if err != nil {
			return emlBody{}, fmt.Errorf("email part %s has an invalid Content-Disposition: %w", id, err)
		}
	}
	filename := dispositionParams["filename"]
	if filename == "" {
		filename = params["name"]
	}
	if filename == "" && (strings.Contains(strings.ToLower(header.Get("Content-Disposition")), "filename*") || strings.Contains(strings.ToLower(header.Get("Content-Type")), "name*")) {
		p.warn(fmt.Sprintf("An encoded filename in email part %s could not be decoded; a generated filename is shown.", id))
	}
	filename = p.decodeHeader(filename)
	contentID := emlContentID(header.Get("Content-ID"))
	node := &emlNode{
		ID: id, MediaType: mediaType, Filename: filename, ContentID: contentID,
		Headers: p.decodeHeaders(header), Inline: disposition != "attachment" && (disposition == "inline" || contentID != ""),
	}
	defer func() {
		if body.plain != "" {
			node.BodyPlain = body.plain
		}
		if body.html != "" {
			node.BodyHTML = body.html
		}
	}()
	if parent == nil {
		p.message.Root = node
	} else {
		parent.Children = append(parent.Children, node)
	}
	if disposition == "attachment" || filename != "" {
		allowBody = false
	}
	decoded, err := emlTransferReader(header.Get("Content-Transfer-Encoding"), input)
	if err != nil {
		return emlBody{}, fmt.Errorf("email part %s: %w", id, err)
	}
	if strings.HasPrefix(mediaType, "multipart/") {
		boundary := params["boundary"]
		if boundary == "" {
			return emlBody{}, fmt.Errorf("email part %s has no multipart boundary", id)
		}
		reader := multipart.NewReader(decoded, boundary)
		var bodies []emlBody
		start := emlContentID(params["start"])
		foundRoot := false
		childDefault := "text/plain"
		if mediaType == "multipart/digest" {
			childDefault = "message/rfc822"
		}
		for index := 1; ; index++ {
			part, err := reader.NextRawPart()
			if err == io.EOF {
				if index == 1 {
					return emlBody{}, fmt.Errorf("email part %s is an empty multipart message", id)
				}
				break
			}
			if err != nil {
				return emlBody{}, fmt.Errorf("read email multipart %s: %w", id, err)
			}
			// A container keeps its own readable body even when it is itself an
			// attachment or related resource excluded from its parent's overview.
			childBody := true
			if mediaType == "multipart/related" {
				isRoot := index == 1
				if start != "" {
					isRoot = emlContentID(part.Header.Get("Content-ID")) == start
				}
				childBody = isRoot
				foundRoot = foundRoot || isRoot
			}
			body, err := p.parsePart(part.Header, part, id+"."+strconv.Itoa(index), depth+1, childBody, childDefault, node)
			if err != nil {
				return emlBody{}, err
			}
			bodies = append(bodies, body)
		}
		if mediaType == "multipart/related" && !foundRoot {
			return emlBody{}, fmt.Errorf("email part %s is missing its multipart/related root", id)
		}
		combined := emlCombineBodies(bodies, mediaType == "multipart/alternative")
		if len(combined.plain)+len(combined.html) > maxEMLBytes {
			return emlBody{}, fmt.Errorf("rendered email exceeds the %d MiB preview limit", maxEMLBytes>>20)
		}
		node.BodyPlain, node.BodyHTML = combined.plain, combined.html
		if !allowBody {
			return emlBody{}, nil
		}
		return combined, nil
	}
	data, err := io.ReadAll(io.LimitReader(decoded, maxEMLBytes-p.decodedBytes+1))
	if err != nil {
		return emlBody{}, fmt.Errorf("decode email part %s: %w", id, err)
	}
	if int64(len(data))+p.decodedBytes > maxEMLBytes {
		return emlBody{}, fmt.Errorf("decoded email exceeds the %d MiB preview limit", maxEMLBytes>>20)
	}
	node.Data = data
	if strings.HasPrefix(mediaType, "text/") {
		text, err := p.decodeText(data, params["charset"])
		if err != nil {
			return emlBody{}, fmt.Errorf("decode email text part %s: %w", id, err)
		}
		p.decodedBytes += int64(len(text))
		if p.decodedBytes > maxEMLBytes {
			return emlBody{}, fmt.Errorf("decoded email exceeds the %d MiB preview limit", maxEMLBytes>>20)
		}
		node.Text = text
		if mediaType == "text/plain" {
			node.BodyPlain = text
		} else if mediaType == "text/html" {
			node.BodyHTML = text
		}
		if allowBody {
			if mediaType == "text/html" {
				return emlBody{html: text}, nil
			}
			if mediaType == "text/plain" {
				return emlBody{plain: text}, nil
			}
		}
	} else {
		p.decodedBytes += int64(len(data))
	}
	if mediaType == "message/rfc822" || mediaType == "message/global" {
		// Keep the original message downloadable while exposing its MIME body as
		// a child. Nested messages share every limit with the enclosing email.
		node.Text = strings.ToValidUTF8(string(data), "\uFFFD")
		nested, err := mail.ReadMessage(bytes.NewReader(data))
		if err != nil || !hasEMLHeaders(nested.Header) {
			p.warn(fmt.Sprintf("Attached email %s has invalid headers; its original bytes are available without a nested preview.", id))
		} else if nestedBody, err := p.parsePart(textproto.MIMEHeader(nested.Header), nested.Body, id+".1", depth+1, true, "text/plain", node); err != nil {
			return emlBody{}, err
		} else {
			node.BodyPlain, node.BodyHTML = nestedBody.plain, nestedBody.html
		}
	}
	if filename == "" {
		filename = "attachment-" + id
		if mediaType == "message/rfc822" || mediaType == "message/global" {
			filename += ".eml"
		} else if extensions, err := mime.ExtensionsByType(mediaType); err == nil && len(extensions) > 0 {
			sort.Strings(extensions)
			filename += extensions[0]
		}
	}
	node.Filename = filename
	p.message.Parts = append(p.message.Parts, emlPart{
		ID: id, Filename: filename, MediaType: mediaType, ContentID: contentID,
		Data: data, Inline: node.Inline,
	})
	return emlBody{}, nil
}

func emlTransferReader(encoding string, input io.Reader) (io.Reader, error) {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "", "7bit", "8bit", "binary":
		return input, nil
	case "base64":
		return base64.NewDecoder(base64.StdEncoding, input), nil
	case "quoted-printable":
		return quotedprintable.NewReader(&emlQuotedPrintableValidator{input: input}), nil
	default:
		return nil, fmt.Errorf("unsupported Content-Transfer-Encoding %q", encoding)
	}
}

// The standard decoder deliberately tolerates some invalid escape sequences.
// Validate escapes first so a damaged attachment is never offered as intact.
type emlQuotedPrintableValidator struct {
	input io.Reader
	state int
}

func (r *emlQuotedPrintableValidator) Read(buffer []byte) (int, error) {
	n, err := r.input.Read(buffer)
	for _, b := range buffer[:n] {
		switch r.state {
		case 0:
			if b == '=' {
				r.state = 1
			}
		case 1:
			switch {
			case b == '\n':
				r.state = 0
			case b == '\r':
				r.state = 3
			case emlHexDigit(b):
				r.state = 2
			default:
				return n, fmt.Errorf("invalid quoted-printable escape")
			}
		case 2:
			if !emlHexDigit(b) {
				return n, fmt.Errorf("invalid quoted-printable escape")
			}
			r.state = 0
		case 3:
			if b != '\n' {
				return n, fmt.Errorf("invalid quoted-printable soft line break")
			}
			r.state = 0
		}
	}
	if err == io.EOF && r.state != 0 {
		return n, fmt.Errorf("truncated quoted-printable escape")
	}
	return n, err
}

func emlHexDigit(b byte) bool {
	return b >= '0' && b <= '9' || b >= 'a' && b <= 'f' || b >= 'A' && b <= 'F'
}

func (p *emlParser) decodeText(data []byte, label string) (string, error) {
	label = strings.TrimSpace(label)
	if label != "" && !strings.EqualFold(label, "utf-8") && !strings.EqualFold(label, "utf8") {
		reader, err := charset.NewReaderLabel(label, bytes.NewReader(data))
		if err != nil {
			p.warn(fmt.Sprintf("Unknown body charset %q; displayed as UTF-8 where possible.", label))
		} else {
			data, err = io.ReadAll(io.LimitReader(reader, maxEMLBytes-p.decodedBytes+1))
			if err != nil {
				return "", err
			}
			if bytes.Contains(data, []byte("\uFFFD")) {
				p.warn("The email charset decoder produced replacement characters; some text may be damaged.")
			}
		}
	}
	if !utf8.Valid(data) {
		p.warn("The email body contains invalid text bytes; replacement characters are shown.")
		return strings.ToValidUTF8(string(data), "\uFFFD"), nil
	}
	return string(data), nil
}

func emlContentID(value string) string {
	return strings.Trim(strings.TrimSpace(value), "<>")
}

func emlCombineBodies(bodies []emlBody, alternative bool) emlBody {
	var combined emlBody
	if alternative {
		// MIME alternatives are ordered from least to most faithful. Keep the
		// last supported rendition of each kind, never concatenate alternatives.
		for _, body := range bodies {
			if body.plain != "" {
				combined.plain = body.plain
			}
			if body.html != "" {
				combined.html = body.html
			}
		}
		return combined
	}
	var plain, markup []string
	hasHTML := false
	for _, body := range bodies {
		if body.plain != "" {
			plain = append(plain, body.plain)
		}
		if body.html != "" {
			hasHTML = true
			markup = append(markup, body.html)
		} else if body.plain != "" {
			markup = append(markup, "<pre>"+html.EscapeString(body.plain)+"</pre>")
		}
	}
	combined.plain = strings.Join(plain, "\n\n")
	if hasHTML {
		combined.html = strings.Join(markup, "\n<hr>\n")
	}
	return combined
}
