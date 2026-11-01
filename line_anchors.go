package main

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Line anchors are fb's one convention for pointing at a line of a text file:
// #L42 names a line and #L10-L20 names a range, the same forms GitHub uses, so
// a link in a Markdown file works in both places. Every numbered view carries
// id="L<n>" on its line numbers (see highlightLines), and pages that show them
// include lineAnchorCSS and lineAnchorJS.
//
// A file with no numbered view of its own (Markdown, plain text, anything over
// the highlighting cap) gets one when the request names a line: the native app
// hands the renderer the full URL, fragment included, and desktopEngine.serve
// records the target in the request context. The deprecated HTTP server never
// receives fragments, so this switch is app-only.
//
// Line count, not size, decides what WebKit can lay out: 8,000 Chroma lines take
// about a second and 53,000 take over thirty, and the growth is steeper than
// linear. So a long file shows a window of lines around the target rather than
// the whole file, which costs the same however large the file is.

const (
	// maxWholeNumberedLines is the longest file shown whole in a line view.
	maxWholeNumberedLines = 10000
	// lineWindowContext is how many lines a window shows on each side of the
	// target. A smaller window would mean more clicks to read onward.
	lineWindowContext = 2000
	// maxWindowBytes bounds the text a window holds, whatever its line lengths.
	maxWindowBytes = 8 << 20
	// maxWindowLineBytes truncates a single line in a window, so a huge one-line
	// file costs a bounded amount of memory.
	maxWindowLineBytes = 64 << 10
)

// lineFragment deliberately requires the L. A bare number is a legitimate
// heading anchor ("## 2024" gets the id 2024), so #2024 must not switch a
// Markdown document away from its rendered view. Pages that already show line
// numbers also accept the bare form; lineAnchorJS handles that.
var lineFragment = regexp.MustCompile(`^L([1-9][0-9]*)(?:-L?([1-9][0-9]*))?$`)

// lineTarget is the first line a #L fragment names; a range's end does not
// change which lines a window should show.
type lineTarget struct{ line int }

type lineTargetKey struct{}

// withLineTarget marks ctx when fragment names a line.
func withLineTarget(ctx context.Context, fragment string) context.Context {
	m := lineFragment.FindStringSubmatch(fragment)
	if m == nil {
		return ctx
	}
	first, err := strconv.Atoi(m[1])
	if err != nil {
		return ctx // Too many digits for any file; there is nothing to show.
	}
	if m[2] != "" {
		// Reversed ranges are accepted, so the lower number is the first line.
		if second, err := strconv.Atoi(m[2]); err == nil && second < first {
			first = second
		}
	}
	return context.WithValue(ctx, lineTargetKey{}, lineTarget{line: first})
}

func lineTargetOf(ctx context.Context) (lineTarget, bool) {
	target, ok := ctx.Value(lineTargetKey{}).(lineTarget)
	return target, ok
}

func hasLineTarget(ctx context.Context) bool {
	_, ok := lineTargetOf(ctx)
	return ok
}

// hasNumberedView reports whether route's normal viewer for name shows line
// numbers (within the highlighting cap). go.sum files render as checksum tables
// with no source section, so they are not among them.
func hasNumberedView(name string) bool {
	switch path.Base(viewName(name)) {
	case "go.sum", "go.work.sum":
		return false
	}
	switch strings.ToLower(path.Ext(viewName(name))) {
	case ".plist", ".json", ".typ":
		return true
	}
	return isGoModFile(name) || isICalendar(name) || highlightable(name)
}

// isBinaryFormat reports whether name's extension is a format whose bytes are
// not text, so a line anchor means nothing and reading the file can be skipped.
func isBinaryFormat(name string) bool {
	ext := strings.ToLower(path.Ext(viewName(name)))
	switch ext {
	case ".parquet", ".pdf", ".doc", ".docx", ".odt", ".epub":
		return true
	}
	return imageExts[ext] || videoExts[ext] || audioExts[ext] || isArchiveName(name) || isTabularContainer(name)
}

// hasRenderedView reports whether name's normal view differs from its plain
// text, which is when a numbered view needs a way back. Oversized files of
// these types may still fall back to plain text there; the link does no harm.
func hasRenderedView(name string) bool {
	ext := strings.ToLower(path.Ext(viewName(name)))
	if _, ok := pandocFormats[ext]; ok || tableDelims[ext] != 0 {
		return true
	}
	switch ext {
	case ".eml", ".html", ".htm", ".svg":
		return true
	}
	switch path.Base(viewName(name)) {
	case "go.sum", "go.work.sum":
		return true
	}
	return false
}

// isNumberableText reports whether data can be shown as numbered lines. Control
// characters are tolerated (form feeds, terminal escapes in logs): a file that
// is valid UTF-8 with no NUL bytes is text for this purpose, and Chroma escapes
// what it writes.
func isNumberableText(data []byte) bool {
	return utf8.Valid(data) && bytes.IndexByte(data, 0) < 0
}

