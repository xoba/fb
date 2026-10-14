package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
)

func TestAssemblyLexerMixedDialects(t *testing.T) {
	// Synthetic host assembly and accelerator instructions exercise the two
	// meanings of # without depending on any user's source files.
	src := "# 1. Host code\n" +
		"        .text   \nentry:\n" +
		"        li a0, (3 << 12) # host comment\n" +
		"        .desc VEC, rows=2, scale=1.25  \n" +
		"#2. Accelerator code\n" +
		"        fnma v2, v0, #1.25e-06, v1 # instruction comment\n" +
		"        max v2, v2, #-127\n" +
		"        mov v3, #0x20 ; semicolon comment\n" +
		"        add a0, a0, 2 // slash comment\n" +
		"        .ascii \"# literal <&>\"\n" +
		"        end\n"
	iterator, err := assemblyLexer.Tokenise(nil, src)
	if err != nil {
		t.Fatal(err)
	}
	tokens := iterator.Tokens()
	var reconstructed strings.Builder
	for _, token := range tokens {
		reconstructed.WriteString(token.Value)
		if token.Type == chroma.Error {
			t.Errorf("unexpected error token: %q", token.Value)
		}
	}
	if got := reconstructed.String(); got != src {
		t.Fatalf("lexer changed source: got %q, want %q", got, src)
	}
	for _, want := range []struct {
		value string
		kind  chroma.TokenType
	}{
		{"# 1. Host code", chroma.Comment},
		{"#2. Accelerator code", chroma.Comment},
		{"# host comment", chroma.Comment},
		{"# instruction comment", chroma.Comment},
		{"; semicolon comment", chroma.Comment},
		{"// slash comment", chroma.Comment},
		{"#1.25e-06", chroma.LiteralNumber},
		{"#-127", chroma.LiteralNumber},
		{"#0x20", chroma.LiteralNumber},
		{"1.25", chroma.LiteralNumber},
		{"\"# literal <&>\"", chroma.LiteralString},
		{"entry:", chroma.NameLabel},
		{".desc", chroma.NameAttribute},
		{"fnma", chroma.NameFunction},
	} {
		found := false
		for _, token := range tokens {
			matchesType := token.Type == want.kind
			if want.kind == chroma.Comment {
				matchesType = token.Type.InCategory(chroma.Comment)
			} else if want.kind == chroma.LiteralNumber || want.kind == chroma.LiteralString {
				matchesType = token.Type.InSubCategory(want.kind)
			}
			if strings.TrimSuffix(token.Value, "\n") == want.value &&
				matchesType {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing %s token for %q: %v", want.kind, want.value, tokens)
		}
	}
}

func TestServesAssemblyHighlighted(t *testing.T) {
	root := t.TempDir()
	src := "# assembly example <&>\nentry:\n\tfnma v2, v0, #1.25e-06, v1 # comment\n"
	server := fileServer{fsys: os.DirFS(root), pandoc: "unused"}
	for _, name := range []string{"example.asm", "example.ASM", "example.asm~"} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(root, name), []byte(src), 0o644); err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodGet, "/"+name, nil)
			req.Header.Set("Sec-Fetch-Dest", "document")
			rec := httptest.NewRecorder()
			server.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
				t.Fatalf("navigation status=%d Content-Type=%q", rec.Code, rec.Header().Get("Content-Type"))
			}
			for _, want := range []string{`id="L1"`, `href="` + name + `?raw=1"`, "<span style=", "#1.25e-06", "&lt;&amp;&gt;"} {
				if !strings.Contains(rec.Body.String(), want) {
					t.Errorf("rendered assembly is missing %q", want)
				}
			}
			for _, query := range []string{"", "?raw=1"} {
				req := httptest.NewRequest(http.MethodGet, "/"+name+query, nil)
				if query != "" {
					req.Header.Set("Sec-Fetch-Dest", "document")
				}
				rec := httptest.NewRecorder()
				server.ServeHTTP(rec, req)
				if rec.Code != http.StatusOK || rec.Body.String() != src {
					t.Errorf("raw request %q: status=%d body=%q, want original source", query, rec.Code, rec.Body.String())
				}
			}
		})
	}
}
