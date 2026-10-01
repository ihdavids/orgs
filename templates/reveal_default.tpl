<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1, maximum-scale=1, user-scalable=no">
  <title>{{title}}</title>
  {%if fontfamily%}<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family={{fontfamily}}&display=swap">{%endif%}
  <link rel="stylesheet" href="{{reveal_cdn}}/reveal.min.css">
{%if not local_theme%}  <link rel="stylesheet" href="{{reveal_cdn}}/theme/{{theme}}.min.css" id="theme">{%endif%}
  <link rel="stylesheet" href="{{hljs_cdn}}/styles/{{hljs_style}}.min.css">
  {%if extra_css%}<link rel="stylesheet" href="{{extra_css}}">{%endif%}
  <style>
  {%autoescape off%}
  {{stylesheet}}
  {%endautoescape%}
  </style>
  {%if local_theme%}
  <!-- The theme: the four-framework mapping and then the theme itself. It
       comes after the exporter's own stylesheet on purpose - a theme is
       allowed to overrule the house style, that being what a theme is. -->
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
<body>

  <div class="reveal">
    <div class="slides" data-fit-box>
      {%autoescape off%}
      {{slide_data}}
      {%endautoescape%}
    </div>
  </div>

  <script src="{{reveal_cdn}}/reveal.min.js"></script>
  <!-- Plugins are script tags plus a `plugins: [...]` list, which is how
       reveal.js 4 and 5 take them. The `dependencies: [...]` option this
       template used to use was removed in reveal 4.0, so the speaker notes,
       the search and the zoom had quietly not been loading at all. -->
  {%for p in plugin_scripts%}
  <script src="{{reveal_cdn}}/plugin/{{p}}/{{p}}.min.js"></script>
  {%endfor%}
  <script>
    Reveal.initialize(Object.assign(
      {%autoescape off%}{{config}}{%endautoescape%},
      { plugins: [ {%autoescape off%}{{plugins}}{%endautoescape%} ] }
    ));
  </script>
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

  {%autoescape off%}
  {{post_scripts}}
  {%endautoescape%}
</body>
</html>