// serveLineView answers a navigation that names a line of a file with no
// numbered view, and reports whether it did. Anything else falls through to
// the normal viewer, where binary formats simply ignore the anchor. A query
// string selects a particular view (?toc=1, ?eml-view=...), so such requests
// keep it. Files already numbered keep their viewer within the highlighting cap.
func (s fileServer) serveLineView(w http.ResponseWriter, r *http.Request, name string, info fs.FileInfo) bool {
	target, targeted := lineTargetOf(r.Context())
	if !targeted || r.URL.RawQuery != "" || isBinaryFormat(name) {
		return false
	}
	withinHighlightCap := info.Size() <= maxHighlightBytes
	if withinHighlightCap && hasNumberedView(name) {
		return false
	}
	if withinHighlightCap {
		data, err := fs.ReadFile(s.fsys, name)
		if err != nil || !isNumberableText(data) {
			return false
		}
		if bytes.Count(data, []byte("\n")) <= maxWholeNumberedLines {
			// The filename picks a lexer, so Markdown and HTML source get colored.
			s.renderSourcePage(w, r, name, info, path.Base(viewName(name)), string(data))
			return true
		}
	}
	return s.serveLineWindow(w, r, name, info, target)
}

// lineWindow is the part of a file shown around a target line.
type lineWindow struct {
	start int // line number of lines[0]
	lines []string
	more  bool // lines follow the window
}

// findLineWindow finds the lines around target. A target past the end of the file
// gets the file's last lines, which takes a second read now that the end is known;
// keeping lines on the way there would make every long scan pay for this rare case.
// ok is false when the content is not text.
func findLineWindow(ctx context.Context, open func() (io.ReadCloser, error), target int) (window lineWindow, ok bool, err error) {
	read := func(target int) (lineWindow, int, bool, error) {
		f, err := open()
		if err != nil {
			return lineWindow{}, 0, false, err
		}
		defer f.Close()
		return readLineWindow(ctx, f, target)
	}
	window, last, ok, err := read(target)
	if err == nil && ok && len(window.lines) == 0 && last > 0 {
		window, _, ok, err = read(last)
	}
	return window, ok, err
}

// readLineWindow reads no more of r than it must, and stores nothing before the
// window, which is what lets it find a window in a file of any size. last is the
// number of lines read. Lines outside the window are only checked for NUL bytes,
// so a stray Latin-1 line far from the target does not disqualify a file; the
// window itself must be valid UTF-8.
func readLineWindow(ctx context.Context, r io.Reader, target int) (window lineWindow, last int, ok bool, err error) {
	reader := bufio.NewReaderSize(r, maxWindowLineBytes)
	held := 0
	for {
		line, truncated, err := readWindowLine(reader)
		if err != nil && err != io.EOF {
			return window, last, false, err
		}
		if err == io.EOF && len(line) == 0 {
			break
		}
		if bytes.IndexByte(line, 0) >= 0 {
			return window, last, false, nil
		}
		last++
		if last&0xfff == 0 && ctx.Err() != nil {
			return window, last, false, ctx.Err()
		}
		if last >= target+lineWindowContext || (last >= target && held > maxWindowBytes) {
			window.more = true
			break
		}
		if last < target-lineWindowContext {
			continue
		}
		text := string(line)
		if truncated {
			text += " [line truncated]"
		}
		window.lines = append(window.lines, text)
		held += len(text)
		// Long lines before the target give way, so the target is always reached.
		for last < target && held > maxWindowBytes && len(window.lines) > 1 {
			held -= len(window.lines[0])
			window.lines = window.lines[1:]
		}
		window.start = last - len(window.lines) + 1
	}
	for _, line := range window.lines {
		if !utf8.ValidString(line) {
			return window, last, false, nil
		}
	}
	return window, last, true, nil
}

// readWindowLine returns the next line with its newline removed, keeping at most
// maxWindowLineBytes of it and consuming the rest without storing it.
func readWindowLine(reader *bufio.Reader) (line []byte, truncated bool, err error) {
	for {
		chunk, readErr := reader.ReadSlice('\n')
		if room := maxWindowLineBytes - len(line); len(chunk) > room {
			chunk, truncated = chunk[:room], true
		}
		line = append(line, chunk...)
		if readErr == bufio.ErrBufferFull {
			continue
		}
		if readErr != nil && readErr != io.EOF {
			return nil, false, readErr
		}
		if truncated {
			return trimPartialRune(line), true, readErr
		}
		line = bytes.TrimSuffix(line, []byte("\n"))
		line = bytes.TrimSuffix(line, []byte("\r"))
		return line, false, readErr
	}
}

// trimPartialRune removes an incomplete UTF-8 sequence left at the end of b by
// cutting it short.
func trimPartialRune(b []byte) []byte {
	for i := len(b) - 1; i >= 0 && i >= len(b)-utf8.UTFMax; i-- {
		if utf8.RuneStart(b[i]) {
			if !utf8.FullRune(b[i:]) {
				return b[:i]
			}
			break
		}
	}
	return b
}

