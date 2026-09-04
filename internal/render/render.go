package render

import (
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	"golang.org/x/net/html"
)

// Options controls how HTML is rendered.
type Options struct {
	Width   int    // wrap width in terminal cells (default 80)
	Preset  string // dark, light, notty, ascii
	BaseURL *url.URL
	Links   string // osc8, inline, none
	// FetchStylesheet loads <link rel="stylesheet"> targets. Returning an
	// error or leaving nil simply skips that stylesheet.
	FetchStylesheet func(rawurl string) ([]byte, error)
}

// Render parses HTML and returns terminal-formatted text.
func Render(r io.Reader, opts Options) (string, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return "", err
	}
	if opts.Width <= 0 {
		opts.Width = 80
	}
	switch opts.Preset {
	case "dark", "light", "notty", "ascii":
	case "":
		opts.Preset = "dark"
	default:
		opts.Preset = "dark"
	}
	switch opts.Links {
	case "osc8", "inline", "none":
	default:
		opts.Links = "osc8"
	}
	rr := &renderer{opts: opts, ps: presets[opts.Preset], links: opts.Links}
	return rr.renderDocument(doc), nil
}

type seg struct {
	text string
	st   styleState
	href string
}

type para struct {
	indent1, indent2 string
	segs             []seg
	align            string
	pre              bool
	nowrap           bool
}

type renderer struct {
	opts  Options
	ps    preset
	ss    *stylesheet
	out   []string
	para  para
	stack []*elemInfo
	st    styleState
	href  string // current <a href> while walking children
	links string

	listDepth   int
	listIndex   int
	listOrdered bool
	listNumW    int
	listBase    string // indentation context for the current list's items

	title string
}

var skipElements = map[string]bool{
	"script": true, "style": true, "template": true, "iframe": true,
	"object": true, "embed": true, "applet": true, "param": true,
	"svg": true, "math": true, "select": true, "option": true,
	"datalist": true, "textarea": true, "input": true, "source": true,
	"track": true, "audio": true, "video": true, "map": true, "area": true,
	"meta": true, "link": true, "base": true, "head": true, "title": true,
	"canvas": true,
}

func (r *renderer) renderDocument(doc *html.Node) string {
	var css []string
	r.collect(doc, &css)

	r.ss = &stylesheet{}
	order := r.ss.add(userAgentCSS(r.ps), 0)
	for _, chunk := range css {
		order = r.ss.add(chunk, order)
	}
	r.ss.sortRules()

	body := findElement(doc, "body")
	if body != nil {
		if !hasElement(body, "h1") && r.title != "" {
			r.titleHeading()
		}
		info := newElemInfo(body)
		st := styleState{}
		r.applyCSS(&st, info, attrValue(body, "style"))
		if !st.displayNone {
			r.stack = append(r.stack, info)
			r.st = st
			r.walkChildren(body)
			r.flushInline()
			r.stack = r.stack[:len(r.stack)-1]
		}
	}
	return finalize(r.out)
}

// collect harvests the document title and author CSS (style elements plus
// fetched stylesheets) before rendering. Script subtrees are never touched.
func (r *renderer) collect(n *html.Node, css *[]string) {
	if n.Type != html.ElementNode {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			r.collect(c, css)
		}
		return
	}
	switch n.Data {
	case "script", "template":
		return
	case "title":
		if r.title == "" {
			r.title = strings.TrimSpace(getText(n))
		}
		return
	case "style":
		if !mediaPrint(attrValue(n, "media")) {
			*css = append(*css, getText(n))
		}
		return
	case "link":
		rel := strings.ToLower(attrValue(n, "rel"))
		href := attrValue(n, "href")
		if strings.Contains(rel, "stylesheet") && href != "" &&
			!mediaPrint(attrValue(n, "media")) && r.opts.FetchStylesheet != nil {
			if b, err := r.opts.FetchStylesheet(r.resolveLink(href)); err == nil {
				*css = append(*css, string(b))
			}
		}
		return
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		r.collect(c, css)
	}
}

func mediaPrint(media string) bool {
	return strings.Contains(strings.ToLower(media), "print")
}

func (r *renderer) walkChildren(n *html.Node) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		r.walk(c)
	}
}

func (r *renderer) walk(n *html.Node) {
	switch n.Type {
	case html.TextNode:
		r.appendText(n.Data)
	case html.ElementNode:
		r.element(n)
	case html.DocumentNode:
		r.walkChildren(n)
	}
}

func (r *renderer) element(n *html.Node) {
	info := newElemInfo(n)
	parentSt := r.st
	st := parentSt
	st.displayNone = false
	st.bg = "" // background-color is not inherited
	r.applyCSS(&st, info, attrValue(n, "style"))

	if st.displayNone || hasAttr(n, "hidden") ||
		attrValue(n, "aria-hidden") == "true" {
		return
	}
	if skipElements[n.Data] {
		return
	}

	r.stack = append(r.stack, info)
	r.st = st
	r.dispatch(n, st)
	r.st = parentSt
	r.stack = r.stack[:len(r.stack)-1]
}

