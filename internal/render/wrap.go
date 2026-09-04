package render

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// lipStyle paints standalone strings (rules, borders, prefixes) outside the
// word-wrap pipeline.
type lipStyle struct {
	fg, bg    string
	bold      bool
	italic    bool
	underline bool
	strike    bool
}

func (s lipStyle) paintPlain(text string) string {
	ls := lipgloss.NewStyle()
	if s.fg != "" {
		ls = ls.Foreground(lipgloss.Color(s.fg))
	}
	if s.bg != "" {
		ls = ls.Background(lipgloss.Color(s.bg))
	}
	if s.bold {
		ls = ls.Bold(true)
	}
	if s.italic {
		ls = ls.Italic(true)
	}
	if s.underline {
		ls = ls.Underline(true)
	}
	if s.strike {
		ls = ls.Strikethrough(true)
	}
	return ls.Render(text)
}

// paint applies the resolved style of a segment, plus an OSC 8 hyperlink when
// the segment belongs to a link. Whitespace-only text is never styled: there
// is nothing to see and empty escape runs just add noise.
func (r *renderer) paint(text string, st styleState, href string) string {
	if text == "" {
		return ""
	}
	if strings.TrimSpace(text) == "" {
		return text
	}
	ls := lipStyle{fg: st.fg, bg: st.bg, bold: st.bold, italic: st.italic,
		underline: st.underline, strike: st.strike}
	if !r.ps.colored {
		ls.fg = ""
		ls.bg = ""
	}
	s := ls.paintPlain(text)
	if href != "" && r.links == "osc8" {
		s = "\x1b]8;;" + href + "\x1b\\" + s + "\x1b]8;;\x1b\\"
	}
	return s
}

const (
	wiWord = iota
	wiBreak
	wiSpace
)

type witem struct {
	kind int
	text string
	st   styleState
	href string
	w    int
	glue bool // attach to the previous word without an intervening space
}

type wline struct {
	indent string
	avail  int
	items  []witem
	cells  int
}

// buildItems splits paragraph segments into words, breaks, and space
// separators. Adjacent segments without whitespace produce glued words.
func buildItems(segs []seg) []witem {
	var items []witem
	prevWasSpace := false
	for _, sg := range segs {
		if sg.text == "\n" {
			items = append(items, witem{kind: wiBreak})
			prevWasSpace = false
			continue
		}
		i := 0
		for i < len(sg.text) {
			rn, sz := utf8.DecodeRuneInString(sg.text[i:])
			if unicode.IsSpace(rn) {
				prevWasSpace = true
				i += sz
				for i < len(sg.text) {
					r2, s2 := utf8.DecodeRuneInString(sg.text[i:])
					if !unicode.IsSpace(r2) {
						break
					}
					i += s2
				}
				continue
			}
			j := i
			for j < len(sg.text) {
				r2, s2 := utf8.DecodeRuneInString(sg.text[j:])
				if unicode.IsSpace(r2) {
					break
				}
				j += s2
			}
			word := sg.text[i:j]
			glue := !prevWasSpace && len(items) > 0 && items[len(items)-1].kind == wiWord
			items = append(items, witem{
				kind: wiWord, text: word, st: sg.st, href: sg.href,
				w:    ansi.StringWidth(word),
				glue: glue,
			})
			prevWasSpace = false
			i = j
		}
	}
	return items
}

// flushPre emits preserved-whitespace content: raw newlines, no wrapping.
func (r *renderer) flushPre() {
	p := r.para
	var b strings.Builder
	emit := func() {
		line := strings.TrimRight(b.String(), " \t")
		if p.indent1 != "" || p.indent2 != "" {
			r.out = append(r.out, r.bgPaint(p.indent2)+line)
		} else {
			r.out = append(r.out, line)
		}
		b.Reset()
	}
	for _, sg := range p.segs {
		for {
			i := strings.IndexByte(sg.text, '\n')
			if i < 0 {
				break
			}
			b.WriteString(r.paint(sg.text[:i], sg.st, sg.href))
			emit()
			sg.text = sg.text[i+1:]
		}
		b.WriteString(r.paint(sg.text, sg.st, sg.href))
	}
	emit()
}

// flushNoWrap emits the paragraph without wrapping; explicit breaks still
// produce line breaks.
func (r *renderer) flushNoWrap() {
	p := r.para
	emit := func(items []witem) {
		if len(items) == 0 {
			return
		}
		r.out = append(r.out, r.bgPaint(p.indent1)+r.emitItems(items))
	}
	var cur []witem
	for _, it := range buildItems(r.para.segs) {
		if it.kind == wiBreak {
			emit(cur)
			cur = cur[:0]
			continue
		}
		if len(cur) > 0 && !it.glue {
			cur = append(cur, witem{kind: wiSpace, text: " ", st: prevStyle(cur), href: prevHref(cur)})
		}
		cur = append(cur, it)
	}
	emit(cur)
}

