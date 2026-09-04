package render

import "testing"

func TestCSSColorParsing(t *testing.T) {
	cases := map[string]string{
		"#abc":                  "#aabbcc",
		"#aabbcc":               "#aabbcc",
		"#aabbccdd":             "#aabbcc",
		"#ABC":                  "#aabbcc",
		"rgb(255, 0, 128)":      "#ff0080",
		"rgb(100%, 0%, 0%)":     "#ff0000",
		"rgba(10, 20, 30, 0.5)": "#0a141e",
		"rgb(10 20 30)":         "#0a141e",
		"hsl(120, 100%, 50%)":   "#00ff00",
		"hsl(0 100% 50%)":       "#ff0000",
		"tomato":                "#ff6347",
		"white":                 "#ffffff",
		"transparent":           "",
		"currentColor":          "",
		"url(bg.png)":           "",
		"#xyz":                  "",
	}
	for in, want := range cases {
		if got := cssColorToHex(in); got != want {
			t.Errorf("cssColorToHex(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseDeclsImportrantAndJunk(t *testing.T) {
	decls := parseDecls(" color: red !IMPORTANT ; garbage; font-weight: 700")
	if len(decls) != 2 {
		t.Fatalf("expected 2 decls, got %d: %+v", len(decls), decls)
	}
	if decls[0].prop != "color" || decls[0].val != "red" {
		t.Errorf("!important not stripped: %+v", decls[0])
	}
	if decls[1].prop != "font-weight" || decls[1].val != "700" {
		t.Errorf("got %+v", decls[1])
	}
}

func TestApplyDecls(t *testing.T) {
	st := &styleState{}
	applyDecls(st, parseDecls(
		"font-weight:bold; font-style:italic; text-decoration:underline line-through; "+
			"background-color:#123456; white-space:pre; text-align:center"))
	if !st.bold || !st.italic {
		t.Errorf("weight/style: %+v", st)
	}
	if !st.underline || !st.strike {
		t.Errorf("decoration: %+v", st)
	}
	if st.bg != "#123456" || !st.pre || st.align != "center" {
		t.Errorf("got %+v", st)
	}
	applyDecls(st, parseDecls("text-decoration:none; white-space:nowrap; font-weight:400"))
	if st.underline || st.strike || st.pre || st.bold {
		t.Errorf("reset failed: %+v", st)
	}
	if !st.nowrap {
		t.Errorf("nowrap not set: %+v", st)
	}
}

func TestParseComplexSelector(t *testing.T) {
	ok := map[string]bool{
		"p": true, ".a": true, "#i": true, "*": true,
		"div p": true, "div > p": true, "div>p": true,
		"a:hover":       true,
		"ul li.active":  true,
		"p:first-child": false, "[data-x]": false, "a::before": false,
		"div + p": false, "h1 ~ p": false, "": false, ".a, .b": false,
	}
	for sel, want := range ok {
		if _, got := parseComplexSelector(sel); got != want {
			t.Errorf("parseComplexSelector(%q) ok = %v, want %v", sel, got, want)
		}
	}
}

func TestMatchSelectorCombinators(t *testing.T) {
	mk := func(tag, id string, classes ...string) *elemInfo {
		return &elemInfo{tag: tag, id: id, classes: classes}
	}
	htmlBody := []*elemInfo{mk("html", ""), mk("body", "")}

	divSectionP := append(append([]*elemInfo{}, htmlBody...), mk("div", ""), mk("section", ""), mk("p", ""))
	divP := append(append([]*elemInfo{}, htmlBody...), mk("div", ""), mk("p", ""))

	sel, ok := parseComplexSelector("div > p")
	if !ok {
		t.Fatal("parse failed")
	}
	if !matchSelector(sel, divP) {
		t.Error("div > p should match div>p")
	}
	if matchSelector(sel, divSectionP) {
		t.Error("div > p must not match div>section>p")
	}

	selD, _ := parseComplexSelector("div p")
	if !matchSelector(selD, divSectionP) {
		t.Error("div p should match div>section>p")
	}
	if matchSelector(selD, append(append([]*elemInfo{}, htmlBody...), mk("p", ""))) {
		t.Error("div p must not match bare p")
	}
}

func TestStylesheetCascade(t *testing.T) {
	ss := &stylesheet{}
	ss.add("p { color: #111111; }", 0)      // tag, order 0
	ss.add(".c { color: #222222; }", 10)    // class beats tag
	ss.add("p.c { color: #333333; }", 20)   // tag+class beats class
	ss.add("#i { color: #444444; }", 30)    // id beats all
	ss.add(".late { color: #555555; }", 40) // tie broken by order
	ss.add(".late { color: #666666; }", 41)
	ss.sortRules()

	st := &styleState{}
	stack := []*elemInfo{{tag: "body"}}
	p := &elemInfo{tag: "p", id: "i", classes: []string{"c", "late"}}
	for _, r := range ss.rules {
		if matchSelector(r.sel, append(stack, p)) {
			applyDecls(st, r.decls)
		}
	}
	if st.fg != "#444444" {
		t.Errorf("id specificity should win, got %q", st.fg)
	}

	p2 := &elemInfo{tag: "p", classes: []string{"late"}}
	st2 := &styleState{}
	for _, r := range ss.rules {
		if matchSelector(r.sel, append(stack, p2)) {
			applyDecls(st2, r.decls)
		}
	}
	if st2.fg != "#666666" {
		t.Errorf("later rule should win ties, got %q", st2.fg)
	}
}

func TestStylesheetAtRules(t *testing.T) {
	ss := &stylesheet{}
	ss.add(`
/* comment { display: none } */
@media screen {
  .in-media { display: none; }
}
@keyframes spin { from { x: 1 } to { x: 2 } }
@import url("x.css");
.out { display: none; }
`, 0)

	if len(ss.rules) != 2 {
		t.Fatalf("expected 2 rules (media flattened, at-rules skipped), got %d", len(ss.rules))
	}
	st := &styleState{}
	for _, r := range ss.rules {
		if matchSelector(r.sel, []*elemInfo{{tag: "body"}, {tag: "p", classes: []string{"in-media"}}}) {
			applyDecls(st, r.decls)
		}
	}
	if !st.displayNone {
		t.Error("media query rule should have been flattened in")
	}
}

func TestLegacyAlignAttribute(t *testing.T) {
	r := &renderer{ss: &stylesheet{}, ps: presets["notty"]}
	st := styleState{}
	r.applyCSS(&st, &elemInfo{tag: "div", alignAttr: "center"}, "")
	if st.align != "center" {
		t.Errorf("align attribute not applied: %+v", st)
	}
}
