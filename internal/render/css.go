package render

import (
	"strings"
)

// A practical CSS subset: tag, .class, #id, * selectors with descendant and
// child combinators; a handful of pseudo-classes that always match. Unsupported
// selectors are dropped rather than failing the whole stylesheet.

type decl struct {
	prop string
	val  string
}

type compoundSel struct {
	tag     string // empty matches any element
	classes []string
	id      string
}

type combinator int

const (
	combDescendant combinator = iota
	combChild
)

type complexSel struct {
	parts []compoundSel
	combs []combinator // len(parts)-1
}

type rule struct {
	sel     complexSel
	decls   []decl
	ids     int
	classes int
	tags    int
	order   int
}

type stylesheet struct {
	rules []rule
}

var supportedPseudoClasses = map[string]bool{
	"hover": true, "focus": true, "active": true, "visited": true, "link": true,
}

// add parses a CSS document and appends its rules. Nested blocks (media
// queries and similar) are flattened and applied regardless of context.
func (ss *stylesheet) add(css string, orderBase int) int {
	css = stripCSSComments(css)
	order := orderBase
	i := 0
	for i < len(css) {
		start := i
		for i < len(css) && css[i] != '{' && css[i] != '}' && css[i] != ';' {
			i++
		}
		if i >= len(css) {
			break
		}
		selText := strings.TrimSpace(css[start:i])
		switch css[i] {
		case ';': // blockless at-rule (e.g. @import) or stray semicolon
			i++
			continue
		case '}': // stray close brace
			i++
			continue
		}
		// css[i] == '{': read the block with nesting
		i++
		blockStart := i
		depth := 1
		for i < len(css) && depth > 0 {
			switch css[i] {
			case '{':
				depth++
			case '}':
				depth--
			}
			i++
		}
		block := css[blockStart : i-1]
		if selText == "" {
			continue
		}
		if strings.HasPrefix(selText, "@") {
			name := atRuleName(selText)
			switch name {
			case "media", "supports", "layer", "container":
				order = ss.add(block, order) // flatten inner rules
			}
			continue
		}
		for _, raw := range strings.Split(selText, ",") {
			sel, ok := parseComplexSelector(raw)
			if !ok {
				continue
			}
			r := rule{sel: sel, decls: parseDecls(block), order: order}
			for _, p := range sel.parts {
				if p.id != "" {
					r.ids++
				}
				r.classes += len(p.classes)
				if p.tag != "" {
					r.tags++
				}
			}
			if len(r.decls) > 0 {
				ss.rules = append(ss.rules, r)
				order++
			}
		}
	}
	return order
}

func atRuleName(sel string) string {
	s := strings.TrimPrefix(sel, "@")
	if i := strings.IndexAny(s, " \t\r\n{"); i >= 0 {
		s = s[:i]
	}
	return strings.ToLower(s)
}

func stripCSSComments(css string) string {
	var b strings.Builder
	for {
		i := strings.Index(css, "/*")
		if i < 0 {
			b.WriteString(css)
			break
		}
		b.WriteString(css[:i])
		j := strings.Index(css[i+2:], "*/")
		if j < 0 {
			break
		}
		b.WriteByte(' ')
		css = css[i+2+j+2:]
	}
	return b.String()
}

// parseComplexSelector parses one compound chain. It reports false for syntax
// this renderer does not support (attribute selectors, sibling combinators,
// generated-content pseudo-elements, ...).
func parseComplexSelector(s string) (complexSel, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return complexSel{}, false
	}
	var sel complexSel
	pendingCombinator := combDescendant
	havePart := false
	for s != "" {
		if strings.HasPrefix(s, ">") {
			if !havePart {
				return complexSel{}, false
			}
			pendingCombinator = combChild
			s = strings.TrimSpace(s[1:])
			continue
		}
		if s[0] == '+' || s[0] == '~' || s[0] == '[' || s[0] == ':' && len(s) > 1 && s[1] == ':' {
			return complexSel{}, false
		}
		comp, rest, ok := parseCompound(s)
		if !ok {
			return complexSel{}, false
		}
		if havePart {
			sel.combs = append(sel.combs, pendingCombinator)
		}
		sel.parts = append(sel.parts, comp)
		havePart = true
		pendingCombinator = combDescendant
		s = strings.TrimLeft(rest, " \t\r\n")
	}
	if len(sel.parts) == 0 {
		return complexSel{}, false
	}
	return sel, true
}

