package gotestspec_test

import (
	"bytes"
	"encoding/xml"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestspec"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// BadgeTestSuite tests the coverage badge renderer: a self-contained SVG
// whose color band follows the coverage percentage.
type BadgeTestSuite struct{}

func (s *BadgeTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

type badgeCtx struct{}

func (s *BadgeTestSuite) BeforeEach(t *gotest.T) *badgeCtx { return &badgeCtx{} }

func render(t *gotest.T, pct float64) string {
	var buf bytes.Buffer
	gotest.NoError(t, gotestspec.RenderCoverageBadge(&buf, pct))
	return buf.String()
}

func (s *BadgeTestSuite) TestRendersWellFormedSVG(t *gotest.T, _ *badgeCtx) {
	svg := render(t, 82.34)

	t.It("is parseable XML rooted at <svg>", func(it *gotest.T) {
		var root struct {
			XMLName xml.Name
		}
		gotest.NoError(it, xml.Unmarshal([]byte(svg), &root))
		gotest.Equal(it, "svg", root.XMLName.Local)
	})

	t.It("labels the badge and prints the percentage with one decimal", func(it *gotest.T) {
		gotest.Contains(it, svg, ">coverage<")
		gotest.Contains(it, svg, ">82.3%<")
	})

	t.It("does not link or embed any external resource", func(it *gotest.T) {
		gotest.NotContains(it, svg, "href")
		gotest.NotContains(it, svg, "<image")
		gotest.NotContains(it, svg, "@import")
	})
}

func (s *BadgeTestSuite) TestColorFollowsCoverageBand(t *gotest.T, _ *badgeCtx) {
	for sub, tc := range gotest.Each(t, []struct {
		Desc  string
		pct   float64
		color string
	}{
		{Desc: "90 and above is bright green", pct: 90, color: "#4c1"},
		{Desc: "80 to 89 is green", pct: 89.9, color: "#97ca00"},
		{Desc: "70 to 79 is yellow-green", pct: 70, color: "#a4a61d"},
		{Desc: "60 to 69 is yellow", pct: 69.9, color: "#dfb317"},
		{Desc: "40 to 59 is orange", pct: 40, color: "#fe7d37"},
		{Desc: "below 40 is red", pct: 39.9, color: "#e05d44"},
		{Desc: "zero is red", pct: 0, color: "#e05d44"},
		{Desc: "full is bright green", pct: 100, color: "#4c1"},
	}) {
		svg := render(sub, tc.pct)
		gotest.Contains(sub, svg, `fill="`+tc.color+`"`)
	}
}

func (s *BadgeTestSuite) TestWidthGrowsWithTheMessage(t *gotest.T, _ *badgeCtx) {
	short := render(t, 5)
	long := render(t, 100)

	t.It("reserves more width for a longer percentage", func(it *gotest.T) {
		gotest.Less(it, widthOf(it, short), widthOf(it, long))
	})
}

func widthOf(t *gotest.T, svg string) int {
	var root struct {
		Width int `xml:"width,attr"`
	}
	gotest.NoError(t, xml.Unmarshal([]byte(svg), &root))
	gotest.NotZero(t, root.Width, "width attribute missing in %s", strings.SplitN(svg, "\n", 2)[0])
	return root.Width
}
