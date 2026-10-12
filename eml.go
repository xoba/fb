package main

import (
	"bytes"
	"fmt"
	"html/template"
	"image"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/net/html"
)

const maxEMLPreviewBytes = 2 << 20

// Every link is derived from the message's actual path and a parser-generated
// part ID. Filenames and Content-IDs from email never become filesystem paths.
func (s fileServer) emlURL(name string, query url.Values) string {
	return (&url.URL{Path: path.Join("/", s.base, name), RawQuery: query.Encode()}).String()
}

func findEMLNode(node *emlNode, id string) *emlNode {
	if node == nil || node.ID == id {
		return node
	}
	for _, child := range node.Children {
		if found := findEMLNode(child, id); found != nil {
			return found
		}
	}
	return nil
}

func emlAttachedMessage(node *emlNode) bool {
	return node.MediaType == "message/rfc822" || node.MediaType == "message/global"
}

// A selected part uses its nearest related container or attached message for
// CID resolution. Forwarded messages cannot supply images to their parent.
func emlCIDScope(root *emlNode, id string) *emlNode {
	scope := root
	var visit func(*emlNode)
	visit = func(node *emlNode) {
		if node.ID != id && !strings.HasPrefix(id, node.ID+".") {
			return
		}
		if node.MediaType == "multipart/related" || emlAttachedMessage(node) {
			scope = node
		}
		for _, child := range node.Children {
			visit(child)
		}
	}
	visit(root)
	return scope
}

func emlFilename(node *emlNode) string {
	name := path.Base(strings.ReplaceAll(node.Filename, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, name)
	if name != "" && name != "." && name != ".." && name != "/" {
		return name
	}
	ext := map[string]string{"text/plain": ".txt", "text/html": ".html", "message/rfc822": ".eml", "image/png": ".png", "image/jpeg": ".jpg", "image/gif": ".gif", "image/webp": ".webp"}[node.MediaType]
	if ext == "" {
		ext = ".bin"
	}
	return "part-" + node.ID + ext
}

// Only verified raster images may be embedded. SVG, HTML, mislabeled content,
// and exceptionally large image dimensions must never enter the page as images.
func emlRasterType(node *emlNode) string {
	switch node.MediaType {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "image/bmp":
	default:
		return ""
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(node.Data))
	if err != nil || config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 40_000_000 {
		return ""
	}
	actual := map[string]string{"png": "image/png", "jpeg": "image/jpeg", "gif": "image/gif", "webp": "image/webp", "bmp": "image/bmp"}[format]
	if actual != node.MediaType {
		return ""
	}
	return actual
}

var emlHTMLPolicy = func() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.AllowElements("a", "abbr", "b", "blockquote", "br", "caption", "code", "dd", "del", "div", "dl", "dt", "em", "h1", "h2", "h3", "h4", "h5", "h6", "hr", "i", "img", "li", "ol", "p", "pre", "s", "small", "span", "strong", "sub", "sup", "table", "tbody", "td", "th", "thead", "tr", "u", "ul")
	p.AllowAttrs("href", "title").OnElements("a")
	p.AllowAttrs("src", "alt", "title").OnElements("img")
	p.AllowAttrs("colspan", "rowspan").OnElements("td", "th")
	p.AllowURLSchemes("http", "https", "mailto")
	p.AllowRelativeURLs(true) // Only our generated CID URLs survive the prepass.
	p.RequireParseableURLs(true)
	p.RequireNoReferrerOnLinks(true)
	p.RequireNoFollowOnLinks(true)
	return p
}()

