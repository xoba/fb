package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/mail"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html/charset"
)

// Each page reads a bounded batch of headers, without decoding message bodies.
// Sorting and filtering apply to that batch; the UI states this for large folders.
const (
	emlInboxPageSize  = 500
	emlInboxHeaderMax = 64 << 10
)

type emlInboxRow struct {
	Filename, Href, From, Subject, To, Date, Size, Warning string
	date                                                   time.Time
}

type emlInboxPage struct {
	Title, FilesHref, Query, FormHref, Previous, Next, Scope string
	Crumbs                                                   []crumb
	Folders                                                  []crumb
	Messages                                                 []emlInboxRow
	Total, Start, End, Page, Pages                           int
}

func emlInboxFile(name string) bool {
	return strings.EqualFold(path.Ext(viewName(name)), ".eml")
}

func (s fileServer) emlInboxHref(name string, directory bool, query url.Values) string {
	u := url.URL{Path: path.Join("/", s.base, name), RawQuery: query.Encode()}
	if directory && !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
	}
	return u.String()
}

func (s fileServer) serveEMLInbox(w http.ResponseWriter, r *http.Request, name string) {
	if !strings.HasSuffix(r.URL.Path, "/") {
		u := *r.URL
		u.Path += "/"
		http.Redirect(w, r, u.String(), http.StatusMovedPermanently)
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) > 1024 {
		http.Error(w, "inbox search is too long", http.StatusBadRequest)
		return
	}
	pageNumber := 1
	if raw := r.URL.Query().Get("page"); raw != "" {
		var err error
		pageNumber, err = strconv.Atoi(raw)
		if err != nil || pageNumber < 1 {
			http.Error(w, "invalid inbox page", http.StatusBadRequest)
			return
		}
	}
	entries, err := fs.ReadDir(s.fsys, name)
	if err != nil {
		http.Error(w, "cannot read directory", http.StatusInternalServerError)
		return
	}
	page := emlInboxPage{
		Title:     "/" + strings.TrimPrefix(strings.TrimSuffix(s.displayPath(name), "/"), "./"),
		FilesHref: s.emlInboxHref(name, true, nil),
		FormHref:  s.emlInboxHref(name, true, nil),
		Crumbs:    s.breadcrumbs(name),
		Query:     query,
	}
	if name == "." && s.base == "" {
		page.Title = "/"
	}
	for i := range page.Crumbs {
		page.Crumbs[i].Href += "?view=inbox"
	}
	if name != "." || s.base != "" {
		u, _ := url.Parse(page.FilesHref)
		u.Path = path.Dir(strings.TrimSuffix(u.Path, "/"))
		if !strings.HasSuffix(u.Path, "/") {
			u.Path += "/"
		}
		u.RawQuery = "view=inbox"
		page.Folders = append(page.Folders, crumb{Name: "../", Href: u.String()})
	}
	var messages []fs.DirEntry
	for _, entry := range entries {
		if entry.IsDir() {
			page.Folders = append(page.Folders, crumb{
				Name: entry.Name() + "/",
				Href: s.emlInboxHref(path.Join(name, entry.Name()), true, url.Values{"view": {"inbox"}}),
			})
		} else if emlInboxFile(entry.Name()) {
			messages = append(messages, entry)
		}
	}
	page.Total = len(messages)
	page.Pages = max(1, (len(messages)+emlInboxPageSize-1)/emlInboxPageSize)
	page.Page = min(pageNumber, page.Pages)
	start := (page.Page - 1) * emlInboxPageSize
	end := min(start+emlInboxPageSize, len(messages))
	page.Start, page.End = start+1, end
	if page.Total == 0 {
		page.Start = 0
	}
	page.Scope = "Messages sorted by date."
	if page.Pages > 1 {
		page.Scope = "Files are grouped into pages by filename; dates and search apply to this page only."
	}
	pageHref := func(n int) string {
		q := url.Values{"view": {"inbox"}, "page": {strconv.Itoa(n)}}
		if query != "" {
			q.Set("q", query)
		}
		return s.emlInboxHref(name, true, q)
	}
	if page.Page > 1 {
		page.Previous = pageHref(page.Page - 1)
	}
	if page.Page < page.Pages {
		page.Next = pageHref(page.Page + 1)
	}
	needle := strings.ToLower(query)
	for _, entry := range messages[start:end] {
		if r.Context().Err() != nil {
			return
		}
		row := s.emlInboxMessage(path.Join(name, entry.Name()), entry)
		if needle == "" || strings.Contains(strings.ToLower(strings.Join([]string{row.Filename, row.Subject, row.From, row.To}, "\n")), needle) {
			page.Messages = append(page.Messages, row)
		}
	}
	sort.SliceStable(page.Messages, func(i, j int) bool {
		a, b := page.Messages[i], page.Messages[j]
		if !a.date.Equal(b.date) {
			return a.date.After(b.date)
		}
		return a.Filename < b.Filename
	})
	var buf bytes.Buffer
	if err := emlInboxTemplate.Execute(&buf, page); err != nil {
		http.Error(w, "cannot render inbox", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeContent(w, r, "inbox.html", time.Time{}, bytes.NewReader(buf.Bytes()))
}

func (s fileServer) emlInboxMessage(name string, entry fs.DirEntry) emlInboxRow {
	row := emlInboxRow{Filename: entry.Name(), Href: s.emlInboxHref(name, false, nil), Subject: "(no subject)", From: "(unknown sender)", Date: "—"}
	if info, err := entry.Info(); err == nil {
		row.Size = humanSize(info.Size())
		row.date = info.ModTime()
	}
	f, err := s.fsys.Open(name)
	if err != nil {
		row.Warning = "Cannot read message headers."
	} else {
		headers, headerErr := readEMLInboxHeaders(f)
		f.Close()
		if headerErr != nil {
			row.Warning = headerErr.Error()
		} else {
			decoder := mime.WordDecoder{CharsetReader: charset.NewReaderLabel}
			decode := func(name string) string {
				raw := headers.Get(name)
				decoded, err := decoder.DecodeHeader(raw)
				if err != nil {
					row.Warning = "Some headers could not be decoded."
					return raw
				}
				return decoded
			}
			if subject := decode("Subject"); subject != "" {
				row.Subject = subject
			}
			if from := decode("From"); from != "" {
				row.From = from
			}
			row.To = decode("To")
			if date, err := mail.ParseDate(headers.Get("Date")); err == nil {
				row.date = date
			}
		}
	}
	if !row.date.IsZero() {
		row.Date = row.date.Format("2006-01-02 15:04 MST")
	}
	return row
}

func readEMLInboxHeaders(reader io.Reader) (mail.Header, error) {
	reader = io.LimitReader(reader, emlInboxHeaderMax+1)
	buffered := bufio.NewReader(reader)
	var header strings.Builder
	for {
		line, err := buffered.ReadString('\n')
		if header.Len()+len(line) > emlInboxHeaderMax {
			return nil, fmt.Errorf("Headers exceed %s.", humanSize(emlInboxHeaderMax))
		}
		header.WriteString(line)
		if line == "\r\n" || line == "\n" || err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.New("Cannot read message headers.")
		}
	}
	message, err := mail.ReadMessage(strings.NewReader(header.String()))
	if err != nil || len(message.Header) == 0 {
		return nil, errors.New("Invalid message headers.")
	}
	return message.Header, nil
}

var emlInboxTemplate = template.Must(template.New("eml-inbox").Parse(`<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Inbox · {{.Title}}</title><style>
:root { color-scheme: light dark; } * { box-sizing: border-box; }
body { font: 16px -apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif; line-height: 1.5; max-width: 1200px; margin: 2rem auto; padding: 0 1.5rem; color: #24292f; background: #fff; }
a { color: #0969da; text-decoration: none; } a:hover { text-decoration: underline; }
nav { display: flex; flex-wrap: wrap; gap: .5rem; align-items: center; } .breadcrumbs { font-size: .9rem; } .breadcrumbs a + a:before { content: "/"; color: #8c959f; margin-right: .5rem; }
header { display: flex; align-items: center; justify-content: space-between; gap: 1rem; } h1 { font-size: 1.8rem; margin-bottom: .5rem; }
.muted, small { color: #57606a; } .warning { display: block; color: #9a6700; }
form { display: flex; flex-wrap: wrap; gap: .5rem; margin: 1rem 0; } input, button { font: inherit; padding: .45rem .65rem; border: 1px solid #d0d7de; border-radius: 6px; color: inherit; background: inherit; } input[type=search] { flex: 1; min-width: 12rem; } button { cursor: pointer; }
.folders { border-top: 1px solid #d0d7de; border-bottom: 1px solid #d0d7de; padding: .65rem 0; margin: 1rem 0; gap: 1rem; }
.table-scroll { overflow-x: auto; } table { width: 100%; border-collapse: collapse; text-align: left; } th { color: #57606a; font-weight: 600; font-size: .85rem; } th,td { padding: .65rem .7rem; border-bottom: 1px solid #d8dee4; vertical-align: top; } th:first-child,td:first-child { padding-left: 0; } td { overflow-wrap: anywhere; } .sender { max-width: 18rem; width: 25%; } .subject { min-width: 14rem; } .date { white-space: nowrap; font-size: .9rem; } small { display: block; font-size: .8rem; } .pagination { justify-content: space-between; margin-top: 1.25rem; }
@media(prefers-color-scheme: dark) { body { color: #e6edf3; background: #0d1117; } a { color: #58a6ff; } .muted,small,th { color: #8b949e; } input,button,.folders,th,td { border-color: #30363d; } .warning { color: #d29922; } }
</style>` + dropJS + freshBackJS + `</head><body>
<nav class="breadcrumbs" aria-label="Breadcrumb">{{range .Crumbs}}<a href="{{.Href}}">{{.Name}}</a>{{end}}</nav>
<header><h1>Inbox</h1><a href="{{.FilesHref}}">Files</a></header>
<p class="muted">{{.Title}} · {{.Total}} message{{if ne .Total 1}}s{{end}}</p>
{{if .Folders}}<nav class="folders" aria-label="Folders">{{range .Folders}}<a href="{{.Href}}">{{.Name}}</a>{{end}}</nav>{{end}}
<form method="get" action="{{.FormHref}}" role="search"><input type="hidden" name="view" value="inbox"><input type="hidden" name="page" value="{{.Page}}"><input type="search" name="q" value="{{.Query}}" aria-label="Search messages on this page" placeholder="Search sender, recipient, subject, or filename"><button type="submit">Search{{if gt .Pages 1}} this page{{end}}</button>{{if .Query}}<a href="{{.FormHref}}?view=inbox&amp;page={{.Page}}">Clear</a>{{end}}</form>
<p class="muted">{{.Scope}}{{if gt .Pages 1}} Showing files {{.Start}}–{{.End}} of {{.Total}}.{{end}}</p>
<div class="table-scroll"><table><thead><tr><th scope="col">Sender</th><th scope="col">Subject</th><th scope="col">Date</th></tr></thead><tbody>
{{range .Messages}}<tr><td class="sender">{{.From}}{{if .To}}<small>To: {{.To}}</small>{{end}}</td><td class="subject"><a href="{{.Href}}">{{.Subject}}</a><small>{{.Filename}}{{if .Size}} · {{.Size}}{{end}}</small>{{if .Warning}}<small class="warning">{{.Warning}}</small>{{end}}</td><td class="date">{{.Date}}</td></tr>
{{else}}<tr><td colspan="3">{{if .Query}}No messages on this page match your search.{{else}}No .eml files in this folder.{{end}}</td></tr>{{end}}
</tbody></table></div>
{{if gt .Pages 1}}<nav class="pagination" aria-label="Inbox pages"><span>{{if .Previous}}<a href="{{.Previous}}">← Previous</a>{{end}}</span><span>Page {{.Page}} of {{.Pages}}</span><span>{{if .Next}}<a href="{{.Next}}">Next →</a>{{end}}</span></nav>{{end}}
` + pageFooter + `</body></html>`))
