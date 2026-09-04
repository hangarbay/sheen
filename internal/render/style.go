package render

import (
	"strconv"
	"strings"
)

// preset controls the look of rendered output.
type preset struct {
	name    string // dark, light, notty, ascii
	colored bool
	ascii   bool
}

var presets = map[string]preset{
	"dark":  {name: "dark", colored: true},
	"light": {name: "light", colored: true},
	"notty": {name: "notty"},
	"ascii": {name: "ascii", ascii: true},
}

func (p preset) bullets(depth int) string {
	if p.ascii {
		return []string{"*", "-", "+"}[(depth-1)%3]
	}
	return []string{"•", "◦", "▪"}[(depth-1)%3]
}

func (p preset) hr() string {
	if p.ascii {
		return "-"
	}
	return "─"
}

func (p preset) quote() string {
	if p.ascii {
		return "> "
	}
	return "│ "
}

// styleState is the computed style carried through the DOM walk. Fields
// mirror the CSS properties this renderer understands.
type styleState struct {
	fg, bg    string
	bold      bool
	italic    bool
	underline bool
	strike    bool
	align     string // "", "left", "center", "right"
	pre       bool   // white-space: pre
	nowrap    bool   // white-space: nowrap
	// displayNone and bg are non-inherited; they are reset at each element.
	displayNone bool
}

// userAgentCSS is the builtin "browser stylesheet". Colored presets get a
// syntax-highlight-like palette; notty/ascii keep structure only.
func userAgentCSS(p preset) string {
	if !p.colored {
		return `
h1,h2,h3,h4,h5,h6{font-weight:bold}
a,u,ins{text-decoration:underline}
del,s,strike{text-decoration:line-through}
strong,b,th,dt,summary{font-weight:bold}
em,i,cite,dfn,var,caption,figcaption{font-style:italic}
code,kbd,samp,var{font-style:normal}
center{text-align:center}
`
	}
	if p.name == "light" {
		return `
h1{font-weight:bold;color:#ad1457}
h2{font-weight:bold;color:#0277bd}
h3{font-weight:bold;color:#2e7d32}
h4{font-weight:bold;color:#b45309}
h5{font-weight:bold;color:#6a1b9a}
h6{font-weight:bold;color:#455a64}
a{text-decoration:underline;color:#0969da}
code,kbd,samp,var{color:#a31515;background-color:#eeeeee}
blockquote{color:#57606a}
img{color:#6e7781}
mark{background-color:#fff3b8}
del,s,strike{text-decoration:line-through}
u,ins{text-decoration:underline}
strong,b,th,dt,summary{font-weight:bold}
em,i,cite,dfn,var,caption,figcaption{font-style:italic}
center{text-align:center}
`
	}
	return `
h1{font-weight:bold;color:#ff79c6}
h2{font-weight:bold;color:#8be9fd}
h3{font-weight:bold;color:#50fa7b}
h4{font-weight:bold;color:#f1fa8c}
h5{font-weight:bold;color:#ffb86c}
h6{font-weight:bold;color:#bd93f9}
a{text-decoration:underline;color:#6cb2ff}
code,kbd,samp,var{color:#f8f8f2;background-color:#3a4152}
blockquote{color:#9aa4b2}
img{color:#7d8590}
mark{background-color:#5a4a00}
del,s,strike{text-decoration:line-through}
u,ins{text-decoration:underline}
strong,b,th,dt,summary{font-weight:bold}
em,i,cite,dfn,var,caption,figcaption{font-style:italic}
center{text-align:center}
`
}

// applyCSS computes an element's style from its inherited state plus the
// matching rules (author CSS) and its inline style attribute.
func (r *renderer) applyCSS(st *styleState, info *elemInfo, styleAttr string) {
	stack := make([]*elemInfo, len(r.stack), len(r.stack)+1)
	copy(stack, r.stack)
	stack = append(stack, info)
	for _, rule := range r.ss.rules {
		if matchSelector(rule.sel, stack) {
			applyDecls(st, rule.decls)
		}
	}
	if align := info.alignAttr; align != "" {
		applyDecls(st, []decl{{prop: "text-align", val: align}})
	}
	if styleAttr != "" {
		applyDecls(st, parseDecls(styleAttr))
	}
}

// applyDecls applies a declaration list to a style state. Inherited properties
// overwrite; background and display are treated as non-inherited by the caller
// (bg is reset per element, displayNone only suppresses the current element).
func applyDecls(st *styleState, decls []decl) {
	for _, d := range decls {
		switch d.prop {
		case "color":
			if d.val == "inherit" || d.val == "currentcolor" {
				continue
			}
			if hex := cssColorToHex(d.val); hex != "" {
				st.fg = hex
			}
		case "background-color":
			st.bg = cssColorToHex(d.val)
		case "background":
			if strings.Contains(d.val, "url(") {
				continue
			}
			st.bg = cssColorToHex(d.val)
		case "font-weight":
			st.bold = parseFontWeight(d.val)
		case "font-style":
			st.italic = d.val == "italic" || d.val == "oblique"
		case "font":
			// light shorthand support: style and weight keywords
			if strings.Contains(d.val, "italic") || strings.Contains(d.val, "oblique") {
				st.italic = true
			}
			if strings.Contains(d.val, "bold") {
				st.bold = true
			}
		case "text-decoration", "text-decoration-line":
			parseTextDecoration(st, d.val)
		case "text-align":
			switch d.val {
			case "center":
				st.align = "center"
			case "right":
				st.align = "right"
			case "left", "justify", "start":
				st.align = ""
			case "end":
				st.align = "right"
			}
		case "white-space":
			switch d.val {
			case "pre", "pre-wrap", "break-spaces":
				st.pre = true
				st.nowrap = false
			case "nowrap":
				st.pre = false
				st.nowrap = true
			case "normal":
				st.pre = false
				st.nowrap = false
			}
		case "display":
			st.displayNone = d.val == "none"
		}
	}
}

func parseFontWeight(v string) bool {
	switch v {
	case "bold", "bolder":
		return true
	case "normal", "lighter":
		return false
	}
	if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
		return n >= 600
	}
	return false
}

func parseTextDecoration(st *styleState, v string) {
	words := strings.Fields(strings.ToLower(v))
	for _, w := range words {
		switch w {
		case "none":
			st.underline = false
			st.strike = false
		case "underline":
			st.underline = true
		case "line-through":
			st.strike = true
		}
	}
}
