package render

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"golang.org/x/net/html"
)

// tableCell is one rendered cell: its styled text plus the cell's own
// background color (used to fill the padded remainder of the column).
type tableCell struct {
	text string
	bg   string
}

func (r *renderer) renderTable(n *html.Node, st styleState) {
	var headers []tableCell
	var rows [][]tableCell
	hasHead := false

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode {
			continue
		}
		switch c.Data {
		case "caption":
			r.renderTableCaption(c, st)
		case "thead":
			for tr := c.FirstChild; tr != nil; tr = tr.NextSibling {
				if tr.Type == html.ElementNode && tr.Data == "tr" {
					headers = r.tableRow(tr)
					hasHead = true
				}
			}
		case "tbody", "tfoot":
			for tr := c.FirstChild; tr != nil; tr = tr.NextSibling {
				if tr.Type == html.ElementNode && tr.Data == "tr" {
					if row := r.tableRow(tr); len(row) > 0 {
						rows = append(rows, row)
					}
				}
			}
		case "tr":
			if row := r.tableRow(c); len(row) > 0 {
				rows = append(rows, row)
			}
		}
	}

	if !hasHead {
		// no explicit thead: promote a leading row of th cells if present
		headers, rows = promoteHeader(rows)
	}

	table := r.boxTable(headers, rows, r.ps, r.availWidth)
	if table != "" {
		r.out = append(r.out, strings.Split(table, "\n")...)
	}
}

func (r *renderer) renderTableCaption(c *html.Node, st styleState) {
	r.endBlock()
	capSt := st
	capSt.displayNone = false
	capSt.bg = ""
	info := newElemInfo(c)
	capSt = styleState{align: capSt.align, pre: capSt.pre, nowrap: capSt.nowrap}
	r.applyCSS(&capSt, info, attrValue(c, "style"))
	r.stack = append(r.stack, info)
	r.st = capSt
	r.startPara(capSt)
	r.walkChildren(c)
	r.flushInline()
	r.endBlock()
	r.st = st
	r.stack = r.stack[:len(r.stack)-1]
}

func (r *renderer) tableRow(tr *html.Node) []tableCell {
	var cells []tableCell
	// tr participates in the selector stack so "tr.add td" style rules match
	info := newElemInfo(tr)
	r.stack = append(r.stack, info)
	for c := tr.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && (c.Data == "td" || c.Data == "th") {
			cells = append(cells, r.cellText(c))
		}
	}
	r.stack = r.stack[:len(r.stack)-1]
	return cells
}

// cellText renders one cell into a temporary buffer and returns its text
// along with the cell's computed background color.
func (r *renderer) cellText(cell *html.Node) tableCell {
	outMark := len(r.out)
	savedPara, savedSt, savedHref := r.para, r.st, r.href
	r.para = para{}
	info := newElemInfo(cell)
	r.stack = append(r.stack, info)
	st := r.st
	st.displayNone = false
	st.bg = ""
	r.applyCSS(&st, info, attrValue(cell, "style"))
	r.st = st
	r.walkChildren(cell)
	r.flushInline()
	r.stack = r.stack[:len(r.stack)-1]

	var lines []string
	for _, l := range r.out[outMark:] {
		if strings.TrimSpace(ansi.Strip(l)) != "" {
			lines = append(lines, l)
		}
	}
	r.out = r.out[:outMark]
	r.para, r.st, r.href = savedPara, savedSt, savedHref
	return tableCell{text: strings.Join(lines, " "), bg: st.bg}
}

func promoteHeader(rows [][]tableCell) ([]tableCell, [][]tableCell) {
	if len(rows) == 0 {
		return nil, nil
	}
	for _, c := range rows[0] {
		if !strings.Contains(c.text, "\x1b[1m") {
			return nil, rows
		}
	}
	return rows[0], rows[1:]
}

// boxTable renders a fixed-width table with box-drawing borders. Cell
// padding carries the cell's background so filled rows read as solid bands.
func (r *renderer) boxTable(headers []tableCell, rows [][]tableCell, ps preset, width int) string {
	all := append([][]tableCell{}, rows...)
	hasHead := len(headers) > 0
	if hasHead {
		all = append([][]tableCell{headers}, all...)
	}
	if len(all) == 0 {
		return ""
	}
	ncols := 0
	for _, row := range all {
		if len(row) > ncols {
			ncols = len(row)
		}
	}
	if ncols == 0 {
		return ""
	}

	widths := make([]int, ncols)
	for _, row := range all {
		for i := 0; i < ncols; i++ {
			var cell tableCell
			if i < len(row) {
				cell = row[i]
			}
			if w := ansi.StringWidth(cell.text); w > widths[i] {
				widths[i] = w
			}
		}
	}

	rowWidth := func() int {
		s := 1
		for _, w := range widths {
			s += w + 3
		}
		return s
	}
	for rowWidth() > width {
		best, bestW := -1, 3
		for i, w := range widths {
			if w > bestW {
				best, bestW = i, w
			}
		}
		if best < 0 {
			break
		}
		widths[best]--
	}

	trunc := func(cell tableCell, w int) tableCell {
		if ansi.StringWidth(cell.text) <= w {
			return cell
		}
		// hard cut: strip styling so escape pairs can't dangle mid-cell
		return tableCell{text: ansi.Truncate(ansi.Strip(cell.text), w, "…"), bg: cell.bg}
	}

	h, vm := ps.hr(), "│"
	corners := [6]string{"┌", "┬", "┐", "├", "┼", "┤"}
	bottom := [3]string{"└", "┴", "┘"}
	if ps.ascii {
		vm = "|"
		corners = [6]string{"+", "+", "+", "+", "+", "+"}
		bottom = [3]string{"+", "+", "+"}
	}

	border := func(l, m, rr string) string {
		parts := make([]string, ncols)
		for i, w := range widths {
			parts[i] = strings.Repeat(h, w+2)
		}
		s := l + strings.Join(parts, m) + rr
		if ps.colored {
			s = lipStyle{fg: "#5c6370", bg: r.bg()}.paintPlain(s)
		}
		return s
	}

	renderRow := func(row []tableCell) string {
		parts := make([]string, ncols)
		for i := 0; i < ncols; i++ {
			var cell tableCell
			if i < len(row) {
				cell = trunc(row[i], widths[i])
			}
			pad := widths[i] - ansi.StringWidth(cell.text)
			if pad < 0 {
				pad = 0
			}
			spaces := strings.Repeat(" ", pad+1)
			padBG := cell.bg
			if padBG == "" {
				padBG = r.bg()
			}
			if ps.colored && padBG != "" {
				spaces = lipStyle{bg: padBG}.paintPlain(spaces)
			}
			parts[i] = r.bgPaint(" ") + cell.text + spaces
		}
		return vm + strings.Join(parts, vm) + vm
	}

	var b strings.Builder
	b.WriteString(border(corners[0], corners[1], corners[2]))
	b.WriteString("\n")
	if hasHead {
		b.WriteString(renderRow(headers))
		b.WriteString("\n")
		b.WriteString(border(corners[3], corners[4], corners[5]))
		b.WriteString("\n")
	}
	for i, row := range rows {
		b.WriteString(renderRow(row))
		if i < len(rows)-1 {
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(border(bottom[0], bottom[1], bottom[2]))
	return b.String()
}
