<!-- Documentation template for html exporter -->
<!DOCTYPE html>
<html>
<head>
  <link rel="stylesheet" href="https://fonts.googleapis.com/css2?family={{fontfamily}}"> 
  <link rel="stylesheet" href="{{hljs_cdn}}/styles/{{hljs_style}}.min.css">
  <style>
	img {
  		box-shadow: 5px 5px 15px 0px #aa8;
		-webkit-box-reflect: below 0px linear-gradient(to bottom, rgba(0,0,0,0.0), rgba(0,0,0,0.2));
	}
  </style>
  <script src="https://cdnjs.cloudflare.com/ajax/libs/jquery/3.7.1/jquery.min.js" integrity="sha512-v2CJ7UaYy4JwqLDIrZUI/4hqeoQieOmAZNXBeQyjo21dadnwR+8ZaIJVT8EE2iyI61OV8e6M8PP2/4hpQINQ/g==" crossorigin="anonymous" referrerpolicy="no-referrer"></script>
{%if wordcloud%}
<script src="https://cdnjs.cloudflare.com/ajax/libs/d3/7.8.5/d3.min.js"></script>
<script src="https://cdnjs.cloudflare.com/ajax/libs/d3-cloud/1.2.7/d3.layout.cloud.min.js"></script>
<script>
function wordcloud(name, words) {
	function draw(words) {
  		d3.select(name)
  	    .attr("width", layout.size()[0])
  	    .attr("height", layout.size()[1])
  	  .append("g")
  	    .attr("transform", "translate(" + layout.size()[0] / 2 + "," + layout.size()[1] / 2 + ")")
  	  .selectAll("text")
  	    .data(words)
  	  .enter().append("text")
  	    .style("font-size", function(d) { return d.size + "px"; })
		.style("fill", function(d){return "hsl(" + Math.random() * 360 + ",72%,70%)"; })
  	    .style("font-family", "Impact")
  	    .attr("text-anchor", "middle")
  	    .attr("transform", function(d) {
  	      return "translate(" + [d.x, d.y] + ")rotate(" + d.rotate + ")";
  	    })
  	    .text(function(d) { return d.text; });
	}

	var layout = d3.layout.cloud()
	    .size([800, 600])
	    .words( words.map(function(d) {
      		return {text: d, size: 20 + Math.random() * 70};
    }))
    .padding(5)
    .rotate(function() { return ~~(Math.random() * 1.5) * 90; })
    .font("Impact")
    .fontSize(function(d) { return d.size; })
    .on("end", draw);

	layout.start();
}
</script>
{%endif%}
<script>
nodes = 
    {%autoescape off%}
    {{nodes_json}}
    {%endautoescape%}
    function injectHeadingCollapse(nodes) {
        nodes.forEach((n, i) => {
            console.log("NODE: ", n.Name);
            $("#"+n.Id + "-title").click(function (){
                $("#"+n.Id + "-content").slideToggle(50, function() {

                });
            });
            if (n.Children && n.Children.length > 0) {
                console.log("HAVE CHILDREN: ", n.Name);
                injectHeadingCollapse(n.Children);
            }
        });
    }

    function onLoadHandler(){
        injectHeadingCollapse(nodes);
    }
</script>

<style>
{%autoescape off%}
{{stylesheet}}
{%endautoescape%}
</style>
</head>
<body{%if havebodyattr%} class="{{bodyattr}}"{%endif%} onload="onLoadHandler()">
    <script src="https://cdn.jsdelivr.net/npm/headjs@1.0.3/dist/1.0.0/head.min.js"></script>
    <div class="header-wrapper">
    	<div id="header" class="header">
    	</div>
    </div>
    <div id="master-wrapper" class="master-wrapper clear">
    	<div id="sidebar" class="sidebar" style="left: 0px">
    	</div>
    </div>


    {%autoescape off%}
    {{html_data}}
    {%endautoescape%}

	<script type="module">
	  import mermaid from 'https://cdn.jsdelivr.net/npm/mermaid@10/dist/mermaid.esm.min.mjs';
	  mermaid.initialize({ startOnLoad: true });
	</script>
    {%autoescape off%}
	{{post_scripts}}
    {%endautoescape%}

</body>
</html>
