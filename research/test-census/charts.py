#!/usr/bin/env python3
"""Generate the census SVG charts from census-full.csv.

Marks: horizontal bars 16px, rounded data-end (r=4) only, 2px in-group gap,
direct value labels in secondary ink, hairline grid, legend for 2-series.
Theme: CSS vars swapped by prefers-color-scheme inside each SVG.
"""
import csv, os, sys

SRC = sys.argv[1]
OUT = sys.argv[2]
os.makedirs(OUT, exist_ok=True)

rows = [r for r in csv.DictReader(open(SRC)) if int(r["test_files"]) > 0]
I = lambda r, k: int(r[k])
N = len(rows)

STYLE = """<style>
svg{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,'Helvetica Neue',Arial,sans-serif}
:root{--s1:#007d9c;--s2:#eb6834;--ink:#24292f;--ink2:#656d76;--grid:#d0d7de}
@media (prefers-color-scheme:dark){:root{--s1:#0a9cc4;--s2:#d95926;--ink:#e6edf3;--ink2:#8b949e;--grid:#30363d}}
.lbl{fill:var(--ink);font-size:13px}
.val{fill:var(--ink2);font-size:12px}
.tick{fill:var(--ink2);font-size:11px}
.grid{stroke:var(--grid);stroke-width:1}
</style>"""

def bar_path(x, y, w, h, r=4):
    """Left edge square (baseline), right edge rounded."""
    if w <= r:
        return f'<rect x="{x}" y="{y}" width="{max(w,1)}" height="{h}" fill-opacity="1"/>'
    return (f'M{x},{y} h{w - r} a{r},{r} 0 0 1 {r},{r} v{h - 2 * r} '
            f'a{r},{r} 0 0 1 -{r},{r} h-{w - r} z')

def nice_max(v):
    import math
    return math.ceil(v / 20) * 20

def hbar_chart(fname, items, unit="%", width=680, lab_w=150, series_var="--s1", xmax=None):
    """Single-series horizontal bar chart. items = [(label, value)]."""
    bh, gap, top = 16, 12, 8
    plot_w = width - lab_w - 60
    xmax = xmax or nice_max(max(v for _, v in items))
    height = top + len(items) * (bh + gap) + 24
    parts = [f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {width} {height}" '
             f'width="{width}" height="{height}" role="img">', STYLE]
    # gridlines at quarters
    for q in (0.25, 0.5, 0.75, 1.0):
        gx = lab_w + plot_w * q
        parts.append(f'<line class="grid" x1="{gx:.0f}" y1="{top}" x2="{gx:.0f}" y2="{height-24}"/>')
        parts.append(f'<text class="tick" x="{gx:.0f}" y="{height-8}" text-anchor="middle">{xmax*q:.0f}{unit}</text>')
    y = top
    for label, v in items:
        w = plot_w * v / xmax
        parts.append(f'<text class="lbl" x="{lab_w-8}" y="{y+bh-4}" text-anchor="end">{label}</text>')
        parts.append(f'<path d="{bar_path(lab_w, y, w, bh)}" fill="var({series_var})"/>')
        parts.append(f'<text class="val" x="{lab_w+w+6:.0f}" y="{y+bh-4}">{v:g}{unit}</text>')
        y += bh + gap
    parts.append("</svg>")
    open(os.path.join(OUT, fname), "w").write("\n".join(parts))
    print("wrote", fname)

