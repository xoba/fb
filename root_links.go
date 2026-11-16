package main

import (
	"bytes"
	"io"
	"io/fs"
	"net/url"
	"path"
	"strings"

	"golang.org/x/net/html"
)

// Root-relative links in rendered documents.
//
// A repository's README often references its own files with root-relative
// paths — ![logo](/docs/img/logo.svg), [guide](/AGENTS.md) — which GitHub
// resolves against the repository root. In fb those URLs would resolve against
// fb's serve root instead (the filesystem root in the desktop app), so they
// break. resolveRootRelativeLinks rewrites them to point into the enclosing
// git repository, the way GitHub reads them.
//
// A link is rewritten only when its target exists under the repository root.
// Anything else keeps fb's own meaning, so a personal note inside a repository
// can still link an absolute path like /Volumes/photos/cat.png.

// repoRoot returns the fs path of the directory holding the git repository
// that contains name: the nearest ancestor of name with a .git entry (a
// directory, or the file that marks a worktree or submodule).
func (s fileServer) repoRoot(name string) (string, bool) {
	for dir := path.Dir(name); ; dir = path.Dir(dir) {
		if _, err := fs.Stat(s.fsys, path.Join(dir, ".git")); err == nil {
			return dir, true
		}
		if dir == "." || dir == "/" {
			return "", false
		}
	}
}

// resolveRootRelativeLinks rewrites root-relative a href and img src URLs in
// a sanitized HTML fragment rendered from the document name, so they resolve
// against name's git repository rather than fb's serve root. Every other byte
// of the fragment passes through unchanged.
func (s fileServer) resolveRootRelativeLinks(fragment []byte, name string) []byte {
	if !bytes.Contains(fragment, []byte(`="/`)) {
		return fragment
	}
	repo, ok := s.repoRoot(name)
	if !ok || (repo == "." && s.base == "") {
		// No repository, or the repository is the serve root itself, where
		// root-relative URLs already resolve the way GitHub would.
		return fragment
	}

	z := html.NewTokenizer(bytes.NewReader(fragment))
	var out bytes.Buffer
	out.Grow(len(fragment))
	changed := false
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			if z.Err() != io.EOF {
				return fragment
			}
			break
		}
		if tt != html.StartTagToken && tt != html.SelfClosingTagToken {
			out.Write(z.Raw())
			continue
		}
		// Token unescapes and lowercases inside the tokenizer's buffer, so
		// keep a copy of the tag exactly as written.
		raw := append([]byte(nil), z.Raw()...)
		tok := z.Token()
		attr := map[string]string{"a": "href", "img": "src"}[tok.Data]
		rewrote := false
		for i, a := range tok.Attr {
			if a.Namespace != "" || a.Key != attr {
				continue
			}
			if resolved, ok := s.resolveRootURL(repo, a.Val); ok {
				tok.Attr[i].Val = resolved
				rewrote = true
			}
		}
		if rewrote {
			out.WriteString(tok.String())
			changed = true
		} else {
			out.Write(raw)
		}
	}
	if !changed {
		return fragment
	}
	return out.Bytes()
}

// resolveRootURL maps a root-relative URL such as /img/logo.svg?x#y onto the
// repository directory repo, keeping its query and fragment. It reports false
// for any other URL, and for targets that do not exist in the repository.
func (s fileServer) resolveRootURL(repo, raw string) (string, bool) {
	if !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "" || u.Host != "" || u.Opaque != "" {
		return "", false
	}
	// Cleaning the rooted path first drops leading ".." segments, so the
	// target cannot climb out of the repository.
	target := path.Join(repo, path.Clean(u.Path))
	if _, err := fs.Stat(s.fsys, target); err != nil {
		return "", false
	}
	resolved := s.base + "/"
	if target != "." {
		resolved += target
	}
	if strings.HasSuffix(u.Path, "/") && !strings.HasSuffix(resolved, "/") {
		resolved += "/"
	}
	u.Path, u.RawPath = resolved, ""
	return u.String(), true
}
