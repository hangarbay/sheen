package render

import (
	"math"
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
	display   string // "", "inline", "block", "none"
	// max-width/width and auto margins are non-inherited; they drive block
	// centering and are reset at each element together with bg/display.
	maxWidthRaw     string
	marginLeftAuto  bool
	marginRightAuto bool
	displayNone     bool
}

// userAgentCSS is the builtin "browser stylesheet". Colored presets get a
// syntax-highlight-like palette; notty/ascii keep structure only.
func userAgentCSS(p preset, dark bool) string {
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
	if !dark {
		return `
h1{font-weight:bold;color:#ad1457}
h2{font-weight:bold;color:#0277bd}
h3{font-weight:bold;color:#2e7d32}
h4{font-weight:bold;color:#b45309}
h5{font-weight:bold;color:#6a1b9a}
h6{font-weight:bold;color:#455a64}
a{text-decoration:underline;color:#0969da}
code,kbd,samp,var{color:#a31515}
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
code,kbd,samp,var{color:#f8f8f2}
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
		decls := parseDecls(styleAttr)
		if r.ss != nil {
			for i := range decls {
				decls[i].val = r.ss.resolve(decls[i].val)
			}
		}
		applyDecls(st, decls)
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
		case "max-width", "width":
			st.maxWidthRaw = d.val
		case "margin":
			toks := strings.Fields(d.val)
			var left, right string
			switch len(toks) {
			case 1:
				left, right = toks[0], toks[0]
			case 2:
				left, right = toks[1], toks[1]
			case 3:
				left, right = toks[1], toks[1]
			case 4:
				left, right = toks[3], toks[1]
			}
			st.marginLeftAuto = left == "auto"
			st.marginRightAuto = right == "auto"
		case "margin-left":
			st.marginLeftAuto = d.val == "auto"
		case "margin-right":
			st.marginRightAuto = d.val == "auto"
		case "display":
			switch d.val {
			case "none":
				st.displayNone = true
				st.display = "none"
			case "block", "flex", "grid", "table", "list-item", "flow-root":
				st.display = "block"
			case "inline", "inline-block", "inline-flex", "inline-grid", "inline-table":
				st.display = "inline"
			}
		}
	}
}

// cssSizeToCells converts a CSS length to terminal cells. ch and bare
// numbers map 1:1, percentages map against the available width, and px
// values scale against an assumed ~1280px browser viewport so common
// "content column" widths (700-1000px) land in a sensible cell range.
func cssSizeToCells(v string, avail int) int {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" || v == "auto" || v == "none" {
		return 0
	}
	i := 0
	sign := 1.0
	if i < len(v) && (v[i] == '-' || v[i] == '+') {
		if v[i] == '-' {
			sign = -1
		}
		i++
	}
	start := i
	for i < len(v) && (v[i] >= '0' && v[i] <= '9' || v[i] == '.') {
		i++
	}
	f, err := strconv.ParseFloat(v[start:i], 64)
	if err != nil || f <= 0 {
		return 0
	}
	f *= sign
	unit := v[i:]
	var cells float64
	switch {
	case unit == "" || unit == "ch":
		cells = f
	case unit == "%":
		cells = f * float64(avail) / 100
	case unit == "px" || unit == "pt":
		cells = f * float64(avail) / 1280
	case unit == "em" || unit == "rem":
		cells = f * 16 * float64(avail) / 1280
	case unit == "vw":
		cells = f * float64(avail) / 100
	default:
		return 0
	}
	if cells < 1 {
		return 0
	}
	return int(math.Round(cells))
}

// hexLuminance returns perceived luminance (0-1) of a #rrggbb color, or -1
// when the value isn't a simple hex color.
func hexLuminance(hex string) float64 {
	if len(hex) != 7 || hex[0] != '#' {
		return -1
	}
	v, err := strconv.ParseUint(hex[1:], 16, 32)
	if err != nil {
		return -1
	}
	r := float64(v>>16&0xFF) / 255
	g := float64(v>>8&0xFF) / 255
	b := float64(v&0xFF) / 255
	return 0.2126*r + 0.7152*g + 0.0722*b
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
