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
    function injectHeadingCollapse(nodes, lvl) {
        nodes.forEach((n, i) => {
            $title = $("#"+n.Id + "-title");
            $title.click(function (){
                $cnt = $("#"+n.Id + "-content");
                $cnt.slideToggle(30, function() {
                    if ($cnt.is(":visible")) {
                        $("#"+n.Id + "-heading-end").removeClass("folded");
                    } else {
                        $("#"+n.Id + "-heading-end").addClass("folded");
                    }
                });
            });
            if (n.Children && n.Children.length > 0) {
                injectHeadingCollapse(n.Children, lvl + 1);
            }
        });
    }

    function buildTreeView(nodes, lvl, parent) {
        nodes.forEach((n, i) => {
            console.log("NODE: ", n.Name);
            if (n.Children && n.Children.length > 0) {
                $elem = $("<li><span class=\"caret caret-down\">" + n.Name + "</span></li>");
                $ul = $("<ul class=\"nested active\"></ul>");
                $elem.append($ul);
                parent.append($elem);
                buildTreeView(n.Children, lvl + 1, $ul);
            } else {
                $elem = $("<li><span class=\"node-link\">" + n.Name + "</span></li>")
                parent.append($elem)
            }
        });
    }

    function activateTreeView() {
        var toggler = document.getElementsByClassName("caret");
        var i;

        for (i = 0; i < toggler.length; i++) {
            toggler[i].addEventListener("click", function() {
            this.parentElement.querySelector(".nested").classList.toggle("active");
            this.classList.toggle("caret-down");
            });
        } 
    }

    function onLoadHandler(){
        injectHeadingCollapse(nodes, 1);
        buildTreeView(nodes, 1, $("#navbar"));
        activateTreeView();
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
        <h1> DOCS </h1>
    	</div>
    </div>
    <div id="master-wrapper" class="master-wrapper clear">

    
    
     
    	<div id="sidebar" class="sidebar" style="padding-right: 20px; margin-left: 20px; cursor: pointer; overflow-y: auto; left: 0px; float: left; width: 340px; height: 1189px;">
            <h2>{{title}}</h2>
            <ul id="navbar" style="font: 16px / 135% 'Roboto', sans-serif;">
            </ul>
    	</div>
        <div>
        {%autoescape off%}
        {{html_data}}
        {%endautoescape%}
        </div>
    </div>


	<script type="module">
	  import mermaid from 'https://cdn.jsdelivr.net/npm/mermaid@10/dist/mermaid.esm.min.mjs';
	  mermaid.initialize({ startOnLoad: true });
	</script>
    {%autoescape off%}
	{{post_scripts}}
    {%endautoescape%}

</body>
</html>
