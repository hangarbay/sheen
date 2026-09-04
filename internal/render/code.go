package render

import (
	"bytes"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"golang.org/x/net/html"
)

func (r *renderer) renderPre(n *html.Node, st styleState) {
	r.endBlock()
	raw := getTextPreserve(n)
	raw = strings.TrimPrefix(raw, "\n")
	raw = strings.TrimRight(raw, "\n")

	lang := codeLanguage(n)
	var lines []string
	if r.ps.colored && lang != "" {
		lines = highlight(raw, lang, r.ps.name)
	}
	if lines == nil {
		lines = strings.Split(raw, "\n")
		cst := st
		cst.underline = false
		cst.strike = false
		if r.ps.colored {
			// fg only: the page/block background shows through so code
			// blocks stay coherent with the document's own theme
			cst.fg = codeFg(r.uaDark)
		}
		for i, ln := range lines {
			lines[i] = r.para.indent2 + r.paint(strings.TrimRight(ln, " \t"), cst, "")
		}
	} else {
		for i, ln := range lines {
			lines[i] = r.para.indent2 + strings.TrimRight(ln, " \t")
		}
	}
	r.out = append(r.out, lines...)
	r.endBlock()
}

func codeFg(dark bool) string {
	if !dark {
		return "#a31515"
	}
	return "#f8f8f2"
}

func firstElement(n *html.Node, tag string) *html.Node {
	if n == nil {
		return nil
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == tag {
			return c
		}
	}
	return nil
}

func codeLanguage(pre *html.Node) string {
	nodes := []*html.Node{pre, firstElement(pre, "code")}
	for _, n := range nodes {
		if n == nil {
			continue
		}
		for _, c := range strings.Fields(attrValue(n, "class")) {
			switch {
			case strings.HasPrefix(c, "language-"):
				return strings.TrimPrefix(c, "language-")
			case strings.HasPrefix(c, "lang-"):
				return strings.TrimPrefix(c, "lang-")
			}
		}
	}
	return ""
}

func highlight(raw, lang, presetName string) []string {
	lexer := lexers.Get(lang)
	if lexer == nil {
		return nil
	}
	styleName := "monokai"
	if presetName == "light" {
		styleName = "friendly"
	}
	sty := styles.Get(styleName)
	formatter := formatters.Get("terminal256")
	if sty == nil || formatter == nil {
		return nil
	}
	lexer = chroma.Coalesce(lexer)
	it, err := lexer.Tokenise(nil, raw)
	if err != nil {
		return nil
	}
	var buf bytes.Buffer
	if err := formatter.Format(&buf, sty, it); err != nil {
		return nil
	}
	out := strings.TrimSuffix(buf.String(), "\n")
	if out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}
