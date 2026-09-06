package gotestspec

import (
	"fmt"
	"io"
	"math"
)

// coverageBadgeColor maps a percentage to its band's fill color (shields.io
// palette, so the badge matches what people expect a coverage badge to
// look like). Boundaries are inclusive at the lower end.
func coverageBadgeColor(pct float64) string {
	switch {
	case pct >= 90:
		return "#4c1"
	case pct >= 80:
		return "#97ca00"
	case pct >= 70:
		return "#a4a61d"
	case pct >= 60:
		return "#dfb317"
	case pct >= 40:
		return "#fe7d37"
	default:
		return "#e05d44"
	}
}

// badgeTextWidth estimates the rendered width of s in the badge's 11px
// Verdana. The estimate only has to be stable, not exact: the SVG pins the
// text to this width via textLength, so the layout never overflows.
func badgeTextWidth(s string) int {
	return int(math.Round(float64(len(s))*6.5)) + 10
}

// RenderCoverageBadge writes a flat-style SVG badge reading "coverage
// <pct>%" to w. The SVG is self-contained: no fonts, images or links are
// fetched when it is displayed.
func RenderCoverageBadge(w io.Writer, pct float64) error {
	label := "coverage"
	message := fmt.Sprintf("%.1f%%", pct)
	color := coverageBadgeColor(pct)

	lw := badgeTextWidth(label)
	mw := badgeTextWidth(message)
	total := lw + mw

	_, err := fmt.Fprintf(w, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="20" role="img" aria-label="%s: %s">
  <title>%s: %s</title>
  <linearGradient id="s" x2="0" y2="100%%"><stop offset="0" stop-color="#bbb" stop-opacity=".1"/><stop offset="1" stop-opacity=".1"/></linearGradient>
  <clipPath id="r"><rect width="%d" height="20" rx="3" fill="#fff"/></clipPath>
  <g clip-path="url(#r)">
    <rect width="%d" height="20" fill="#555"/>
    <rect x="%d" width="%d" height="20" fill="%s"/>
    <rect width="%d" height="20" fill="url(#s)"/>
  </g>
  <g fill="#fff" text-anchor="middle" font-family="Verdana,Geneva,DejaVu Sans,sans-serif" text-rendering="geometricPrecision" font-size="110">
    <text aria-hidden="true" x="%d" y="150" fill="#010101" fill-opacity=".3" transform="scale(.1)" textLength="%d">%s</text>
    <text x="%d" y="140" transform="scale(.1)" textLength="%d">%s</text>
    <text aria-hidden="true" x="%d" y="150" fill="#010101" fill-opacity=".3" transform="scale(.1)" textLength="%d">%s</text>
    <text x="%d" y="140" transform="scale(.1)" textLength="%d">%s</text>
  </g>
</svg>
`,
		total, label, message,
		label, message,
		total,
		lw,
		lw, mw, color,
		total,
		lw*5, (lw-10)*10, label,
		lw*5, (lw-10)*10, label,
		(lw*2+mw)*5, (mw-10)*10, message,
		(lw*2+mw)*5, (mw-10)*10, message,
	)
	return err
}