// Strip all fetch-capable URLs before sanitizing markup. The general document
// policy permits local/remote images and relative links, which email must not.
func (s fileServer) emlHTML(source, name string, root *emlNode) template.HTML {
	if len(source) > maxEMLPreviewBytes {
		return ""
	}
	doc, err := html.Parse(strings.NewReader(source))
	if err != nil {
		return ""
	}
	cids := map[string]*emlNode{}
	var collect func(*emlNode)
	collect = func(node *emlNode) {
		if node == nil {
			return
		}
		if node != root && emlAttachedMessage(node) {
			return
		}
		if node.ContentID != "" && emlRasterType(node) != "" {
			if _, exists := cids[node.ContentID]; !exists {
				cids[node.ContentID] = node
			}
		}
		for _, child := range node.Children {
			collect(child)
		}
	}
	collect(root)
	var clean func(*html.Node)
	clean = func(node *html.Node) {
		if node.Type == html.ElementNode {
			attrs := make([]html.Attribute, 0, len(node.Attr))
			for _, attr := range node.Attr {
				if attr.Namespace != "" {
					continue
				}
				switch {
				case node.Data == "a" && attr.Key == "href":
					u, err := url.Parse(strings.TrimSpace(attr.Val))
					if err != nil {
						continue
					}
					scheme := strings.ToLower(u.Scheme)
					if !((scheme == "https" || scheme == "http") && u.Host != "" || scheme == "mailto") {
						continue
					}
					attr.Val = u.String()
				case node.Data == "img" && attr.Key == "src":
					u, err := url.Parse(strings.TrimSpace(attr.Val))
					if err != nil || !strings.EqualFold(u.Scheme, "cid") {
						continue
					}
					cid, err := url.PathUnescape(u.Opaque)
					part := cids[strings.Trim(cid, "<>")]
					if err != nil || part == nil {
						continue
					}
					attr.Val = s.emlURL(name, url.Values{"eml-part": {part.ID}, "inline": {"1"}})
				case attr.Key == "src" || attr.Key == "href":
					continue
				}
				attrs = append(attrs, attr)
			}
			node.Attr = attrs
			if node.Data == "img" {
				src, alt := "", "Image omitted"
				for _, attr := range attrs {
					if attr.Key == "src" {
						src = attr.Val
					}
					if attr.Key == "alt" && attr.Val != "" {
						alt = attr.Val
					}
				}
				if src == "" {
					node.Type, node.Data, node.Attr = html.TextNode, "["+alt+"]", nil
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			clean(child)
		}
	}
	clean(doc)
	var buf bytes.Buffer
	if err := html.Render(&buf, doc); err != nil {
		return ""
	}
	return template.HTML(emlHTMLPolicy.SanitizeBytes(buf.Bytes()))
}

type emlTreeView struct {
	ID, Href, Filename, MediaType, Size string
	Selected                            bool
	Children                            []*emlTreeView
}

type emlPage struct {
	Title, Subject, RawHref, InboxHref, OverviewHref string
	Crumbs                                           []crumb
	Tree                                             *emlTreeView
	Headers, SummaryHeaders                          []emlHeader
	Warnings                                         []string
	HTML                                             template.HTML
	Plain, ImageHref, Source, Error                  string
	SourceTruncated                                  bool
	PartTitle, PartType, PartSize, PartDownload      string
	PartSource                                       string
	PartHeaders                                      []emlHeader
}

func (s fileServer) emlTree(node *emlNode, name, selected string) *emlTreeView {
	if node == nil {
		return nil
	}
	view := &emlTreeView{ID: node.ID, Href: s.emlURL(name, url.Values{"eml-view": {node.ID}}), Filename: node.Filename, MediaType: node.MediaType, Selected: node.ID == selected}
	if len(node.Children) == 0 || len(node.Data) > 0 {
		view.Size = humanSize(int64(len(node.Data)))
	}
	for _, child := range node.Children {
		view.Children = append(view.Children, s.emlTree(child, name, selected))
	}
	return view
}

func emlPreview(text string) string {
	if len(text) <= maxEMLPreviewBytes {
		return text
	}
	return strings.ToValidUTF8(text[:maxEMLPreviewBytes], "\uFFFD") + "\n[Preview truncated — save the original for the complete contents.]"
}

func (s fileServer) serveEML(w http.ResponseWriter, r *http.Request, name string, info fs.FileInfo) {
	release, ok := acquireHeavy(r.Context())
	if !ok {
		return
	}
	defer release()
	page := emlPage{Title: path.Base(name), Subject: path.Base(name), Crumbs: s.breadcrumbs(path.Dir(name)), RawHref: s.emlURL(name, url.Values{"raw": {"1"}}), OverviewHref: s.emlURL(name, nil)}
	page.InboxHref = s.emlURL(path.Dir(name), url.Values{"view": {"inbox"}})
	// Directory URLs require the slash before the query, including archive dirs.
	u, _ := url.Parse(page.InboxHref)
	u.Path = strings.TrimSuffix(u.Path, "/") + "/"
	page.InboxHref = u.String()
	var data []byte
	var err error
	if info.Size() > maxEMLBytes {
		err = fmt.Errorf("message exceeds the %d MiB browsing limit; open the original source to save a copy", maxEMLBytes>>20)
	} else {
		var file fs.File
		file, err = s.fsys.Open(name)
		if err == nil {
			data, err = io.ReadAll(io.LimitReader(file, maxEMLBytes+1))
			file.Close()
		}
	}
	var message *emlMessage
	if err == nil {
		message, err = parseEML(data)
	}
	if r.URL.Query().Has("eml-part") {
		if err != nil {
			http.Error(w, "cannot decode email: "+err.Error(), http.StatusUnprocessableEntity)
			return
		}
		part := findEMLNode(message.Root, r.URL.Query().Get("eml-part"))
		if part == nil || (len(part.Children) != 0 && !emlAttachedMessage(part)) {
			http.NotFound(w, r)
			return
		}
		filename := emlFilename(part)
		disposition, mediaType := "attachment", "application/octet-stream"
		if r.URL.Query().Get("inline") == "1" {
			if mediaType = emlRasterType(part); mediaType == "" {
				http.Error(w, "this part cannot be embedded", http.StatusUnsupportedMediaType)
				return
			}
			disposition = "inline"
		}
		w.Header().Set("Content-Type", mediaType)
		w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": filename}))
		w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox; frame-ancestors 'none'")
		http.ServeContent(w, r, filename, info.ModTime(), bytes.NewReader(part.Data))
		return
	}
	page.Source, page.SourceTruncated = emlPreview(string(data)), len(data) > maxEMLPreviewBytes
	if err != nil {
		page.Error = "Cannot preview this email: " + err.Error()
	} else {
		page.Headers, page.Warnings = message.Headers, message.Warnings
		if message.Subject != "" {
			page.Subject = message.Subject
		}
		for _, want := range []string{"From", "To", "Cc", "Date", "Reply-To"} {
			for _, header := range message.Headers {
				if strings.EqualFold(header.Name, want) {
					page.SummaryHeaders = append(page.SummaryHeaders, header)
				}
			}
		}
		selected := r.URL.Query().Get("eml-view")
		if selected == "" {
			selected = message.Root.ID
		}
		part := findEMLNode(message.Root, selected)
		if part == nil {
			http.NotFound(w, r)
			return
		}
		page.Tree = s.emlTree(message.Root, name, selected)
		if selected == message.Root.ID {
			page.HTML, page.Plain = s.emlHTML(message.HTML, name, message.Root), emlPreview(message.Plain)
			if len(message.HTML) > maxEMLPreviewBytes {
				page.Warnings = append(page.Warnings, "The HTML body exceeds the 2 MiB preview limit. Select its MIME part to inspect the source.")
			}
		} else {
			page.PartTitle, page.PartType, page.PartSize = emlFilename(part), part.MediaType, humanSize(int64(len(part.Data)))
			page.PartHeaders = part.Headers
			if strings.HasPrefix(part.MediaType, "multipart/") {
				page.PartTitle, page.PartSize = "Part "+part.ID, fmt.Sprintf("%d child parts", len(part.Children))
			}
			if len(part.Children) == 0 || emlAttachedMessage(part) {
				page.PartDownload = s.emlURL(name, url.Values{"eml-part": {part.ID}})
			}
			scope := emlCIDScope(message.Root, selected)
			switch {
			case part.MediaType == "text/html":
				page.HTML = s.emlHTML(part.Text, name, scope)
				page.PartSource = emlPreview(part.Text)
				if len(part.Text) > maxEMLPreviewBytes {
					page.Warnings = append(page.Warnings, "The HTML preview exceeds 2 MiB; inspect its source below or save the part.")
				}
			case strings.HasPrefix(part.MediaType, "text/"):
				page.Plain = emlPreview(part.Text)
			case emlAttachedMessage(part) || strings.HasPrefix(part.MediaType, "multipart/"):
				page.HTML, page.Plain = s.emlHTML(part.BodyHTML, name, scope), emlPreview(part.BodyPlain)
				if emlAttachedMessage(part) && len(part.Children) > 0 {
					page.PartHeaders = append(append([]emlHeader(nil), part.Headers...), part.Children[0].Headers...)
				}
				if len(part.BodyHTML) > maxEMLPreviewBytes {
					page.Warnings = append(page.Warnings, "The HTML body exceeds the 2 MiB preview limit. Choose its text or HTML child part.")
				}
			case emlRasterType(part) != "":
				page.ImageHref = s.emlURL(name, url.Values{"eml-part": {part.ID}, "inline": {"1"}})
			default:
				if len(part.Children) > 0 {
					page.Plain = "Choose a child part from the MIME tree above."
				} else {
					page.Plain = "No inline preview for this part. Save a copy to open it in another viewer."
				}
			}
		}
	}
	var buf bytes.Buffer
	if err := emlTemplate.Execute(&buf, page); err != nil {
		http.Error(w, "cannot render email", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src 'self'; style-src 'unsafe-inline'; base-uri 'none'; object-src 'none'; form-action 'none'; frame-ancestors 'none'")
	http.ServeContent(w, r, path.Base(name)+".html", info.ModTime(), bytes.NewReader(buf.Bytes()))
}

var emlTemplate = template.Must(template.New("email").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>{{.Subject}} — {{.Title}}</title>
<style>
:root{color-scheme:light}body{margin:0 auto;max-width:76em;padding:36px 42px;font:15px/1.6 -apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;color:#1f2328;background:#fff}a{color:#0969da;text-decoration:none}a:hover{text-decoration:underline}nav,.tools{display:flex;flex-wrap:wrap;gap:.65em;align-items:center}nav{margin-bottom:1rem}.tools{margin:1rem 0}.tools a{border:1px solid #d0d7de;border-radius:6px;padding:3px 10px}h1{font-size:1.65rem;overflow-wrap:anywhere;margin:1rem 0}.muted{color:#59636e;font-size:.85rem}.eml-tree{border:1px solid #d1d9e0;border-radius:8px;background:#f6f8fa;padding:12px 20px;margin:1rem 0 2rem;overflow:auto}.eml-tree ul{list-style:none;padding-left:1.25rem;margin:.2rem 0}.eml-tree>ul{padding-left:0}.eml-tree a{display:inline-block;padding:3px 7px;border-radius:4px}.eml-tree .selected{background:#ddf4ff;font-weight:600}.eml-tree code{font-size:.8rem;color:#59636e;margin-right:.5em}#eml-body{border-top:1px solid #d1d9e0;padding-top:1.5rem;overflow-wrap:anywhere}#eml-body img{max-width:100%;height:auto}#eml-body table{border-collapse:collapse;max-width:100%}#eml-body td,#eml-body th{border:1px solid #d1d9e0;padding:6px 12px}blockquote{border-left:3px solid #d1d9e0;margin-left:0;padding-left:1em;color:#59636e}pre{white-space:pre-wrap;overflow-wrap:anywhere;font:13px/1.55 ui-monospace,SFMono-Regular,monospace}.plain-body{font:inherit}.headers{display:grid;grid-template-columns:max-content minmax(0,1fr);gap:3px 16px;margin:0 0 1rem}.headers dt{color:#59636e}.headers dd{margin:0;white-space:pre-wrap;overflow-wrap:anywhere}details{margin:1rem 0}summary{cursor:pointer;color:#59636e}.warning{padding:10px 14px;border:1px solid #d4a72c;border-radius:6px;background:#fff8c5}.source{max-height:40em;overflow:auto;padding:12px;background:#f6f8fa}.part-title{font-size:1.2rem}@media(max-width:600px){body{padding:14px}.headers{gap:3px 8px}.eml-tree{padding:10px}}
</style></head><body>
<nav aria-label="Path">{{range .Crumbs}}<a href="{{.Href}}">{{.Name}}</a><span>/</span>{{end}}<strong>{{.Title}}</strong></nav>
<div class="tools"><a href="{{.InboxHref}}">Folder inbox</a><a href="{{.OverviewHref}}">Message overview</a><a href="{{.RawHref}}" download="{{.Title}}">Save original .eml</a></div>
{{with .Tree}}<section class="eml-tree" aria-label="MIME parts"><strong>MIME parts</strong><ul>{{template "node" .}}</ul></section>{{end}}
{{define "node"}}<li><a href="{{.Href}}"{{if .Selected}} class="selected" aria-current="page"{{end}}><code>{{.ID}}</code>{{if .Filename}}{{.Filename}} <span class="muted">{{.MediaType}}</span>{{else}}{{.MediaType}}{{if .Children}} /{{end}}{{end}}</a> <span class="muted">{{.Size}}</span>{{if .Children}}<ul>{{range .Children}}{{template "node" .}}{{end}}</ul>{{end}}</li>{{end}}
<h1>{{.Subject}}</h1><dl class="headers">{{range .SummaryHeaders}}<dt>{{.Name}}</dt><dd>{{.Value}}</dd>{{end}}</dl>
{{if .Headers}}<details><summary>All message headers</summary><dl class="headers">{{range .Headers}}<dt>{{.Name}}</dt><dd>{{.Value}}</dd>{{end}}</dl></details>{{end}}
{{with .Error}}<p class="warning">{{.}}</p>{{end}}{{range .Warnings}}<p class="warning">{{.}}</p>{{end}}
{{if .PartTitle}}<h2 class="part-title">{{.PartTitle}}</h2><p class="muted">{{.PartType}} · {{.PartSize}}{{if .PartDownload}} · <a href="{{.PartDownload}}" download="{{.PartTitle}}">Save this part</a>{{end}}</p>{{end}}
{{with .PartHeaders}}<details><summary>Part headers</summary><dl class="headers">{{range .}}<dt>{{.Name}}</dt><dd>{{.Value}}</dd>{{end}}</dl></details>{{end}}
<section id="eml-body">{{if .HTML}}{{.HTML}}{{else if .ImageHref}}<img src="{{.ImageHref}}" alt="{{.PartTitle}}">{{else if .Plain}}<pre class="plain-body">{{.Plain}}</pre>{{else if not .Error}}<p class="muted">No readable body. Choose a MIME part above.</p>{{end}}</section>
{{if and .HTML .Plain}}<details><summary>Plain-text alternative</summary><pre>{{.Plain}}</pre></details>{{end}}
{{with .PartSource}}<details><summary>HTML source</summary><pre class="source">{{.}}</pre></details>{{end}}
<p class="muted">External images and active content are blocked. Links open only when you follow them.</p>
<details><summary>Original message source{{if .SourceTruncated}} (preview truncated){{end}}</summary><pre class="source">{{.Source}}</pre></details>
` + pageFooter + `</body></html>`))
