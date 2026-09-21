#!/usr/bin/env python3
"""Render every html_styles theme over a sample page, for eyeballing them.

The sample is exporter-shaped: the heading-wrapper divs, the status/todo/tag
spans, the checkbox list classes, the statistic cookie and the footnote markup
are the ones internal/app/orgs/plugs/html actually emits, so a theme that looks
right here looks right on a real export.

    python3 tools/htmlthemes/preview.py            # every theme
    python3 tools/htmlthemes/preview.py slate      # just one

Writes the pages to a temp directory and prints them; open those in a browser.
"""

import os
import sys
import tempfile

HERE = os.path.dirname(os.path.abspath(__file__))
STYLES = os.path.normpath(os.path.join(HERE, "..", "..", "templates", "html_styles"))

# html_default.tpl sets these on images inline, ahead of the theme's own css, so
# the preview carries them too - a theme has to be able to undo them.
TEMPLATE_INLINE = (
    "img{box-shadow:5px 5px 15px 0px #aa8;"
    "-webkit-box-reflect:below 0px linear-gradient(to bottom,"
    "rgba(0,0,0,0.0),rgba(0,0,0,0.2));}"
)

PAGE = (
    '<!DOCTYPE html><html><head><meta charset="utf-8"><title>%s</title>'
    "<style>%s</style><style>%s</style></head><body>%s</body></html>"
)

SAMPLE = r"""
<h1 class="title">The Wandering Notebook</h1>
<p class="subtitle">notes, specifications and the odd digression</p>

<div id="a" class="heading-wrapper">
<div id="a-title" class="heading-title-wrapper title-level-2">
<h2 id="a-heading"><span class="status" style="color:#0000CC;"> TODO </span> Getting started <span class="tags">project&#xa0;writing</span></h2>
</div>
<div id="a-content" class="heading-content-wrapper content-level-2">
<div id="a-text" class="heading-content-text">
<p>This paragraph exists to show the running text of the theme: a <a href="#">link to somewhere</a>, a piece of <code class="verbatim">verbatim</code>, some <strong>strong emphasis</strong>, a little <em>italic</em>, and a scheduled stamp <span class="timestamp">&lt;2026-09-14 Mon 09:30&gt;</span> sitting inline. The measure is deliberately narrow so long-form prose stays comfortable to read over many paragraphs.<sup class="footnote-reference"><a id="footnote-reference-1" href="#footnote-1">1</a></sup></p>
<blockquote><p>Everything should be made as simple as possible, but no simpler.</p></blockquote>

<div id="b" class="heading-wrapper">
<div id="b-title" class="heading-title-wrapper title-level-3">
<h3 id="b-heading"><span class="status" style="color:#CC9900;"> DOING </span> Checklist <code class="statistic">[1/3]</code></h3>
</div>
<div id="b-content" class="heading-content-wrapper content-level-3">
<div id="b-text" class="heading-content-text">
<ul>
<li class="checked">Draft the palette</li>
<li class="indeterminate">Wire the tokens through</li>
<li class="unchecked">Screenshot every variant</li>
</ul>
<ol><li>Ordered items<ul><li>and a nested bullet</li></ul></li><li>keep their rhythm</li></ol>
<dl><dt>theme</dt><dd>a stylesheet under html_styles, picked by name</dd><dt>variant</dt><dd>the light or dark half of a family</dd></dl>
</div></div></div>

<div id="c" class="heading-wrapper">
<div id="c-title" class="heading-title-wrapper title-level-4">
<h4 id="c-heading">A deeper section</h4>
</div>
<div id="c-content" class="heading-content-wrapper content-level-4">
<div id="c-text" class="heading-content-text">
<p>Nested bodies pick up a hairline down the left edge instead of an indent.</p>
<pre><code >func GetStylesheet(name string, fontfamily string) string {
	if data, err := os.ReadFile(path("html_styles/" + name + "_style.css")); err == nil {
		return string(data)
	}
	return ""
}</code></pre>
<h5>A label heading</h5>
<p>Trailing prose under a small heading.</p>
</div></div></div>
</div></div></div>

<div id="d" class="heading-wrapper">
<div id="d-title" class="heading-title-wrapper title-level-2">
<h2 id="d-heading"><span class="todo">WAITING</span> <span class="priority">[#A]</span> Reference table</h2>
</div>
<div id="d-content" class="heading-content-wrapper content-level-2">
<div id="d-text" class="heading-content-text">
<table>
<thead><tr><th class="align-left">Theme</th><th class="align-left">Family</th><th class="align-right">Measure</th><th class="align-left">Accent</th></tr></thead>
<tbody>
<tr><td class="align-left">journal</td><td class="align-left">serif</td><td class="align-right">44rem</td><td class="align-left">oxblood</td></tr>
<tr><td class="align-left">nordic</td><td class="align-left">sans</td><td class="align-right">46rem</td><td class="align-left">blue</td></tr>
<tr><td class="align-left">slate</td><td class="align-left">mono labels</td><td class="align-right">48rem</td><td class="align-left">violet</td></tr>
<tr class="active-row"><td class="align-left">default</td><td class="align-left">legacy</td><td class="align-right">n/a</td><td class="align-left">orange</td></tr>
</tbody>
<caption>The families and where each one is meant to be used.</caption>
</table>
<pre class="example">an example block
  keeps its own spacing</pre>
<hr>
<figure><img src="data:image/svg+xml;utf8,%3Csvg xmlns=%27http://www.w3.org/2000/svg%27 width=%27520%27 height=%27170%27%3E%3Crect width=%27520%27 height=%27170%27 fill=%27%23808a99%27/%3E%3Ctext x=%2750%25%27 y=%2755%25%27 fill=%27white%27 font-family=%27sans-serif%27 font-size=%2718%27 text-anchor=%27middle%27%3Efigure%3C/text%3E%3C/svg%3E" alt="figure"><figcaption>A figure caption sits under the image.</figcaption></figure>
</div></div></div>

<div class="footnotes">
<hr class="footnotes-separatator">
<div class="footnote-definitions">
<div class="footnote-definition"><sup id="footnote-1">1</sup><div class="footnote-body"><p>Footnotes hang their marker in the margin.</p></div></div>
</div></div>
"""


def themes():
    return sorted(n[: -len("_style.css")] for n in os.listdir(STYLES)
                  if n.endswith("_style.css"))


def main():
    wanted = sys.argv[1:] or themes()
    out = tempfile.mkdtemp(prefix="orgthemes-")
    for name in wanted:
        path = os.path.join(STYLES, name + "_style.css")
        if not os.path.exists(path):
            raise SystemExit("no such theme: %s (have: %s)"
                             % (name, ", ".join(themes())))
        with open(path) as f:
            # GetStylesheet substitutes this before the css reaches the page.
            css = f.read().replace("{{fontfamily}}", "Inconsolata")
        page = os.path.join(out, name + ".html")
        with open(page, "w") as f:
            f.write(PAGE % (name, TEMPLATE_INLINE, css, SAMPLE))
        print(page)


if __name__ == "__main__":
    main()
