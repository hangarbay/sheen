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
	r.ss.add(userAgentCSS(r.ps, true), 0, false)
	r.ss.add(`.warn { color: #ff0000; }`, 1000, false)
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
	r.ss.add(userAgentCSS(r.ps, true), 0, false)
	r.ss.add(`p{color:#ff0000}`, 1000, false)
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

func TestCSSVarResolution(t *testing.T) {
	r := &renderer{opts: Options{Width: 80}, ps: presets["dark"], links: "inline"}
	r.ss = &stylesheet{}
	r.ss.add(userAgentCSS(r.ps, true), 0, false)
	r.ss.add(`:root { --accent: #FFB000; } .e { color: var(--accent); }`, 1000, false)
	r.ss.resolveVars(true)
	st := styleState{}
	r.applyCSS(&st, &elemInfo{tag: "p", classes: []string{"e"}}, "")
	if st.fg != "#ffb000" {
		t.Errorf("var() not resolved, fg=%q", st.fg)
	}
}

func TestRootNotSelectorTolerated(t *testing.T) {
	r := &renderer{opts: Options{Width: 80}, ps: presets["dark"], links: "inline"}
	r.ss = &stylesheet{}
	r.ss.add(userAgentCSS(r.ps, true), 0, false)
	r.ss.add(`:root:not([data-theme="light"]) { --bg: #0C1116; } body { background-color: var(--bg); }`, 1000, false)
	r.ss.resolveVars(true)
	st := styleState{}
	r.applyCSS(&st, &elemInfo{tag: "body"}, "")
	if st.bg != "#0c1116" {
		t.Errorf(":root:not(...) rule dropped, bg=%q", st.bg)
	}
}

func TestBlockCenteringCh(t *testing.T) {
	out := renderStr(t,
		`<style>.w{max-width:20ch;margin:0 auto}</style><div class="w"><p>hello world</p></div>`,
		plainOpts(40))
	// 40-wide terminal, 20-cell column → 10 cells of left padding
	if !strings.HasPrefix(out, strings.Repeat(" ", 10)+"hello world") {
		t.Errorf("expected centered column, got %q", out)
	}
}

func TestBlockPercentWidth(t *testing.T) {
	out := renderStr(t,
		`<style>.w{width:50%;margin:0 auto}</style><div class="w"><p>hi</p></div>`,
		plainOpts(40))
	if !strings.HasPrefix(out, strings.Repeat(" ", 10)+"hi") {
		t.Errorf("got %q", out)
	}
}

func TestPxViewportHeuristic(t *testing.T) {
	out := renderStr(t,
		`<style>.w{max-width:640px;margin:0 auto}</style><div class="w"><p>x</p></div>`,
		plainOpts(128))
	// 640px of an assumed 1280px viewport → 64 cells → 32 left
	if !strings.HasPrefix(out, strings.Repeat(" ", 32)+"x") {
		t.Errorf("got %q", out)
	}
}

func TestMarginLeftAutoRightAligns(t *testing.T) {
	out := renderStr(t,
		`<style>.r{max-width:10ch;margin-left:auto}</style><div class="r"><p>x</p></div>`,
		plainOpts(30))
	if !strings.HasPrefix(out, strings.Repeat(" ", 20)+"x") {
		t.Errorf("got %q", out)
	}
}

func TestPageBackgroundSheet(t *testing.T) {
	out := renderStr(t,
		`<style>body{background-color:#FFFFFF}</style><p>hi</p>`,
		Options{Width: 20, Preset: "dark", Links: "inline"})
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		if w := ansi.StringWidth(line); w != 20 {
			t.Errorf("line %d width %d, want 20 (sheet fill): %q", i, w, line)
		}
		if !strings.Contains(line, "\x1b[48;2;255;255;255m") {
			t.Errorf("line %d missing white background: %q", i, line)
		}
	}
}

func TestBlockBackgroundBand(t *testing.T) {
	out := renderStr(t,
		`<style>body{background-color:#10151C}.band{background-color:#333333}</style><div class="band">x</div>`,
		Options{Width: 16, Preset: "dark", Links: "inline"})
	lines := strings.Split(out, "\n")
	if len(lines) != 1 {
		t.Fatalf("want one band line, got %q", out)
	}
	if w := ansi.StringWidth(lines[0]); w != 16 {
		t.Errorf("band width %d, want 16", w)
	}
	if !strings.Contains(lines[0], "\x1b[48;2;51;51;51m") {
		t.Errorf("band missing #333 background: %q", lines[0])
	}
}

func TestTableCellBackgroundPadding(t *testing.T) {
	out := renderStr(t,
		`<style>td.hot{background-color:#FF0000}</style>`+
			`<table><tr><td class="hot">a</td><td>b</td></tr></table>`,
		Options{Width: 40, Preset: "dark", Links: "inline"})
	if !strings.Contains(out, "\x1b[48;2;255;0;0m") {
		t.Errorf("cell background missing: %q", out)
	}
	// the a cell is padded well past its 1-cell content, and the padding
	// carries the cell background (solid band, not a single red letter)
	red := strings.Count(out, "\x1b[48;2;255;0;0m")
	if red < 2 {
		t.Errorf("expected text + padding bg runs, got %d in %q", red, out)
	}
}

func TestBandCoversListIndents(t *testing.T) {
	out := renderStr(t,
		`<style>body{background-color:#10151C}.band{background-color:#333333}</style>`+
			`<div class="band"><ul><li>one</li><li>two</li></ul></div>`,
		Options{Width: 30, Preset: "dark", Links: "inline"})
	lines := strings.Split(out, "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 list lines, got %d: %q", len(lines), out)
	}
	for i, line := range lines {
		if w := ansi.StringWidth(line); w != 30 {
			t.Errorf("line %d width %d, want full band width 30", i, w)
		}
		// the bullet/indent prefix itself must carry the band background
		if !strings.HasPrefix(line, "\x1b[48;2;51;51;51m") {
			t.Errorf("line %d indent not underlaid with band bg: %q", i, line)
		}
	}
}