func parseCompound(s string) (compoundSel, string, bool) {
	var c compoundSel
	i := 0
	for i < len(s) {
		switch s[i] {
		case '*':
			i++
		case '.':
			j := i + 1
			for j < len(s) && isSelectorRune(s[j]) {
				j++
			}
			if j == i+1 {
				return c, s, false
			}
			c.classes = append(c.classes, strings.ToLower(s[i+1:j]))
			i = j
		case '#':
			j := i + 1
			for j < len(s) && isSelectorRune(s[j]) {
				j++
			}
			if j == i+1 {
				return c, s, false
			}
			c.id = strings.ToLower(s[i+1 : j])
			i = j
		case ':':
			j := i + 1
			if j < len(s) && s[j] == ':' {
				return c, s, false // pseudo-elements not supported
			}
			for j < len(s) && isSelectorRune(s[j]) {
				j++
			}
			if j == i+1 {
				return c, s, false
			}
			pseudo := strings.ToLower(s[i+1 : j])
			if !supportedPseudoClasses[pseudo] {
				return c, s, false
			}
			// supported pseudo-classes always match; they only add specificity
			c.classes = append(c.classes, ":"+pseudo)
			i = j
		case ' ', '\t', '\r', '\n', '>', '+', '~', '[', ')':
			return c, s[i:], true
		default:
			if !isSelectorRune(s[i]) {
				return c, s, false
			}
			j := i
			for j < len(s) && isSelectorRune(s[j]) {
				j++
			}
			if c.tag != "" {
				return c, s, false
			}
			c.tag = strings.ToLower(s[i:j])
			i = j
		}
	}
	return c, "", true
}

func isSelectorRune(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' ||
		b == '-' || b == '_' || b == '\\'
}

// sort orders rules by ascending specificity then document order, so applying
// them in sequence implements the CSS cascade.
func (ss *stylesheet) sortRules() {
	rs := ss.rules
	// insertion sort: stable, sheets are small
	for i := 1; i < len(rs); i++ {
		for j := i; j > 0 && ruleLess(rs[j], rs[j-1]); j-- {
			rs[j], rs[j-1] = rs[j-1], rs[j]
		}
	}
}

func ruleLess(a, b rule) bool {
	if a.ids != b.ids {
		return a.ids < b.ids
	}
	if a.classes != b.classes {
		return a.classes < b.classes
	}
	if a.tags != b.tags {
		return a.tags < b.tags
	}
	return a.order < b.order
}

// elemInfo carries the selector-matching facts about one element.
type elemInfo struct {
	tag       string
	id        string
	classes   []string
	alignAttr string // legacy align="..." attribute, treated as text-align
}

func (ei *elemInfo) hasClass(c string) bool {
	for _, cl := range ei.classes {
		if cl == c {
			return true
		}
	}
	return false
}

func matchCompound(c compoundSel, ei *elemInfo) bool {
	if c.tag != "" && c.tag != ei.tag {
		return false
	}
	if c.id != "" && c.id != ei.id {
		return false
	}
	for _, cl := range c.classes {
		if strings.HasPrefix(cl, ":") {
			continue // supported pseudo-classes always match
		}
		if !ei.hasClass(cl) {
			return false
		}
	}
	return true
}

func matchSelector(sel complexSel, stack []*elemInfo) bool {
	si := len(stack) - 1
	last := len(sel.parts) - 1
	if si < 0 || !matchCompound(sel.parts[last], stack[si]) {
		return false
	}
	for j := last; j > 0; j-- {
		switch sel.combs[j-1] {
		case combChild:
			si--
			if si < 0 || !matchCompound(sel.parts[j-1], stack[si]) {
				return false
			}
		case combDescendant:
			si--
			for si >= 0 && !matchCompound(sel.parts[j-1], stack[si]) {
				si--
			}
			if si < 0 {
				return false
			}
		}
	}
	return true
}

// parseDecls parses "prop: value; prop2: value2" including inline style
// attributes. Unsupported junk is tolerated; unknown properties are dropped
// at apply time.
func parseDecls(s string) []decl {
	var out []decl
	for _, part := range strings.Split(s, ";") {
		i := strings.Index(part, ":")
		if i < 0 {
			continue
		}
		prop := strings.ToLower(strings.TrimSpace(part[:i]))
		val := strings.TrimSpace(part[i+1:])
		if j := strings.Index(strings.ToLower(val), "!important"); j >= 0 {
			val = strings.TrimSpace(val[:j])
		}
		if prop == "" || val == "" {
			continue
		}
		out = append(out, decl{prop: prop, val: val})
	}
	return out
}
