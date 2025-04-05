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
  <link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/font-awesome/4.7.0/css/font-awesome.min.css">

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

    function findParent(n, treeNodes) {
        for (var i = 0; i < treeNodes.length; ++i) {
            var x = treeNodes[i];
            if (x.Id === n.Parent) {
                return x;
            }
            if (x.Children && x.Children.length > 0) {
                var v = findParent(n, x.Children);
                if (v !== null) {
                    return v;
                }
            }
        }
        return null;
    }

    function showParent(n) {
        if (n.Parent !== "") {
            console.log("Looking for parent of: ", n.Name);
            var p = findParent(n, nodes);
            if (p !== null) {
                console.log("Parent: ", p.Name);
                $("#"+p.Id).show();
                showParent(p);
            }
        }
    }

    function showRecursive(n) {
        //console.log("SHOWING: ", n.Name);
        $("#"+n.Id).show();
        $("#"+n.Id + "-text").show();
        showParent(n);

        //if (n.Children && n.Children.length > 0) {
        //    n.Children.forEach((c,i) => {
        //        showRecursive(c);
        //    })
        //}
    }

    function hideEverything(n) {
        //console.log("HIDING: ", n.Name);
        $("#searchoutput").hide();
        $("#searchoutput").html("");
        $("#"+n.Id).hide();
        $("#"+n.Id + "-text").hide();
        if (n.Children && n.Children.length > 0) {
            n.Children.forEach((c,i) => {
                hideEverything(c);
            })
        }
    }

    function showNode(n) {
        let me = n;
        nodes.forEach((xx,i) => {hideEverything(xx); });
        showRecursive(me);
    }

    function buildTreeView(treeNodes, lvl, parent) {
        treeNodes.forEach((n, i) => {
            if (n.Children && n.Children.length > 0) {
                var $elem = $("<li><span class=\"caret caret-down\">" + n.Name + "</span></li>");
                var $ul = $("<ul class=\"nested active\"></ul>");
                $elem.append($ul);
                parent.append($elem);
                buildTreeView(n.Children, lvl + 1, $ul);
                $elem.click(function (event) {
                    showNode(n);
                    event.stopPropagation();
                });
                $("#"+n.Id).hide();
            } else {
                $elem = $("<li><span class=\"node-link\">" + n.Name + "</span></li>")
                $elem.click(function (event) {
                    showNode(n);
                    event.stopPropagation();
                });
                parent.append($elem);
                $("#"+n.Id).hide();
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
        showRecursive(nodes[0]);
    }

    function onLoadHandler(){
        injectHeadingCollapse(nodes, 1);
        buildTreeView(nodes, 1, $("#navbar"));
        activateTreeView();
    }

    function searchbar() {
        nodes.forEach((xx,i) => {hideEverything(xx); });
        const toFind = $("#searchfield").val();
        const re = new RegExp(toFind); 

        var elem = "";
        var allHeadings = $(".heading-content-text");
        allHeadings.each(function () {
            const content = $(this).html();
            const id = $(this).parent.id();
            console.log(content);
            var m = content.match(re);
            if (m) {
                elem += content + " " + id;
            }
        });
        for (i = 0; i < allHeadings.length; i++) {
        } 
        var elem = $(elem);
        


        $("#searchoutput").append(elem);
        $("#searchoutput").show();
        return false;
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
    <div class="doc-wrapper">
     <div class="header-wrapper">
    	<div id="header" class="header">
            <div class="header-container">
                <div class="header-left"><h1 class="header">DOCS</h1></div>
                <div class="header-right" style="justify-content: flex-end; align-self: stretch; flex-grow: 4;">
                    <div style="float: right;">
                    <form onSubmit="return searchbar();">
                    <input id="searchfield" name="searchfield" style="border-radius: 5px; padding: 6px; padding-right: 0px; border: none; margin-top: 10px; margin-right: 6px; font-size: 10px" type="text" placeholder="Search..."/>
                    <button type="submit" style="border-radius: 5px; float: right; padding: 6px 10px; margin-top: 10px; margin-right: 6px; background: #ddd; font-size: 10px; border:none; cursor: pointer;"><i class="fa fa-search"></i></button>
                    </form>
                    </div>
                </div>
            </div>
    	</div>
      </div>
      <div id="master-wrapper" class="master-wrapper clear">
    	<div id="sidebar" class="sidebar" style="padding-right: 0px; margin-right: 0px; margin-left: 1px; cursor: pointer; overflow-y: auto; left: 0px; float: left; width: 25%; min-width: 150px; height: 1189px;">
            <h2>{{title}}</h2>
            <ul id="navbar" style="font: 16px / 135% 'Roboto', sans-serif;">
            </ul>
    	</div>
        <div class="display-box">
            <div id="searchoutput" style="">
            </div>
        {%autoescape off%}
        {{html_data}}
        {%endautoescape%}
        </div>
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