func (r *renderer) dispatch(n *html.Node, st styleState) {
	switch n.Data {
	case "br":
		r.para.segs = append(r.para.segs, seg{text: "\n", st: st, href: r.href})

	case "wbr":
		// soft break opportunity: nothing to do

	case "hr":
		r.endBlock()
		line := strings.Repeat(r.ps.hr(), r.opts.Width)
		if r.ps.colored {
			line = lipStyle{fg: "#5c6370"}.paintPlain(line)
		}
		r.out = append(r.out, line)
		r.endBlock()

	case "h1", "h2", "h3", "h4", "h5", "h6":
		r.endBlock()
		r.startPara(st)
		r.walkChildren(n)
		r.flushInline()
		r.endBlock()

	case "p", "figcaption", "caption", "summary", "address":
		r.endBlock()
		r.startPara(st)
		r.walkChildren(n)
		r.flushInline()
		r.endBlock()

	case "dt":
		r.flushInline()
		r.startPara(st)
		r.walkChildren(n)
		r.flushInline()

	case "dd":
		r.flushInline()
		r.para.indent1 = "  "
		r.para.indent2 = "  "
		r.startPara(st)
		r.walkChildren(n)
		r.flushInline()
		r.para.indent1 = ""
		r.para.indent2 = ""

	case "li":
		r.flushInline()
		marker := r.listMarker()
		base := r.listBase
		r.para.indent1 = base + marker + " "
		r.para.indent2 = base + strings.Repeat(" ", ansi.StringWidth(marker)+1)
		r.startPara(st)
		r.walkChildren(n)
		r.flushInline()

	case "ul", "ol":
		r.flushInline()
		pDepth, pIndex, pOrdered, pNumW, pBase := r.listDepth, r.listIndex, r.listOrdered, r.listNumW, r.listBase
		pI1, pI2 := r.para.indent1, r.para.indent2
		r.listDepth = pDepth + 1
		r.listOrdered = n.Data == "ol"
		r.listBase = pI2
		start := 1
		if r.listOrdered {
			if v, err := strconv.Atoi(attrValue(n, "start")); err == nil {
				start = v
			}
			r.listNumW = len(strconv.Itoa(start + countChildren(n, "li") - 1))
		}
		r.listIndex = start - 1
		r.walkChildren(n)
		r.flushInline()
		r.para.indent1, r.para.indent2 = pI1, pI2
		r.listDepth, r.listIndex, r.listOrdered, r.listNumW, r.listBase = pDepth, pIndex, pOrdered, pNumW, pBase
		r.flushInline()

	case "blockquote":
		r.endBlock()
		mark := len(r.out)
		r.startPara(st)
		r.walkChildren(n)
		r.flushInline()
		r.emitBlockquote(mark, st)
		r.endBlock()

	case "pre":
		r.renderPre(n, st)

	case "table":
		r.endBlock()
		r.renderTable(n, st)
		r.endBlock()

	case "a":
		href := r.resolveLink(attrValue(n, "href"))
		prev := r.href
		r.href = href
		r.walkChildren(n)
		r.href = prev
		if href != "" && r.links == "inline" {
			st2 := st
			st2.underline = false
			if r.ps.colored {
				st2.fg = "#7d8590"
			}
			r.para.segs = append(r.para.segs, seg{text: " (" + href + ")", st: st2})
		}

	case "img":
		r.renderImage(n, st)

	case "q":
		r.para.segs = append(r.para.segs, seg{text: "“", st: st, href: r.href})
		r.walkChildren(n)
		r.para.segs = append(r.para.segs, seg{text: "”", st: st, href: r.href})

	default:
		r.walkChildren(n)
	}
}

func (r *renderer) listMarker() string {
	r.listIndex++
	if r.listOrdered {
		return fmt.Sprintf("%*s.", r.listNumW, strconv.Itoa(r.listIndex))
	}
	return r.ps.bullets(r.listDepth)
}

func (r *renderer) startPara(st styleState) {
	r.para.align = st.align
	r.para.pre = st.pre
	r.para.nowrap = st.nowrap
}

func (r *renderer) titleHeading() {
	st := styleState{}
	r.applyCSS(&st, &elemInfo{tag: "h1"}, "")
	prev := r.st
	r.st = st
	r.appendText(r.title)
	r.flushInline()
	r.endBlock()
	r.st = prev
}

func (r *renderer) emitBlockquote(mark int, st styleState) {
	lines := r.out[mark:]
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	r.out = r.out[:mark]
	glyph := strings.TrimRight(r.ps.quote(), " ")
	if r.ps.colored && st.fg != "" {
		glyph = lipStyle{fg: st.fg}.paintPlain(glyph)
	}
	for _, line := range lines {
		if line == "" {
			r.out = append(r.out, glyph)
		} else {
			r.out = append(r.out, glyph+" "+line)
		}
	}
}

