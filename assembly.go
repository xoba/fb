package main

import (
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// Assembly files can mix CPU instructions and custom accelerator dialects.
// GAS already recognizes arbitrary mnemonics, labels, and dot directives.
// Extend a private copy of its rules for numeric # operands (which otherwise
// become comments), scientific notation, registers, and descriptor expressions.
// The .s lexer and Chroma's global registry keep their original behavior.
var assemblyLexer = chroma.MustNewLexer(&chroma.Config{Name: "Assembly"}, func() chroma.Rules {
	rules := lexers.Get("gas").(*chroma.RegexLexer).MustRules().Clone()
	// Trailing spaces must not consume the newline that ends an instruction.
	for i, rule := range rules["whitespace"] {
		if rule.Pattern == `\s+` {
			rules["whitespace"][i].Pattern = `[^\S\r\n]+`
		}
	}
	operands := []chroma.Rule{
		{Pattern: `"(?:\\.|[^"\\\r\n])*"|'(?:\\.|[^'\\\r\n])*'`, Type: chroma.LiteralString},
		// A hash directly before a number denotes an immediate, while '# text'
		// and hashes at the start of a line remain comments.
		{Pattern: `#?[+-]?(?:0[xX][0-9a-fA-F]+|0[bB][01]+|(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?)(?![\w.])`, Type: chroma.LiteralNumber},
		{Pattern: `(?i:\b(?:[xfv](?:[12]?\d|3[01])|s(?:[0-9]|1[01])|t[0-6]|a[0-7]|zero|ra|sp|gp|tp|fp|imm[0-3]|(?:in|out)[0-2])\b)`, Type: chroma.NameBuiltin},
		{Pattern: `/(?![/*])|[=+<>&^~|]+`, Type: chroma.Operator},
	}
	for _, state := range []string{"instruction-args", "directive-args"} {
		rules[state] = append(append([]chroma.Rule(nil), operands...), rules[state]...)
	}
	return rules
})
