<!DOCTYPE html>
<html>
<head>
  <style>
	.impress-supported .fallback-message {
	    display: none;
	}
	/*
    Now let's style the presentation steps.

    We start with basics to make sure it displays correctly in everywhere ...

    width: 900px;
*/

.step {
    position: relative;
    padding: 10px;
    margin: 10px auto;
	min-width: 1024px;

    -webkit-box-sizing: border-box;
    -moz-box-sizing:    border-box;
    -ms-box-sizing:     border-box;
    -o-box-sizing:      border-box;
    box-sizing:         border-box;

    font-family: 'PT Serif', georgia, serif;
    font-size: 48px;
    line-height: 1;
    text-shadow: 0 2px 2px rgba(0, 0, 0, .1);
}


/*
    ... and we enhance the styles for impress.js.

    Basically we remove the margin and make inactive steps a little bit transparent.
*/
.impress-enabled .step {
    margin: 0;
    opacity: 0.3;

    -webkit-transition: opacity 1s;
    -moz-transition:    opacity 1s;
    -ms-transition:     opacity 1s;
    -o-transition:      opacity 1s;
    transition:         opacity 1s;
}

.impress-enabled .step.active { opacity: 1 }

/*
    These 'slide' step styles were heavily inspired by HTML5 Slides:
    http://html5slides.googlecode.com/svn/trunk/styles.css

    ;)

    They cover everything what you see on first three steps of the demo.

    All impress.js steps are wrapped inside a div element of 0 size! This means that relative
    values for width and height (example: width: 100%) will not work. You need to use pixel
    values. The pixel values used here correspond to the data-width and data-height given to the
    #impress root element. When the presentation is viewed on a larger or smaller screen, impress.js
    will automatically scale the steps to fit the screen.
*/
.slide {
    display: block;

    width: 900px;
    height: 700px;
    padding: 40px 60px;

    background-color: white;
    border: 1px solid rgba(0, 0, 0, .3);
    border-radius: 10px;
    box-shadow: 0 2px 6px rgba(0, 0, 0, .1);

    color: rgb(102, 102, 102);
    text-shadow: 0 2px 2px rgba(0, 0, 0, .1);

    font-family: 'Open Sans', Arial, sans-serif;
    font-size: 30px;
    line-height: 36px;
    letter-spacing: -1px;
}

.slide q {
    display: block;
    font-size: 50px;
    line-height: 72px;

    margin-top: 100px;
}

.slide q strong {
    white-space: nowrap;
}

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
  {%autoescape off%}
  {{themedata}}
  {%endautoescape%}
  </style>
  <link rel="stylesheet" href="https://fonts.googleapis.com/css2?family={{fontfamily}}"> 
  <link rel="stylesheet" href="{{hljs_cdn}}/styles/{{hljs_style}}.min.css">
  <link rel="stylesheet" href="{{impress_cdn}}/css/impress-common.css">
  <meta charset="utf-8" />
    <meta name="viewport" content="width=1024" />
    <meta name="apple-mobile-web-app-capable" content="yes" />
</head>
<body class="impress-not-supported"
    data-transition-duration="500"
    data-width="1024"
    data-height="768"
    data-max-scale="3"
    data-min-scale="0"
    data-perspective="100"
>
<div class="fallback-message">
    <p>Your browser <b>doesn't support the features required</b> by impress.js, so you are presented with a simplified version of this presentation.</p>
    <p>For the best experience please use the latest <b>Chrome</b>, <b>Safari</b> or <b>Firefox</b> browser.</p>
</div>
	<div id="impress">
        {%autoescape off%}
        {{slide_data}}
        {%endautoescape%}
	</div>
	<div id="impress-toolbar"></div>
	<div class="impress-progressbar"><div></div></div>
	<div class="impress-progress"></div>
	<script>
	if ("ontouchstart" in document.documentElement) { 
	    document.querySelector(".hint").innerHTML = "<p>Swipe left or right to navigate</p>";
	}
	</script>
	<script src="{{hljs_cdn}}/highlight.min.js"></script>
	<script src="https://cdn.jsdelivr.net/npm/headjs@1.0.3/dist/1.0.0/head.min.js"></script>
	<script src="{{impress_cdn}}/js/impress.js"></script>
    <script>impress().init();</script>
	<script>hljs.highlightAll();</script>
	<script>impress.addPreInitPlugin( rel );</script>
	<script type="module">
	  import mermaid from 'https://cdn.jsdelivr.net/npm/mermaid@10/dist/mermaid.esm.min.mjs';
	  mermaid.initialize({ startOnLoad: true });
	</script>
</body>
</html>