func (r *renderer) renderImage(n *html.Node, st styleState) {
	alt := strings.TrimSpace(attrValue(n, "alt"))
	src := attrValue(n, "src")
	if alt == "" && src == "" {
		return
	}
	if alt == "" {
		return // decorative image
	}
	label := "[image: " + alt + "]"
	r.para.segs = append(r.para.segs, seg{text: label, st: st, href: r.href})
	if resolved := r.resolveLink(src); resolved != "" && r.links == "inline" {
		st2 := st
		st2.italic = false
		if r.ps.colored {
			st2.fg = "#7d8590"
		}
		r.para.segs = append(r.para.segs, seg{text: " (" + resolved + ")", st: st2})
	}
}

func (r *renderer) resolveLink(href string) string {
	if href == "" {
		return ""
	}
	u, err := url.Parse(href)
	if err != nil {
		return ""
	}
	if u.Scheme == "javascript" || u.Scheme == "data" && !strings.HasPrefix(u.Opaque, "image") {
		return ""
	}
	if !u.IsAbs() && r.opts.BaseURL != nil {
		return r.opts.BaseURL.ResolveReference(u).String()
	}
	return href
}

func (r *renderer) appendText(s string) {
	if s == "" {
		return
	}
	st := r.st
	if r.para.pre {
		r.para.segs = append(r.para.segs, seg{text: s, st: st, href: r.href})
		return
	}
	s = collapseSpace(s)
	if s == "" {
		return
	}
	if s == " " {
		if len(r.para.segs) == 0 || strings.HasSuffix(r.para.segs[len(r.para.segs)-1].text, " ") {
			return
		}
	}
	if strings.HasPrefix(s, " ") {
		if len(r.para.segs) == 0 || strings.HasSuffix(r.para.segs[len(r.para.segs)-1].text, " ") {
			s = s[1:]
			if s == "" {
				return
			}
		}
	}
	if last := len(r.para.segs) - 1; last >= 0 && r.para.segs[last].text != "\n" &&
		r.para.segs[last].href == r.href && sameStyle(r.para.segs[last].st, st) {
		r.para.segs[last].text += s
		return
	}
	r.para.segs = append(r.para.segs, seg{text: s, st: st, href: r.href})
}

func (r *renderer) endBlock() {
	r.flushInline()
	if len(r.out) > 0 && r.out[len(r.out)-1] != "" {
		r.out = append(r.out, "")
	}
}

func (r *renderer) flushInline() {
	if len(r.para.segs) == 0 {
		return
	}
	switch {
	case r.para.pre:
		r.flushPre()
	case r.para.nowrap:
		r.flushNoWrap()
	default:
		r.wrapPara()
	}
	r.para.segs = nil
}

func finalize(out []string) string {
	for len(out) > 0 && out[0] == "" {
		out = out[1:]
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n")
}

func collapseSpace(s string) string {
	var b strings.Builder
	prevSpace := false
	for i := 0; i < len(s); {
		rn, sz := utf8.DecodeRuneInString(s[i:])
		if unicode.IsSpace(rn) {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
			i += sz
			continue
		}
		b.WriteRune(rn)
		prevSpace = false
		i += sz
	}
	return b.String()
}

func sameStyle(a, b styleState) bool {
	return a.fg == b.fg && a.bg == b.bg && a.bold == b.bold && a.italic == b.italic &&
		a.underline == b.underline && a.strike == b.strike
}

func newElemInfo(n *html.Node) *elemInfo {
	info := &elemInfo{
		tag:       strings.ToLower(n.Data),
		id:        strings.ToLower(attrValue(n, "id")),
		alignAttr: strings.ToLower(strings.TrimSpace(attrValue(n, "align"))),
	}
	for _, c := range strings.Fields(attrValue(n, "class")) {
		info.classes = append(info.classes, strings.ToLower(c))
	}
	return info
}

func attrValue(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func hasAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}

func findElement(n *html.Node, tag string) *html.Node {
	if n.Type == html.ElementNode && n.Data == tag {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findElement(c, tag); found != nil {
			return found
		}
	}
	return nil
}

func hasElement(n *html.Node, tag string) bool {
	if n.Type == html.ElementNode && n.Data == tag {
		return true
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if hasElement(c, tag) {
			return true
		}
	}
	return false
}

func getText(n *html.Node) string {
	var b strings.Builder
	var rec func(*html.Node)
	rec = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
			return
		}
		if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "template") {
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			rec(c)
		}
	}
	rec(n)
	return b.String()
}

func getTextPreserve(n *html.Node) string {
	var b strings.Builder
	var rec func(*html.Node)
	rec = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
			return
		}
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style", "template":
				return
			case "br":
				b.WriteString("\n")
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			rec(c)
		}
	}
	rec(n)
	return b.String()
}

func countChildren(n *html.Node, tag string) int {
	count := 0
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == tag {
			count++
		}
	}
	return count
}
