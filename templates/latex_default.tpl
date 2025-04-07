\documentclass{{docclass_opts}}{% verbatim %}{{% endverbatim %}{{docclass}}{% verbatim %}}{% endverbatim %}
%\usepackage[utf8]{inputenc}
\usepackage{listings}
\usepackage{hyperref}
\usepackage{csquotes}
\usepackage{makecell}
\usepackage[T1]{fontenc}
\usepackage[greek,english]{babel}
\usepackage{CJKutf8}
\usepackage{graphicx}
% Required by minted for code coloring
\usepackage{pygmentize}
\usepackage{minted}
\usepackage{grffile}
\usepackage{longtable}
\usepackage{wrapfig}
\usepackage{rotating}
\usepackage{textcomp}
\usepackage{capt-of}
\usepackage{amsmath}
\usepackage{amssymb}
\usepackage[singlelinecheck=false]{caption}
\usepackage{shortvrb}
\usepackage{stfloats}
% Needed for strikethrough
\usepackage[normalem]{ulem}
% Checkbox Setup
\usepackage{enumitem,amssymb}
% DndBook required
\usepackage[english]{babel}
\usepackage[utf8]{inputenc}
\newlist{todolist}{itemize}{2}
\setlist[todolist]{label=$\square$}
\usepackage{pifont}
\newcommand{\cmark}{\ding{51}}%
\newcommand{\xmark}{\ding{55}}%
\newcommand{\tridot}{\ding{213}}%
\newcommand{\inp}{\rlap{$\square$}{\large\hspace{1pt}\tridot}}
\newcommand{\done}{\rlap{$\square$}{\raisebox{2pt}{\large\hspace{1pt}\cmark}}%
\hspace{-2.5pt}}
\newcommand{\wontfix}{\rlap{$\square$}{\large\hspace{1pt}\xmark}}


{%autoescape off%}
{{latex_data}}
{%endautoescape%}