// serveLineWindow shows the lines around the target. Reports false, having
// written nothing, when the file turns out not to be text.
func (s fileServer) serveLineWindow(w http.ResponseWriter, r *http.Request, name string, info fs.FileInfo, target lineTarget) bool {
	open := func() (io.ReadCloser, error) { return s.fsys.Open(name) }
	window, ok, err := findLineWindow(r.Context(), open, target.line)
	if err != nil || !ok {
		return false
	}
	// Plain text: a lexer starting mid-file would color most of a window wrongly.
	code, err := highlightLines("window.txt", strings.Join(window.lines, "\n")+"\n", window.start)
	if err != nil {
		return false
	}
	end := window.start + len(window.lines) - 1
	page := sourcePage{
		Title:   path.Base(name),
		Crumbs:  s.breadcrumbs(path.Dir(name)),
		RawHref: (&url.URL{Path: path.Base(name), RawQuery: "raw=1"}).String(),
		AltHref: lineViewAltHref(r, name),
		Code:    code,
		Window:  &sourceWindow{From: window.start, To: end},
	}
	if window.start > 1 {
		page.Window.Earlier = window.start - 1
	}
	if window.more {
		page.Window.Later = end + 1
	}
	var buf bytes.Buffer
	if err := sourceTemplate.Execute(&buf, page); err != nil {
		return false
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeContent(w, r, path.Base(name)+".html", info.ModTime(), bytes.NewReader(buf.Bytes()))
	return true
}

// lineViewAltHref is the link from a numbered view back to name's normal view.
// The query matters: WebKit treats URLs differing only by fragment as one document,
// so without it, Back from the rendered view to name#L5 would change the fragment
// without asking for the numbered view again. Nothing reads the parameter.
func lineViewAltHref(r *http.Request, name string) string {
	if !hasLineTarget(r.Context()) || !hasRenderedView(name) {
		return ""
	}
	return (&url.URL{Path: path.Base(name), RawQuery: "view=rendered"}).String()
}

// isMinifiedJSON reports whether data is all on one line, which is when
// pretty-printing it is worth more than keeping the file's own line numbers.
// A multi-line file displays as written so that #L42 means line 42 on disk.
func isMinifiedJSON(data []byte) bool {
	return !bytes.Contains(bytes.TrimSpace(data), []byte("\n"))
}

// lineAnchorCSS highlights the lines lineAnchorJS marks. Measured on a 30,000-line
// file, each of the obvious alternatives makes moving between lines take seconds
// to half a minute rather than a fiftieth of one: :target and :has recalculate every
// line when the fragment changes, and box-shadow makes WebKit recompute overflow,
// so the left edge is a background gradient. Only targeted lines get the scroll
// margin, for the same reason.
const lineAnchorCSS = `  div.source pre span.line-target {
    background: linear-gradient(#d4a72c, #d4a72c) left / 3px 100% no-repeat, #fff8c5;
  }
  div.source pre span.line-target > [id] { scroll-margin-top: 30vh; }
  p.line-window { color: #57606a; font-size: 0.85rem; }
  p.line-window a { margin-left: 0.75rem; }
`

// lineAnchorJS marks and scrolls to the lines the fragment names, in every form:
// #L42, a range (#L10-L20), and the bare alias (#42). It opens a collapsed section
// around the target (a calendar's source). Back and Reload keep the reader's
// position, except in a window, where the target is the reason the page exists
// and a line outside it means asking the renderer for a different window.
const lineAnchorJS = `<script>
(function () {
  var marked = [];
  var windowed = document.querySelector("div.source[data-windowed]") !== null;
  function apply(scroll, changed) {
    marked.forEach(function (line) { line.classList.remove("line-target"); });
    marked = [];
    var m = /^#L?([1-9][0-9]*)(?:-L?([1-9][0-9]*))?$/.exec(location.hash);
    if (!m) return;
    var from = +m[1], to = m[2] ? +m[2] : from;
    if (to < from) { var swap = from; from = to; to = swap; }
    var first = document.getElementById("L" + from);
    if (!first) {
      if (windowed && changed && location.hash.charAt(1) === "L") location.reload();
      else if (windowed) scrollTo(0, document.documentElement.scrollHeight); // past the end
      return;
    }
    for (var n = from, number = first; n <= to && number; n++, number = document.getElementById("L" + n)) {
      number.parentElement.classList.add("line-target");
      marked.push(number.parentElement);
    }
    var section = first.closest("details");
    if (section) section.open = true;
    if (scroll) first.scrollIntoView({ block: "start" });
  }
  var entry = performance.getEntriesByType("navigation")[0];
  apply(windowed || !entry || entry.type === "navigate", false);
  addEventListener("hashchange", function () { apply(true, true); });
})();
</script>
`
