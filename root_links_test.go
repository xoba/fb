package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func rootLinksFS() fstest.MapFS {
	return fstest.MapFS{
		"proj/.git/HEAD":         {Data: []byte("ref: refs/heads/main\n")},
		"proj/README.md":         {Data: []byte("# Proj\n")},
		"proj/docs/guide.md":     {Data: []byte("# Guide\n")},
		"proj/img/logo.svg":      {Data: []byte("<svg/>")},
		"proj/AGENTS.md":         {Data: []byte("# Agents\n")},
		"proj/sub/file.txt":      {Data: []byte("x\n")},
		"Volumes/photos/cat.png": {Data: []byte("png")},
		"loose/README.md":        {Data: []byte("# Loose\n")},
		"loose/img/logo.svg":     {Data: []byte("<svg/>")},
		"wt/.git":                {Data: []byte("gitdir: /elsewhere\n")},
		"wt/img/logo.svg":        {Data: []byte("<svg/>")},
		"wt/notes/README.md":     {Data: []byte("# Worktree\n")},
	}
}

func TestResolveRootRelativeLinks(t *testing.T) {
	const fragment = `<p><img src="/img/logo.svg" alt="Logo"/> ` +
		`<a href="/AGENTS.md#setup">agents</a> ` +
		`<a href="/missing.md">missing</a> ` +
		`<img src="//cdn.example/x.png"/> ` +
		`<a href="https://example.com/">ext</a> ` +
		`<a href="/Volumes/photos/cat.png">cat</a> ` +
		`<a href="/sub/">sub</a> ` +
		`<a href="/">root</a> ` +
		`<a href="/../../Volumes/photos/cat.png">climb</a> ` +
		`<a href="#frag">frag</a> ` +
		`<code>src=&#34;/img/logo.svg&#34;</code></p>`

	tests := []struct {
		name, doc, base string
		want, keep      []string
	}{
		{
			name: "readme at repo root",
			doc:  "proj/README.md",
			want: []string{
				`<img src="/proj/img/logo.svg" alt="Logo"/>`,
				`<a href="/proj/AGENTS.md#setup">agents</a>`,
				`<a href="/proj/sub/">sub</a>`,
				`<a href="/proj/">root</a>`,
			},
			keep: []string{
				`<a href="/missing.md">missing</a>`,
				`<img src="//cdn.example/x.png"/>`,
				`<a href="https://example.com/">ext</a>`,
				// Not in the repository: keeps fb's absolute meaning.
				`<a href="/Volumes/photos/cat.png">cat</a>`,
				// ".." cannot climb out to reach Volumes/photos/cat.png.
				`<a href="/../../Volumes/photos/cat.png">climb</a>`,
				`<a href="#frag">frag</a>`,
				`<code>src=&#34;/img/logo.svg&#34;</code>`,
			},
		},
		{
			name: "nested document resolves against the repo root",
			doc:  "proj/docs/guide.md",
			want: []string{`<img src="/proj/img/logo.svg" alt="Logo"/>`},
		},
		{
			name: "mounted under an archive base",
			doc:  "proj/README.md",
			base: "/notes/bundle.zip",
			want: []string{`<img src="/notes/bundle.zip/proj/img/logo.svg" alt="Logo"/>`},
		},
		{
			name: "worktree marked by a .git file",
			doc:  "wt/notes/README.md",
			want: []string{`<img src="/wt/img/logo.svg" alt="Logo"/>`},
		},
		{
			name: "outside any repository",
			doc:  "loose/README.md",
			keep: []string{`<img src="/img/logo.svg" alt="Logo"/>`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := fileServer{fsys: rootLinksFS(), base: tt.base}
			got := string(s.resolveRootRelativeLinks([]byte(fragment), tt.doc))
			for _, want := range append(tt.want, tt.keep...) {
				if !strings.Contains(got, want) {
					t.Errorf("missing %s\nin %s", want, got)
				}
			}
		})
	}
}

func TestResolveRootRelativeLinksRepoAtServeRoot(t *testing.T) {
	fsys := fstest.MapFS{
		".git/HEAD":    {Data: []byte("ref: refs/heads/main\n")},
		"img/logo.svg": {Data: []byte("<svg/>")},
	}
	in := []byte(`<p><img src="/img/logo.svg" alt="Logo" /></p>`)
	got := fileServer{fsys: fsys}.resolveRootRelativeLinks(in, "README.md")
	if string(got) != string(in) {
		t.Fatalf("got %s, want the fragment untouched", got)
	}
}

// TestReadmeRootRelativeImageRendered goes through the real pandoc: a repo's
// README that references /img/logo.svg, as GitHub READMEs do, must show the
// repository's image both as a page and as a directory listing's preview.
func TestReadmeRootRelativeImageRendered(t *testing.T) {
	if _, err := exec.LookPath("pandoc"); err != nil {
		t.Skip("pandoc not available")
	}
	root := t.TempDir()
	for name, content := range map[string]string{
		"repo/.git/HEAD":     "ref: refs/heads/main\n",
		"repo/img/logo.svg":  `<svg xmlns="http://www.w3.org/2000/svg"/>`,
		"repo/AGENTS.md":     "# Agents\n",
		"repo/README.md":     "# Repo\n\n![Logo](/img/logo.svg)\n\nSee [AGENTS.md](/AGENTS.md).\n",
		"repo/docs/guide.md": "![Logo](/img/logo.svg)\n",
	} {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	server := fileServer{fsys: os.DirFS(root), pandoc: "pandoc", dir: root}

	for _, target := range []string{"/repo/", "/repo/README.md", "/repo/docs/guide.md"} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d; body: %s", target, rec.Code, rec.Body.String())
		}
		body := strings.Join(strings.Fields(rec.Body.String()), " ")
		if !strings.Contains(body, `src="/repo/img/logo.svg"`) {
			t.Errorf("%s: body lacks the repository image src:\n%s", target, body)
		}
		if target != "/repo/docs/guide.md" && !strings.Contains(body, `href="/repo/AGENTS.md"`) {
			t.Errorf("%s: body lacks the repository link:\n%s", target, body)
		}
	}
}
