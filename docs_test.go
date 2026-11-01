package main

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The documentation ships inside the app, so these tests hold it to what the code
// does: every supported type must be documented, and every link must lead somewhere.

const docsRoot = "docs"

func readDocs(t *testing.T) map[string]string {
	t.Helper()
	pages := map[string]string{}
	err := filepath.WalkDir(docsRoot, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".md" {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		pages[filepath.ToSlash(p)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) == 0 {
		t.Fatal("no documentation found")
	}
	return pages
}

// sortedKeys returns a registry's keys, whatever it maps them to.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// documentedTypes names the file-type page responsible for each type. Formats
// dispatched by a helper or a switch rather than a table are listed with a
// check that the code still handles them, so a removed format fails here as well.
func documentedTypes() map[string][]string {
	byPage := map[string][]string{
		"file-types/source-code.md": append(sortedKeys(highlightExts), sortedKeys(highlightNames)...),
		"file-types/csv-tsv.md":     sortedKeys(tableDelims),
		"file-types/images.md":      sortedKeys(imageExts),
		"file-types/video.md":       sortedKeys(videoExts),
		"file-types/audio.md":       sortedKeys(audioExts),
	}
	for ext := range pandocFormats {
		page := map[string]string{
			".md":    "file-types/markdown.md",
			".rst":   "file-types/restructuredtext.md",
			".ipynb": "file-types/notebooks.md",
			".docx":  "file-types/word-processing.md",
			".doc":   "file-types/word-processing.md",
			".odt":   "file-types/word-processing.md",
			".rtf":   "file-types/word-processing.md",
			".epub":  "file-types/epub.md",
		}[ext]
		if page == "" {
			panic("pandocFormats has an extension with no documentation page: " + ext)
		}
		byPage[page] = append(byPage[page], ext)
	}
	for _, name := range []string{".zip", ".jar", ".tar", ".tar.gz", ".tgz", ".tar.bz2"} {
		if !isArchiveName("file" + name) {
			panic(name + " is documented as an archive but is not one")
		}
		byPage["file-types/archives.md"] = append(byPage["file-types/archives.md"], name)
	}
	for _, name := range []string{"go.mod", "go.work", "go.sum", "go.work.sum"} {
		if !isGoModFile(name) {
			panic(name + " is documented as a Go module file but is not one")
		}
		byPage["file-types/go-modules.md"] = append(byPage["file-types/go-modules.md"], name)
	}
	for _, ext := range []string{".sqlite", ".sqlite3", ".db"} {
		if !isTabularContainer("file"+ext) || isXLSX("file"+ext) {
			panic(ext + " is documented as a database but is not one")
		}
		byPage["file-types/sqlite.md"] = append(byPage["file-types/sqlite.md"], ext)
	}
	if !isXLSX("file.xlsx") {
		panic(".xlsx is documented as a workbook but is not one")
	}
	if !isICalendar("file.ics") {
		panic(".ics is documented as a calendar but is not one")
	}
	byPage["file-types/xlsx.md"] = []string{".xlsx"}
	byPage["file-types/calendar.md"] = []string{".ics"}
	byPage["file-types/parquet.md"] = []string{".parquet"}
	byPage["file-types/json.md"] = []string{".json"}
	byPage["file-types/typst.md"] = []string{".typ"}
	byPage["file-types/property-lists.md"] = []string{".plist"}
	byPage["file-types/email.md"] = []string{".eml"}
	byPage["file-types/pdf.md"] = []string{".pdf"}
	byPage["file-types/html-svg.md"] = []string{".html", ".htm", ".svg"}
	byPage["file-types/plain-text.md"] = []string{".txt"}
	return byPage
}

func TestDocsCoverEveryFileType(t *testing.T) {
	pages := readDocs(t)
	index := pages[docsRoot+"/file-types/index.md"]
	if index == "" {
		t.Fatal("the file type index is missing")
	}
	byPage := documentedTypes()
	for page, types := range byPage {
		content, ok := pages[docsRoot+"/"+page]
		if !ok {
			t.Errorf("%s does not exist, but documents %v", page, types)
			continue
		}
		for _, name := range types {
			if !strings.Contains(content, "`"+name+"`") {
				t.Errorf("%s does not mention `%s`", page, name)
			}
			if !strings.Contains(index, "`"+name+"`") {
				t.Errorf("file-types/index.md does not mention `%s`", name)
			}
		}
		// The index must lead to the page.
		if !strings.Contains(index, "("+path.Base(page)+")") {
			t.Errorf("file-types/index.md does not link to %s", path.Base(page))
		}
	}
	// Every page in the folder is accounted for, so none can drift out of the table.
	for name := range pages {
		rel := strings.TrimPrefix(name, docsRoot+"/")
		if strings.HasPrefix(rel, "file-types/") && rel != "file-types/index.md" {
			if _, ok := byPage[rel]; !ok {
				t.Errorf("%s documents no registered type", rel)
			}
		}
	}
}

// Each file type page follows the same outline, so a reader knows where to look.
func TestDocsFileTypePagesFollowTheTemplate(t *testing.T) {
	for name, content := range readDocs(t) {
		rel := strings.TrimPrefix(name, docsRoot+"/")
		if !strings.HasPrefix(rel, "file-types/") || rel == "file-types/index.md" {
			continue
		}
		for _, want := range []string{"\n**", "\n## What you see\n", "\n## Options\n", "\n## Limits and fallbacks\n", "\n## Try it\n", "(index.md)", "(../index.md)"} {
			if !strings.Contains(content, want) {
				t.Errorf("%s is missing %q", rel, want)
			}
		}
		if !strings.HasPrefix(content, "# ") {
			t.Errorf("%s must start with a title", rel)
		}
	}
}

var (
	markdownLink = regexp.MustCompile(`\]\(([^()\s]+)\)`)
	codeFence    = regexp.MustCompile("(?ms)^ *```.*?^ *```")
	codeSpan     = regexp.MustCompile("`[^`\n]*`")
	headingLine  = regexp.MustCompile(`(?m)^#{1,6} +(.+?) *$`)
)

// proseOf removes code, whose link-like text is an example rather than a link.
func proseOf(content string) string {
	return codeSpan.ReplaceAllString(codeFence.ReplaceAllString(content, ""), "")
}

// docLinks returns a page's link targets. Removing a code span leaves a link whose
// text was code intact, while a whole link written inside code is only an example.
func docLinks(content string) []string {
	var links []string
	for _, m := range markdownLink.FindAllStringSubmatch(proseOf(content), -1) {
		links = append(links, m[1])
	}
	return links
}

// gfmAnchor is Pandoc's gfm_auto_identifiers rule for a heading, which the app's
// Help search also applies. TestDocsAnchorsMatchPandoc checks the two agree.
func gfmAnchor(heading string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(heading) {
		switch {
		case r == ' ':
			b.WriteRune('-')
		case r == '-' || r == '_' || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r > 127:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func headingAnchors(content string) map[string]bool {
	anchors := map[string]bool{}
	seen := map[string]int{}
	for _, m := range headingLine.FindAllStringSubmatch(codeFence.ReplaceAllString(content, ""), -1) {
		anchor := gfmAnchor(strings.ReplaceAll(m[1], "`", ""))
		if n := seen[anchor]; n > 0 {
			anchors[anchor+"-"+strconv.Itoa(n)] = true
		} else {
			anchors[anchor] = true
		}
		seen[anchor]++
	}
	return anchors
}

// embeddedExamples must match the list in macos/Scripts/embed-docs.sh.
var embeddedExamples = []string{
	"examples/README.md", "examples/blurb.txt", "examples/mime-tour.eml", "examples/mime-tour-reply.eml",
	"examples/documents/", "examples/formats/", "examples/source/", "examples/directory-demo/",
}

func isEmbeddedExample(repoPath string) bool {
	// A folder itself, such as examples/ for its Inbox, is embedded when it has embedded files.
	if repoPath == "examples" {
		return true
	}
	for _, allowed := range embeddedExamples {
		if repoPath == strings.TrimSuffix(allowed, "/") || strings.HasPrefix(repoPath, allowed) {
			return true
		}
	}
	return false
}

func TestDocsEmbedListMatchesTheScript(t *testing.T) {
	script, err := os.ReadFile("macos/Scripts/embed-docs.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, allowed := range embeddedExamples {
		if !strings.Contains(string(script), strings.TrimSuffix(allowed, "/")) {
			t.Errorf("embed-docs.sh does not embed %s", allowed)
		}
	}
}

// Links are resolved the way the app does, so a broken one fails here rather than
// showing a reader a missing-file page.
func TestDocsLinksResolve(t *testing.T) {
	pages := readDocs(t)
	for name, content := range pages {
		for _, link := range docLinks(content) {
			u, err := url.Parse(link)
			if err != nil {
				t.Errorf("%s: unparseable link %q", name, link)
				continue
			}
			if u.Scheme == "https" || u.Scheme == "http" || u.Scheme == "mailto" {
				continue
			}
			if u.Scheme != "" {
				t.Errorf("%s: %q will be removed by the sanitizer", name, link)
				continue
			}
			if strings.HasPrefix(u.Path, "/") {
				t.Errorf("%s: %q is absolute, which only works on one Mac", name, link)
				continue
			}
			target := name
			if u.Path != "" {
				target = path.Join(path.Dir(name), u.Path)
			}
			if err := checkDocTarget(target); err != nil {
				t.Errorf("%s: %q: %v", name, link, err)
				continue
			}
			if !strings.HasPrefix(target, docsRoot+"/") && !isEmbeddedExample(target) {
				t.Errorf("%s: %q leads to %s, which does not ship with the app", name, link, target)
			}
			checkDocAnchor(t, name, link, target, u.Fragment, pages)
		}
	}
}

// checkDocTarget finds a linked file on disk, or inside an archive on disk.
func checkDocTarget(target string) error {
	_, err := os.Stat(filepath.FromSlash(target))
	if err == nil {
		return nil
	}
	for prefix := path.Dir(target); prefix != "." && prefix != "/"; prefix = path.Dir(prefix) {
		if _, statErr := os.Stat(filepath.FromSlash(prefix)); statErr == nil && isArchiveName(prefix) {
			archive, zipErr := zip.OpenReader(filepath.FromSlash(prefix))
			if zipErr != nil {
				return zipErr
			}
			defer archive.Close()
			_, openErr := archive.Open(strings.TrimPrefix(target, prefix+"/"))
			return openErr
		}
	}
	return err
}

var (
	lineAnchorInDocs = regexp.MustCompile(`^L([0-9]+)(?:-L([0-9]+))?$`)
	cellAnchorInDocs = regexp.MustCompile(`^cell=[1-9][0-9]*,[1-9][0-9]*$`)
)

func checkDocAnchor(t *testing.T, page, link, target, fragment string, pages map[string]string) {
	t.Helper()
	switch {
	case fragment == "":
	case lineAnchorInDocs.MatchString(fragment):
		m := lineAnchorInDocs.FindStringSubmatch(fragment)
		data, err := os.ReadFile(filepath.FromSlash(target))
		if err != nil {
			t.Errorf("%s: %q: %v", page, link, err)
			return
		}
		lines := bytes.Count(data, []byte("\n")) + 1
		for _, number := range m[1:] {
			if n, err := strconv.Atoi(number); number != "" && (err != nil || n < 1 || n > lines) {
				t.Errorf("%s: %q names a line past the end of a %d-line file", page, link, lines)
			}
		}
	case cellAnchorInDocs.MatchString(fragment):
		if tableDelims[strings.ToLower(path.Ext(target))] == 0 {
			t.Errorf("%s: %q uses a cell anchor on a file that is not a table", page, link)
		}
	case strings.HasSuffix(target, ".md"):
		content, ok := pages[target]
		if !ok {
			data, err := os.ReadFile(filepath.FromSlash(target))
			if err != nil {
				t.Errorf("%s: %q: %v", page, link, err)
				return
			}
			content = string(data)
		}
		if !headingAnchors(content)[fragment] {
			t.Errorf("%s: %q: %s has no heading with that anchor", page, link, target)
		}
	default:
		t.Errorf("%s: %q uses an anchor fb cannot resolve", page, link)
	}
}

// Samples the docs link to must be tracked, or they will not ship: most of the
// examples folder is ignored, and a local file would pass TestDocsLinksResolve.
func TestDocsSamplesAreTracked(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	if err := exec.Command("git", "rev-parse", "--is-inside-work-tree").Run(); err != nil {
		t.Skip("not a git checkout")
	}
	for name, content := range readDocs(t) {
		for _, link := range docLinks(content) {
			u, err := url.Parse(link)
			if err != nil || u.Scheme != "" || u.Path == "" {
				continue
			}
			target := path.Join(path.Dir(name), u.Path)
			if !strings.HasPrefix(target, "examples/") {
				continue
			}
			for prefix := target; prefix != "."; prefix = path.Dir(prefix) {
				if isArchiveName(prefix) {
					target = prefix
					break
				}
			}
			// Tracked files and folders holding tracked files both list something.
			out, err := exec.Command("git", "ls-files", "--", target).Output()
			if err != nil || len(strings.TrimSpace(string(out))) == 0 {
				t.Errorf("%s: %q is not tracked by git, so it will not ship", name, link)
			}
		}
	}
}

// Every page renders through the real converter, its links survive sanitizing, and
// the anchor rule used above and by Help search agrees with the identifiers it makes.
func TestDocsAnchorsMatchPandoc(t *testing.T) {
	if _, err := exec.LookPath("pandoc"); err != nil {
		t.Skip("pandoc not available")
	}
	server := fileServer{fsys: os.DirFS("."), pandoc: "pandoc"}
	for name, content := range readDocs(t) {
		req := httptest.NewRequest(http.MethodGet, "/"+name, nil)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status %d", name, rec.Code)
			continue
		}
		body := rec.Body.String()
		for anchor := range headingAnchors(content) {
			if !strings.Contains(body, `id="`+anchor+`"`) {
				t.Errorf("%s: Pandoc did not produce the anchor %q", name, anchor)
			}
		}
		if prose := proseOf(content); strings.Contains(prose, "](") && !strings.Contains(body, "<a href=") {
			t.Errorf("%s: links did not survive rendering", name)
		}
	}
}
