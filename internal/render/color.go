package render

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// cssColorToHex converts a CSS color value to a #rrggbb string. It returns an
// empty string when the value cannot be represented in a terminal (e.g.
// "transparent", "currentColor", gradients).
func cssColorToHex(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	if v == "" {
		return ""
	}
	if hex, ok := cssNamedColors[v]; ok {
		return hex
	}
	switch {
	case strings.HasPrefix(v, "#"):
		return parseHexColor(v)
	case strings.HasPrefix(v, "rgb"):
		return parseRGBFunc(v)
	case strings.HasPrefix(v, "hsl"):
		return parseHSLFunc(v)
	}
	return ""
}

func parseHexColor(v string) string {
	h := strings.TrimPrefix(v, "#")
	switch len(h) {
	case 3:
		if !isHexDigits(h) {
			return ""
		}
		return "#" + strings.ToLower(string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]}))
	case 4:
		if !isHexDigits(h) {
			return ""
		}
		return "#" + strings.ToLower(string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]}))
	case 6, 8:
		if !isHexDigits(h) {
			return ""
		}
		return "#" + strings.ToLower(h[:6])
	}
	return ""
}

func isHexDigits(s string) bool {
	for _, r := range s {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return false
		}
	}
	return true
}

// parseRGBFunc handles rgb()/rgba() with comma or space syntax, numbers or
// percentages, and an optional alpha component.
func parseRGBFunc(v string) string {
	tokens := cssFuncTokens(v)
	if len(tokens) < 3 {
		return ""
	}
	vals := make([]float64, 0, 3)
	for _, t := range tokens[:3] {
		f, pct, ok := parseCSSNumber(t)
		if !ok {
			return ""
		}
		if pct {
			f = f * 255 / 100
		}
		vals = append(vals, clampByte(f))
	}
	return fmt.Sprintf("#%02x%02x%02x", int(vals[0]), int(vals[1]), int(vals[2]))
}

func parseHSLFunc(v string) string {
	tokens := cssFuncTokens(v)
	if len(tokens) < 3 {
		return ""
	}
	nums := make([]float64, 0, 3)
	for i, t := range tokens[:3] {
		t = strings.TrimSuffix(t, "deg")
		f, pct, ok := parseCSSNumber(t)
		if !ok {
			return ""
		}
		if i > 0 && !pct {
			return ""
		}
		nums = append(nums, f)
	}
	r, g, b := hslToRGB(nums[0], nums[1]/100, nums[2]/100)
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

// cssFuncTokens extracts the arguments of a CSS functional value, e.g.
// "rgb(1, 2, 3)" -> ["1", "2", "3"].
func cssFuncTokens(v string) []string {
	open := strings.Index(v, "(")
	if open < 0 {
		return nil
	}
	close := strings.LastIndex(v, ")")
	if close < open {
		close = len(v)
	}
	inner := strings.NewReplacer(",", " ", "/", " ").Replace(v[open+1 : close])
	return strings.Fields(inner)
}

func parseCSSNumber(t string) (float64, bool, bool) {
	pct := strings.HasSuffix(t, "%")
	if pct {
		t = strings.TrimSuffix(t, "%")
	}
	f, err := strconv.ParseFloat(t, 64)
	if err != nil {
		return 0, false, false
	}
	return f, pct, true
}

func clampByte(f float64) float64 {
	return math.Max(0, math.Min(255, math.Round(f)))
}

func hslToRGB(h, s, l float64) (int, int, int) {
	h = math.Mod(math.Mod(h, 360)+360, 360) / 360
	var f func(n float64) float64
	f = func(n float64) float64 {
		k := math.Mod(n+h*12, 12)
		a := s * math.Min(l, 1-l)
		return l - a*math.Max(-1, math.Min(math.Min(k-3, 9-k), 1))
	}
	return int(clampByte(f(0) * 255)), int(clampByte(f(8) * 255)), int(clampByte(f(4) * 255))
}

// cssNamedColors maps common CSS color keywords to hex values.
var cssNamedColors = map[string]string{
	"black": "#000000", "white": "#ffffff", "red": "#ff0000", "green": "#008000",
	"blue": "#0000ff", "yellow": "#ffff00", "orange": "#ffa500", "purple": "#800080",
	"pink": "#ffc0cb", "brown": "#a52a2a", "gray": "#808080", "grey": "#808080",
	"cyan": "#00ffff", "magenta": "#ff00ff", "silver": "#c0c0c0", "gold": "#ffd700",
	"maroon": "#800000", "navy": "#000080", "teal": "#008080", "olive": "#808000",
	"lime": "#00ff00", "aqua": "#00ffff", "fuchsia": "#ff00ff", "violet": "#ee82ee",
	"indigo": "#4b0082", "coral": "#ff7f50", "salmon": "#fa8072", "tomato": "#ff6347",
	"crimson": "#dc143c", "darkred": "#8b0000", "darkgreen": "#006400",
	"darkblue": "#00008b", "darkgray": "#a9a9a9", "darkgrey": "#a9a9a9",
	"lightgray": "#d3d3d3", "lightgrey": "#d3d3d3", "whitesmoke": "#f5f5f5",
	"gainsboro": "#dcdcdc", "lightblue": "#add8e6", "lightgreen": "#90ee90",
	"lightyellow": "#ffffe0", "orchid": "#da70d6", "plum": "#dda0dd",
	"slategray": "#708090", "slategrey": "#708090", "steelblue": "#4682b4",
	"dodgerblue": "#1e90ff", "royalblue": "#4169e1", "skyblue": "#87ceeb",
	"seagreen": "#2e8b57", "mediumseagreen": "#3cb371", "forestgreen": "#228b22",
	"khaki": "#f0e68c", "beige": "#f5f5dc", "ivory": "#fffff0", "azure": "#f0ffff",
	"lavender": "#e6e6fa", "tan": "#d2b48c", "wheat": "#f5deb3", "chocolate": "#d2691e",
	"peru": "#cd853f", "sienna": "#a0522d", "firebrick": "#b22222", "hotpink": "#ff69b4",
	"deeppink": "#ff1493", "goldenrod": "#daa520", "darkorange": "#ff8c00",
	"lightcyan": "#e0ffff", "aliceblue": "#f0f8ff", "ghostwhite": "#f8f8ff",
	"linen": "#faf0e6", "snow": "#fffafa", "honeydew": "#f0fff0", "mintcream": "#f5fffa",
}