def grouped_chart(fname, groups, s1_name, s2_name, unit="%", width=680, lab_w=170):
    """Two-series grouped horizontal bars. groups = [(label, v1, v2)]."""
    bh, ingap, ggap, top = 16, 2, 18, 34
    plot_w = width - lab_w - 60
    xmax = nice_max(max(max(a, b) for _, a, b in groups))
    gh = 2 * bh + ingap
    height = top + len(groups) * (gh + ggap) + 24
    parts = [f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {width} {height}" '
             f'width="{width}" height="{height}" role="img">', STYLE]
    # legend
    parts.append(f'<rect x="{lab_w}" y="6" width="12" height="12" rx="3" fill="var(--s1)"/>')
    parts.append(f'<text class="lbl" x="{lab_w+18}" y="16">{s1_name}</text>')
    lx2 = lab_w + 18 + 7 * len(s1_name) + 40
    parts.append(f'<rect x="{lx2}" y="6" width="12" height="12" rx="3" fill="var(--s2)"/>')
    parts.append(f'<text class="lbl" x="{lx2+18}" y="16">{s2_name}</text>')
    for q in (0.25, 0.5, 0.75, 1.0):
        gx = lab_w + plot_w * q
        parts.append(f'<line class="grid" x1="{gx:.0f}" y1="{top}" x2="{gx:.0f}" y2="{height-24}"/>')
        parts.append(f'<text class="tick" x="{gx:.0f}" y="{height-8}" text-anchor="middle">{xmax*q:.0f}{unit}</text>')
    y = top
    for label, v1, v2 in groups:
        parts.append(f'<text class="lbl" x="{lab_w-8}" y="{y+gh/2+5:.0f}" text-anchor="end">{label}</text>')
        for i, (v, var) in enumerate(((v1, "--s1"), (v2, "--s2"))):
            by = y + i * (bh + ingap)
            w = plot_w * v / xmax
            parts.append(f'<path d="{bar_path(lab_w, by, w, bh)}" fill="var({var})"/>')
            parts.append(f'<text class="val" x="{lab_w+w+6:.0f}" y="{by+bh-4}">{v:g}{unit}</text>')
        y += gh + ggap
    parts.append("</svg>")
    open(os.path.join(OUT, fname), "w").write("\n".join(parts))
    print("wrote", fname)

# ---- chart 1: framework adoption (share of repos) ----
fw = {}
for r in rows:
    for pair in r["frameworks"].split(";"):
        if pair:
            fw[pair.split(":")[0]] = fw.get(pair.split(":")[0], 0) + 1
nice = {"httptest": "httptest (stdlib)", "testify": "testify assert/require",
        "testify-suite": "testify/suite", "testify-mock": "testify/mock",
        "go-cmp": "go-cmp", "gomock": "gomock", "gomega": "gomega",
        "ginkgo": "ginkgo", "quick": "testing/quick (stdlib)",
        "testcontainers": "testcontainers-go", "dockertest": "dockertest",
        "snapshot-lib": "snapshot libraries", "gocheck": "gocheck", "goconvey": "goconvey"}
items = sorted(fw.items(), key=lambda kv: -kv[1])[:10]
hbar_chart("frameworks.svg", [(nice.get(k, k), round(100 * v / N)) for k, v in items])

# ---- chart 2: parallelism broad vs shallow ----
def cohorts():
    by_stars = sorted(rows, key=lambda r: -int(r["stars"]))
    return [("Full corpus", rows), ("Top 100 by stars", by_stars[:100]), ("Long tail (400)", by_stars[-400:])]
groups = []
for name, rs in cohorts():
    t = sum(I(r, "test_funcs") for r in rs)
    adopters = 100 * sum(1 for r in rs if I(r, "parallel_tests") + I(r, "parallel_subtests") > 0) / len(rs)
    partests = 100 * sum(I(r, "parallel_tests") for r in rs) / max(t, 1)
    groups.append((name, round(adopters), round(partests, 1)))
grouped_chart("parallel.svg", groups, "Repos using t.Parallel() anywhere", "Tests that run in parallel")

# ---- chart 3: maturity gradient ----
by_stars = sorted(rows, key=lambda r: -int(r["stars"]))
top, tail = by_stars[:100], by_stars[-400:]
def share(rs, k): return round(100 * sum(1 for r in rs if I(r, k) > 0) / len(rs))
markers = [("TestMain", "testmain_pkgs"), ("Native fuzz tests", "fuzz_funcs"),
           ("t.Cleanup()", "cleanup_calls"), ("t.Helper()", "helper_calls")]
grouped_chart("maturity.svg", [(lbl, share(top, k), share(tail, k)) for lbl, k in markers],
              "Top 100 by stars", "Long tail (bottom 400)")

# ---- chart 4: container images ----
imgs = {}
for r in rows:
    seen = set()
    for field in ("container_images", "compose_images"):
        for pair in r[field].split(";"):
            if pair:
                base = pair.split(":")[0].split("/")[-1]
                if base not in seen:
                    imgs[base] = imgs.get(base, 0) + 1
                    seen.add(base)
nice_img = {"all-in-one": "jaeger", "clickhouse-server": "clickhouse",
            "opentelemetry-collector-contrib": "otel collector"}
top_imgs = sorted(imgs.items(), key=lambda kv: -kv[1])[:10]
hbar_chart("containers.svg", [(nice_img.get(k, k), v) for k, v in top_imgs],
           unit="", series_var="--s1")
print("N =", N)
