
<!DOCTYPE html>
<html>
<head>
  <link rel="stylesheet" href="https://fonts.googleapis.com/css2?family={{fontfamily}}"> 
  <link rel="stylesheet" href="{{hljs_cdn}}/styles/{{hljs_style}}.min.css">
  <style>
  table {
    border-collapse: collapse;
    margin: 25px 0;
    font-size: 0.9em;
    font-family: sans-serif;
    min-width: 400px;
    box-shadow: 0 0 20px rgba(0, 0, 0, 0.15);
	border-collapse:separate;	
	border-radius: 20px;
	
	}
	thead tr {
    background-color: #009879;
    color: #ffffff;
    text-align: left;
	border-radius:6px;
	}
	th, td {
	    padding: 12px 15px;
	}
	tbody tr {
 	   border-bottom: 1px solid #dddddd;
	}
	tbody tr:nth-of-type(even) {
	    background-color: #232323;
	}
	tbody tr:last-of-type {
 	   border-bottom: 2px solid #009879;
	}
	tbody tr.active-row {
	    font-weight: bold;
	    color: #009879;
	}
	img {
  		box-shadow: 5px 5px 15px 0px #aa8;
		-webkit-box-reflect: below 0px linear-gradient(to bottom, rgba(0,0,0,0.0), rgba(0,0,0,0.2));
	}
  </style>
<style>
{{stylesheet}}
</style>
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

</head>
<body>

	<script src="https://cdn.jsdelivr.net/npm/headjs@1.0.3/dist/1.0.0/head.min.js"></script>

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