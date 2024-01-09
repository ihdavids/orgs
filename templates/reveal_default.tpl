<!DOCTYPE html>
<html>
<head>
  <link rel="stylesheet" href="https://fonts.googleapis.com/css2?family={{fontfamily}}"> 
  <link rel="stylesheet" href="{{reveal_cdn}}/reveal.min.css">
  <link rel="stylesheet" href="{{reveal_cdn}}/theme/{{theme}}.css">
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
  </style>
<style>
{{stylesheet}}
</style>
</head>
<body>

	<div class="reveal">
		<div class="slides">
        {%autoescape off%}
        {{slide_data}}
        {%endautoescape%}
        </div>
    </div>
	<script src="https://cdn.jsdelivr.net/npm/headjs@1.0.3/dist/1.0.0/head.min.js"></script>
	<script src="{{reveal_cdn}}/reveal.min.js"></script>
	<script src="{{reveal_cdn}}/plugin/highlight/highlight.min.js"></script>


	<!--<script src="index.js"></script>-->
	<script>
		// More info about config & dependencies:
		// - https://github.com/hakimel/reveal.js#configuration
		// - https://github.com/hakimel/reveal.js#dependencies
		Reveal.initialize({
			center: false,
			navigationMode: "grid",
			dependencies: [
				{ src: '{{reveal_cdn}}/plugin/markdown/markdown.min.js' },
				{ src: '{{reveal_cdn}}/plugin/notes/notes.min.js', async: true },
				{ src: '{{reveal_cdn}}/plugin/math/math.min.js', async: true },
				{ src: '{{reveal_cdn}}/plugin/search/search.min.js', async: true },
				{ src: '{{reveal_cdn}}/plugin/zoom/zoom.min.js', async: true },
				//{ src: '{{reveal_cdn}}/plugin/highlight/highlight.min.js', async: true},
				//{ src: '{{reveal_cdn}}/plugin/highlight/highlight.min.js', callback: function () { hljs.initHighlightingOnLoad(); } },
				//{ src: '//cdn.socket.io/socket.io-1.3.5.js', async: true },
				//{ src: 'plugin/multiplex/master.js', async: true },
				// and if you want speaker notes
				//{ src: '{{reveal_cdn}}/plugin/notes-server/client.js', async: true }

			],
			markdown: {
				//            renderer: myrenderer,
				smartypants: true
			},
			plugins: [RevealHighlight]
		});
		Reveal.configure({
			// PDF Configurations
			pdfMaxPagesPerSlide: 1

		});
		Reveal.getPlugins();
	</script>
	<script type="module">
	  import mermaid from 'https://cdn.jsdelivr.net/npm/mermaid@10/dist/mermaid.esm.min.mjs';
	  mermaid.initialize({ startOnLoad: true });
	</script>

    {%autoescape off%}
	{{post_scripts}}
    {%endautoescape%}

</body>
</html>