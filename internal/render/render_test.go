package render

import (
	"net/url"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func renderStr(t *testing.T, html string, opts Options) string {
	t.Helper()
	out, err := Render(strings.NewReader(html), opts)
	if err != nil {
		t.Fatalf("render error: %v", err)
	}
	return out
}

func plainOpts(width int) Options {
	return Options{Width: width, Preset: "notty", Links: "inline"}
}

func TestHeadingsAndParagraphs(t *testing.T) {
	out := renderStr(t, "<h1>Title</h1><p>First para</p><p>Second para</p>", plainOpts(80))
	want := "Title\n\nFirst para\n\nSecond para"
	if got := ansi.Strip(out); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNestedListsAreCompact(t *testing.T) {
	out := renderStr(t,
		"<ul><li>one</li><li>two<ul><li>nested</li></ul></li><li>three</li></ul>",
		plainOpts(80))
	want := "• one\n• two\n  ◦ nested\n• three"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestOrderedListStartAndNumbering(t *testing.T) {
	out := renderStr(t, `<ol start="9"><li>nine</li><li>ten</li></ol>`, plainOpts(80))
	if !strings.Contains(out, " 9.") || !strings.Contains(out, "10.") {
		t.Errorf("expected right-aligned numbers 9 and 10, got %q", out)
	}
}

func TestLinksInlineMode(t *testing.T) {
	out := renderStr(t, `<a href="https://example.com">click</a>`, plainOpts(80))
	if got := ansi.Strip(out); got != "click (https://example.com)" {
		t.Errorf("got %q", got)
	}
}

func TestLinksOSC8(t *testing.T) {
	opts := Options{Width: 80, Preset: "notty", Links: "osc8"}
	out := renderStr(t, `<a href="https://example.com">click</a>`, opts)
	if !strings.Contains(out, "\x1b]8;;https://example.com\x1b\\") {
		t.Errorf("missing OSC 8 sequence in %q", out)
	}
}

func TestJavascriptHrefNotLinked(t *testing.T) {
	out := renderStr(t, `<a href="javascript:alert(1)">x</a>`, plainOpts(80))
	if strings.Contains(out, "javascript:") {
		t.Errorf("javascript: URL leaked: %q", out)
	}
}

func TestLinkRelativeToBaseURL(t *testing.T) {
	out := renderStr(t, `<a href="/docs/page.html">docs</a>`, Options{
		Width: 80, Preset: "notty", Links: "inline",
		BaseURL: mustURL("https://example.com/app/"),
	})
	if !strings.Contains(out, "https://example.com/docs/page.html") {
		t.Errorf("relative URL not resolved: %q", out)
	}
}

func TestTable(t *testing.T) {
	out := renderStr(t,
		"<table><thead><tr><th>Name</th><th>Qty</th></tr></thead>"+
			"<tbody><tr><td>Apples</td><td>4</td></tr></tbody></table>",
		plainOpts(80))
	for _, want := range []string{"Name", "Qty", "Apples", "│", "─"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q:\n%s", want, out)
		}
	}
}

func TestCodeBlockPlainAndColored(t *testing.T) {
	html := `<pre><code class="language-python">import os</code></pre>`
	plain := renderStr(t, html, plainOpts(80))
	if !strings.Contains(plain, "import os") {
		t.Errorf("code text missing: %q", plain)
	}
	colored := renderStr(t, html, Options{Width: 80, Preset: "dark", Links: "inline"})
	if !strings.Contains(colored, "\x1b[") {
		t.Errorf("expected chroma ANSI highlighting, got %q", colored)
	}
}

func TestBlockquote(t *testing.T) {
	out := renderStr(t, "<blockquote><p>wisdom</p></blockquote>", plainOpts(80))
	if out != "│ wisdom" {
		t.Errorf("got %q", out)
	}
	ascii := renderStr(t, "<blockquote><p>wisdom</p></blockquote>",
		Options{Width: 80, Preset: "ascii", Links: "inline"})
	if !strings.HasPrefix(ascii, "> wisdom") {
		t.Errorf("ascii quote got %q", ascii)
	}
}

func TestHorizontalRule(t *testing.T) {
	out := renderStr(t, "<p>a</p><hr><p>b</p>", plainOpts(10))
	if !strings.Contains(out, strings.Repeat("─", 10)) {
		t.Errorf("expected 10-cell rule in %q", out)
	}
}

func TestImageAlt(t *testing.T) {
	out := renderStr(t, `<p><img src="x.png" alt="A cat"></p>`, plainOpts(80))
	if !strings.Contains(out, "[image: A cat]") {
		t.Errorf("got %q", out)
	}
	// decorative images vanish
	out = renderStr(t, `<p>a<img src="x.png" alt="">b</p>`, plainOpts(80))
	if out != "ab" {
		t.Errorf("decorative image got %q", out)
	}
}

func TestEntities(t *testing.T) {
	out := renderStr(t, "<p>&lt;tag&gt; &amp; &#65;</p>", plainOpts(80))
	if out != "<tag> & A" {
		t.Errorf("got %q", out)
	}
}

func TestWrappingRespectsWidth(t *testing.T) {
	words := strings.Repeat("lorem ipsum dolor sit amet ", 10)
	out := renderStr(t, "<p>"+words+"</p>", plainOpts(40))
	for i, line := range strings.Split(out, "\n") {
		if w := ansi.StringWidth(line); w > 40 {
			t.Errorf("line %d too wide (%d): %q", i, w, line)
		}
	}
}

func TestLongWordHardSplit(t *testing.T) {
	out := renderStr(t, "<p>"+strings.Repeat("x", 100)+"</p>", plainOpts(30))
	for i, line := range strings.Split(out, "\n") {
		if w := ansi.StringWidth(line); w > 30 {
			t.Errorf("line %d too wide (%d)", i, w)
		}
	}
}

func TestScriptSkippedAndNoScriptRendered(t *testing.T) {
	out := renderStr(t,
		`<p>before</p><script>alert("<b>evil</b>");</script><p>after</p>`+
			`<noscript>no js</noscript>`,
		plainOpts(80))
	if strings.Contains(out, "alert") || strings.Contains(out, "evil") {
		t.Errorf("script content leaked: %q", out)
	}
	if !strings.Contains(out, "no js") {
		t.Errorf("noscript content should render: %q", out)
	}
}

func TestTemplateAndSVGSkipped(t *testing.T) {
	out := renderStr(t, `<template><p>tpl</p></template><svg><circle/></svg><p>ok</p>`, plainOpts(80))
	if strings.Contains(out, "tpl") || strings.Contains(out, "circle") {
		t.Errorf("got %q", out)
	}
}

func TestTitleFallbackWhenNoH1(t *testing.T) {
	out := renderStr(t, "<html><head><title>Doc Title</title></head><body><p>hi</p></body></html>",
		plainOpts(80))
	if got := ansi.Strip(out); got != "Doc Title\n\nhi" {
		t.Errorf("got %q", got)
	}
}

func TestCSSColorsReachRender(t *testing.T) {
	r := &renderer{opts: Options{Width: 80}, ps: presets["dark"], links: "inline"}
	r.ss = &stylesheet{}
	r.ss.add(userAgentCSS(r.ps), 0)
	r.ss.add(`.warn { color: #ff0000; }`, 1000)
	r.ss.sortRules()
	st := styleState{}
	r.applyCSS(&st, &elemInfo{tag: "p", classes: []string{"warn"}}, "")
	if st.fg != "#ff0000" {
		t.Errorf("expected #ff0000, got %q", st.fg)
	}
}

func TestCSSDisplayNone(t *testing.T) {
	out := renderStr(t,
		`<style>.hide{display:none}#gone{display:none}</style>`+
			`<p class="hide">a</p><p id="gone">b</p><p>c</p>`,
		plainOpts(80))
	if out != "c" {
		t.Errorf("got %q", out)
	}
}

func TestInlineStyleAttrWins(t *testing.T) {
	r := &renderer{opts: Options{Width: 80}, ps: presets["dark"], links: "inline"}
	r.ss = &stylesheet{}
	r.ss.add(userAgentCSS(r.ps), 0)
	r.ss.add(`p{color:#ff0000}`, 1000)
	r.ss.sortRules()
	st := styleState{}
	r.applyCSS(&st, &elemInfo{tag: "p"}, "color:#00ff00")
	if st.fg != "#00ff00" {
		t.Errorf("inline style should win, got %q", st.fg)
	}
}

func TestCSSTextAlignCenter(t *testing.T) {
	out := renderStr(t, `<p style="text-align:center">hi</p>`, plainOpts(10))
	if out != strings.Repeat(" ", 4)+"hi" {
		t.Errorf("got %q", out)
	}
}

func TestCSSWhiteSpacePre(t *testing.T) {
	out := renderStr(t, `<p style="white-space:pre">a   b
c</p>`, plainOpts(80))
	if !strings.Contains(out, "a   b") || !strings.Contains(out, "\nc") {
		t.Errorf("got %q", out)
	}
}

func TestHiddenAttributes(t *testing.T) {
	out := renderStr(t, `<p hidden>gone</p><p aria-hidden="true">gone2</p><p>stay</p>`, plainOpts(80))
	if out != "stay" {
		t.Errorf("got %q", out)
	}
}

func TestMediaPrintStylesIgnored(t *testing.T) {
	out := renderStr(t,
		`<style media="print">.x{display:none}</style><style>.x{color:#ff0000}</style>`+
			`<p class="x">visible</p>`,
		plainOpts(80))
	if out != "visible" {
		t.Errorf("print media rule was applied: %q", out)
	}
}

func TestDescendantAndChildSelectors(t *testing.T) {
	html := `<style>div p{display:none} div > span{display:none}</style>` +
		`<div><section><p>deep</p></section><span>child</span></div><p>outer</p>`
	out := renderStr(t, html, plainOpts(80))
	if out != "outer" {
		t.Errorf("expected only 'outer' to survive: %q", out)
	}
}

func TestWhitespaceCollapsing(t *testing.T) {
	out := renderStr(t, "<p>\n  a\n\n   b  </p>", plainOpts(80))
	if out != "a b" {
		t.Errorf("got %q", out)
	}
}

func TestHardBreak(t *testing.T) {
	out := renderStr(t, "<p>one<br>two</p>", plainOpts(80))
	if out != "one\ntwo" {
		t.Errorf("got %q", out)
	}
}

func TestDefinitionList(t *testing.T) {
	out := renderStr(t, "<dl><dt>term</dt><dd>def</dd></dl>", plainOpts(80))
	want := "term\n  def"
	if got := ansi.Strip(out); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func mustURL(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}
