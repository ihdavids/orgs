<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1, maximum-scale=1">
  <meta name="apple-mobile-web-app-capable" content="yes">
  <title>{{title}}</title>
  {%if fontfamily%}<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family={{fontfamily}}&display=swap">{%endif%}
  <link rel="stylesheet" href="{{webslides_cdn}}/static/css/webslides.css">
  {%if icons%}<link rel="stylesheet" href="{{webslides_cdn}}/static/css/svg-icons.css">{%endif%}
  {%if animate%}<link rel="stylesheet" href="{{animate_cdn}}">{%endif%}
  <link rel="stylesheet" href="{{hljs_cdn}}/styles/{{hljs_style}}.min.css">
  {%if extra_css%}<link rel="stylesheet" href="{{extra_css}}">{%endif%}
  <style>
  {%autoescape off%}
  {{stylesheet}}
  {%endautoescape%}
  {%if fontfamily%}
  body, .text-landing { font-family: "{{fontfamily}}", -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; }
  {%endif%}
  </style>
  {%if local_theme%}
  <!-- The theme: the four-framework mapping, then the theme itself. -->
  <style>
  {%autoescape off%}
  {{themedata}}
  {%endautoescape%}
  </style>
  {%endif%}
  {%autoescape off%}
  {{head}}
  {%endautoescape%}
</head>
<body class="ws-theme-{{theme}}">

<main role="main">
  <article id="webslides"{%if vertical%} class="vertical"{%endif%}>
    {%autoescape off%}
    {{slide_data}}
    {%endautoescape%}
  </article>
</main>

<!-- The speaker's own crib sheet: a :NOTES: drawer on a slide, shown on `n`
     and never on the screen behind you. WebSlides has no notes view of its
     own, so this is ours, and it is deliberately the same key reveal.js uses
     for the same thing. -->
{%if notes%}
<div id="ws-notes-panel" aria-live="polite"></div>
{%endif%}

<script src="{{webslides_cdn}}/static/js/webslides.js"></script>
{%if icons%}<script defer src="{{webslides_cdn}}/static/js/svg-icons.js"></script>{%endif%}
<script src="{{hljs_cdn}}/highlight.min.js"></script>
<script>
  window.ws = new WebSlides({%autoescape off%}{{options}}{%endautoescape%});
  if (window.hljs) { document.querySelectorAll('pre code').forEach(function (b) { hljs.highlightElement(b); }); }
</script>
{%if notes%}
<script>
(function () {
  var panel = document.getElementById('ws-notes-panel');
  if (!panel) { return; }
  var show = function () {
    // Ask WebSlides which slide it is on rather than looking for the class:
    // during a transition *both* slides carry `current` for a moment, and the
    // one the DOM finds first is the one being left behind - so a note read
    // then is the note for the slide you have just walked away from.
    var cur = (window.ws && window.ws.currentSlide_ && window.ws.currentSlide_.el) ||
      document.querySelector('#webslides section.current') ||
      document.querySelector('#webslides section');
    var notes = cur ? cur.querySelector('aside.ws-notes') : null;
    panel.innerHTML = notes ? notes.innerHTML : '<p class="ws-notes-empty">No notes on this slide.</p>';
  };
  document.addEventListener('keyup', function (e) {
    // Not while something is being typed into: a notes panel that opens when
    // you type an n into the zoom box is a panel nobody can use.
    var t = e.target || {};
    if (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable) { return; }
    if (e.key === 'n' || e.key === 'N') { panel.classList.toggle('open'); show(); }
    if (e.key === 'Escape') { panel.classList.remove('open'); }
  });
  document.addEventListener('ws:slide-change', show);
  show();
})();
</script>
{%endif%}
{%if mermaid%}
<script type="module">
  import mermaid from 'https://cdn.jsdelivr.net/npm/mermaid@10/dist/mermaid.esm.min.mjs';
  mermaid.initialize({ startOnLoad: true, maxTextSize: 2000000{%if mermaid_theme%}, theme: '{{mermaid_theme}}'{%endif%} });
</script>
{%endif%}
{%if wordcloud%}
<script src="https://cdnjs.cloudflare.com/ajax/libs/d3/7.8.5/d3.min.js"></script>
<script src="https://cdnjs.cloudflare.com/ajax/libs/d3-cloud/1.2.7/d3.layout.cloud.min.js"></script>
<script>
function wordcloud(name, words) {
  var layout = d3.layout.cloud()
    .size([800, 600])
    .words(words.map(function (d) { return { text: d, size: 20 + Math.random() * 70 }; }))
    .padding(5)
    .rotate(function () { return ~~(Math.random() * 1.5) * 90; })
    .font("Impact")
    .fontSize(function (d) { return d.size; })
    .on("end", function (words) {
      d3.select(name)
        .attr("width", layout.size()[0]).attr("height", layout.size()[1])
        .append("g")
        .attr("transform", "translate(" + layout.size()[0] / 2 + "," + layout.size()[1] / 2 + ")")
        .selectAll("text").data(words).enter().append("text")
        .style("font-size", function (d) { return d.size + "px"; })
        .style("fill", function () { return "hsl(" + Math.random() * 360 + ",72%,70%)"; })
        .style("font-family", "Impact")
        .attr("text-anchor", "middle")
        .attr("transform", function (d) { return "translate(" + [d.x, d.y] + ")rotate(" + d.rotate + ")"; })
        .text(function (d) { return d.text; });
    });
  layout.start();
}
</script>
{%endif%}
{%if backdrop%}
<style>
{%autoescape off%}
{{backdrop_css}}
{%endautoescape%}
</style>
<script>
{%autoescape off%}
window.ORG_BACKDROP = {{backdrop_config}};
{{backdrop_js}}
{%endautoescape%}
</script>
{%endif%}
{%if autofit%}
<style>
{%autoescape off%}
{{autofit_css}}
{%endautoescape%}
</style>
<script>
{%autoescape off%}
{{autofit_js}}
{%endautoescape%}
</script>
{%endif%}
</body>
</html>