// emitItems paints a line's items, coalescing adjacent items that share the
// same style and hyperlink into single escape runs.
func (r *renderer) emitItems(items []witem) string {
	var b strings.Builder
	var run []witem
	flushRun := func() {
		if len(run) == 0 {
			return
		}
		text := make([]string, len(run))
		for i, it := range run {
			text[i] = it.text
		}
		b.WriteString(r.paint(strings.Join(text, ""), run[0].st, run[0].href))
		run = run[:0]
	}
	for _, it := range items {
		join := len(run) > 0 && it.kind == wiWord && run[0].kind == wiWord &&
			sameStyle(run[0].st, it.st) && run[0].href == it.href
		if it.kind == wiSpace && len(run) > 0 && sameStyle(run[0].st, it.st) &&
			run[0].href == it.href {
			join = true
		}
		if join {
			run = append(run, it)
			continue
		}
		flushRun()
		switch it.kind {
		case wiSpace:
			b.WriteString(r.paint(" ", it.st, it.href))
		default:
			run = append(run, it)
		}
	}
	flushRun()
	return b.String()
}

// wrapPara word-wraps the pending paragraph at the render width and appends
// the resulting lines to the output.
func (r *renderer) wrapPara() {
	p := r.para
	width := r.opts.Width
	avail1 := width - ansi.StringWidth(p.indent1)
	avail2 := width - ansi.StringWidth(p.indent2)
	if avail1 < 1 {
		avail1 = 1
	}
	if avail2 < 1 {
		avail2 = 1
	}

	items := buildItems(p.segs)
	var lines []wline
	cur := wline{indent: p.indent1, avail: avail1}
	pendingSpace := witem{kind: wiSpace, text: " "}

	flush := func() {
		lines = append(lines, cur)
		cur = wline{indent: p.indent2, avail: avail2}
	}

	for _, it := range items {
		switch it.kind {
		case wiBreak:
			flush()
			continue
		case wiSpace:
			continue
		}
		if it.glue && len(cur.items) > 0 && cur.cells+it.w <= cur.avail {
			cur.items = append(cur.items, it)
			cur.cells += it.w
			continue
		}
		if len(cur.items) > 0 && cur.cells+1+it.w > cur.avail {
			flush()
		}
		if it.w > cur.avail {
			for it.w > cur.avail {
				chunk, rest := splitWord(it, cur.avail)
				if chunk.w > 0 {
					if len(cur.items) > 0 {
						flush()
					}
					cur.items = append(cur.items, chunk)
					cur.cells += chunk.w
					flush()
				}
				it = rest
			}
			if it.w == 0 {
				continue
			}
			cur.items = append(cur.items, it)
			cur.cells += it.w
			continue
		}
		if len(cur.items) > 0 && !it.glue {
			sp := pendingSpace
			sp.st = prevStyle(cur.items)
			sp.href = prevHref(cur.items)
			cur.items = append(cur.items, sp)
			cur.cells++
		}
		cur.items = append(cur.items, it)
		cur.cells += it.w
	}
	if len(cur.items) > 0 {
		flush()
	}

	for _, ln := range lines {
		var b strings.Builder
		b.WriteString(r.bgPaint(ln.indent))
		pad := ln.avail - ln.cells
		if pad > 0 {
			switch p.align {
			case "center":
				b.WriteString(strings.Repeat(" ", pad/2))
			case "right":
				b.WriteString(strings.Repeat(" ", pad))
			}
		}
		b.WriteString(r.emitItems(ln.items))
		s := b.String()
		if s == "" {
			s = ln.indent
		}
		r.out = append(r.out, s)
	}
}

func prevStyle(items []witem) styleState {
	for i := len(items) - 1; i >= 0; i-- {
		if items[i].kind == wiWord {
			return items[i].st
		}
	}
	return styleState{}
}

func prevHref(items []witem) string {
	for i := len(items) - 1; i >= 0; i-- {
		if items[i].kind == wiWord {
			return items[i].href
		}
	}
	return ""
}

// splitWord breaks an oversized word into a chunk that fits avail and the
// remainder, splitting on display cells.
func splitWord(it witem, avail int) (witem, witem) {
	used := 0
	i := 0
	for i < len(it.text) {
		rn, sz := utf8.DecodeRuneInString(it.text[i:])
		w := ansi.StringWidth(string(rn))
		if used+w > avail {
			break
		}
		used += w
		i += sz
	}
	head := witem{kind: wiWord, text: it.text[:i], st: it.st, href: it.href, w: used}
	tail := witem{kind: wiWord, text: it.text[i:], st: it.st, href: it.href,
		w: ansi.StringWidth(it.text[i:])}
	return head, tail
}
