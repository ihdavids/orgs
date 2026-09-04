{% autoescape off %}{% verbatim %}%% ---------------------------------------------------------------------------
%% orgs D&D character sheet
%%
%% Rendered by the dndlatex / dndpdf exporters from an org character sheet.
%% Compile with: pdflatex sheet.tex
%%
%% The preamble is wrapped in a pongo2 verbatim block so that LaTeX macro
%% arguments (#1) and grouping braces cannot be mistaken for template syntax.
%% ---------------------------------------------------------------------------
\documentclass[10pt]{article}

\usepackage[T1]{fontenc}
\usepackage[utf8]{inputenc}
\usepackage[letterpaper,margin=0.5in,includefoot,footskip=14pt]{geometry}
\usepackage{xcolor}
\usepackage{tikz}
\usepackage{array}
\usepackage{tabularx}
\usepackage{multicol}
\usepackage{enumitem}
\usepackage{fancyhdr}
\usepackage{needspace}
\usetikzlibrary{calc}

% Fonts: pretty ones when the distribution has them, graceful fallback if not.
% (the flags are set first because macro parameters cannot appear inside the
%  branches of \IfFileExists)
\newif\ifdndgaramond
\newif\ifdndcinzel
\IfFileExists{ebgaramond.sty}{\dndgaramondtrue}{\dndgaramondfalse}
\IfFileExists{cinzel.sty}{\dndcinzeltrue}{\dndcinzelfalse}
\ifdndgaramond
  \usepackage[type1,lining]{ebgaramond}
\else
  \usepackage{lmodern}
\fi
\ifdndcinzel
  \usepackage[type1]{cinzel}
  \newcommand{\dndhead}[1]{{\cinzel #1}}
\else
  \newcommand{\dndhead}[1]{{\scshape #1}}
\fi

\definecolor{dndink}{HTML}{1C1A17}
\definecolor{dndaccent}{HTML}{7B1B1B}
\definecolor{dndgold}{HTML}{9A751B}
\definecolor{dndline}{HTML}{B3A585}
\definecolor{dndpaper}{HTML}{FBF7EE}
\definecolor{dndpanel}{HTML}{F2EBDC}
\definecolor{dndmuted}{HTML}{6B6355}

\color{dndink}
\pagecolor{dndpaper}
\setlength{\parindent}{0pt}
\setlength{\columnsep}{14pt}
\renewcommand{\arraystretch}{1.12}

\pagestyle{fancy}
\fancyhf{}
\renewcommand{\headrulewidth}{0pt}
\renewcommand{\footrulewidth}{0.4pt}
\renewcommand{\footrule}{\color{dndline}\hrule width\headwidth height\footrulewidth}

% ---------------------------------------------------------------------------
% Building blocks
% ---------------------------------------------------------------------------

\tikzset{
  panel/.style={draw=dndline, line width=0.9pt, rounded corners=2.5pt,
                fill=dndpanel, inner sep=4pt},
  tile/.style={draw=dndline, line width=0.9pt, rounded corners=3pt,
               fill=dndpaper, inner sep=4pt, align=center},
  shieldtile/.style={draw=dndaccent, line width=1.1pt, rounded corners=3pt,
                     fill=dndpaper, inner sep=4pt, align=center},
  caption/.style={fill=dndpanel, inner xsep=4pt, inner ysep=1pt,
                  font=\scriptsize\bfseries, text=dndaccent},
}

\newsavebox{\dndbuf}

% \sheetbox{width}{title}{body}
\newcommand{\sheetbox}[3]{%
  \begin{lrbox}{\dndbuf}%
    \begin{minipage}{#1}#3\end{minipage}%
  \end{lrbox}%
  \begin{tikzpicture}
    \node[panel] (bx) {\usebox{\dndbuf}};
    \node[caption, anchor=west] at ([xshift=10pt]bx.north west) {\dndhead{\MakeUppercase{#2}}};
  \end{tikzpicture}\par\vspace{3pt}%
}

% \plainbox{width}{body} - a box with no title
\newcommand{\plainbox}[2]{%
  \begin{lrbox}{\dndbuf}%
    \begin{minipage}{#1}#2\end{minipage}%
  \end{lrbox}%
  \begin{tikzpicture}\node[panel] (bx) {\usebox{\dndbuf}};\end{tikzpicture}\par\vspace{5pt}%
}

% \abilitytile{name}{mod}{score}
\newcommand{\abilitytile}[3]{%
  \begin{tikzpicture}
    \node[tile, minimum width=1.0in, minimum height=0.5in] (a)
      {\begin{tabular}{c}
         {\scriptsize\dndhead{#1}}\\[-4pt]
         {\fontsize{17}{17}\selectfont\bfseries #2}\\[1pt]
       \end{tabular}};
    \node[draw=dndline, line width=0.8pt, circle, fill=dndpanel, inner sep=1pt,
          minimum size=14pt, font=\scriptsize] at (a.south) {#3};
  \end{tikzpicture}%
}

% \stattile{width}{label}{value}{sub}
\newcommand{\stattile}[4]{%
  \begin{tikzpicture}
    \node[tile, minimum width=#1, minimum height=0.46in]
      {\begin{tabular}{c}
         {\scriptsize\dndhead{#2}}\\[-4pt]
         {\fontsize{15}{15}\selectfont\bfseries #3}\\[-3pt]
         {\tiny\color{dndmuted} #4}
       \end{tabular}};
  \end{tikzpicture}%
}

% \shieldtileX{width}{label}{value}{sub} - the armour class tile
\newcommand{\shieldtileX}[4]{%
  \begin{tikzpicture}
    \node[shieldtile, minimum width=#1, minimum height=0.46in]
      {\begin{tabular}{c}
         {\scriptsize\dndhead{#2}}\\[-4pt]
         {\fontsize{15}{15}\selectfont\bfseries #3}\\[-3pt]
         {\tiny\color{dndmuted} #4}
       \end{tabular}};
  \end{tikzpicture}%
}

% proficiency pips
\newcommand{\pipon}{\tikz[baseline=-0.5ex]\node[draw=dndink, line width=0.6pt, circle,
  fill=dndink, inner sep=1.6pt]{};}
\newcommand{\pipoff}{\tikz[baseline=-0.5ex]\node[draw=dndmuted, line width=0.6pt, circle,
  fill=dndpaper, inner sep=1.6pt]{};}
\newcommand{\pipexp}{\tikz[baseline=-0.5ex]\node[draw=dndgold, line width=1pt, circle,
  fill=dndgold, inner sep=1.6pt]{};}
\newcommand{\slotbox}{\tikz[baseline=-0.4ex]\node[draw=dndmuted, line width=0.6pt,
  rounded corners=1pt, minimum size=7pt, inner sep=0pt, fill=dndpaper]{};}
\newcommand{\slotboxused}{\tikz[baseline=-0.4ex]\node[draw=dndmuted, line width=0.6pt,
  rounded corners=1pt, minimum size=7pt, inner sep=0pt, fill=dndmuted]{};}

% \statline{pip}{name}{value}
\newcommand{\statline}[3]{%
  \makebox[10pt][l]{#1}\,%
  \parbox[t]{\dimexpr\linewidth-52pt\relax}{\raggedright #2}%
  \hfill\makebox[32pt][r]{\textbf{#3}}\par%
}

% headed small caps label
\newcommand{\fieldlabel}[1]{{\scriptsize\color{dndmuted}\dndhead{\MakeUppercase{#1}}}}
\newcommand{\fieldbox}[2]{%
  \parbox[t]{\linewidth}{\fieldlabel{#1}\\[1pt] #2}\par\vspace{3pt}%
}

% a rule that separates entries inside a box
\newcommand{\boxrule}{\vspace{2pt}{\color{dndline}\hrule height 0.4pt}\vspace{3pt}}

\newcommand{\featureentry}[3]{%
  \needspace{2\baselineskip}%
  {\bfseries\color{dndaccent} #1}\ {\tiny\color{dndmuted}\dndhead{#2}}\par
  \vspace{1pt}{\small #3}\par\vspace{5pt}%
}

\newlength{\colA}\newlength{\colB}\newlength{\colC}
\newlength{\colAin}\newlength{\colBin}\newlength{\colCin}
\setlength{\colA}{0.295\textwidth}
\setlength{\colB}{0.345\textwidth}
\setlength{\colC}{0.325\textwidth}
\setlength{\colAin}{\dimexpr\colA-16pt\relax}
\setlength{\colBin}{\dimexpr\colB-16pt\relax}
\setlength{\colCin}{\dimexpr\colC-16pt\relax}
{% endverbatim %}

\fancyfoot[L]{\scriptsize\color{dndmuted} {{ sheet.name }}}
\fancyfoot[C]{\scriptsize\color{dndmuted} \thepage}
\fancyfoot[R]{\scriptsize\color{dndmuted} {{ sheet.classLine }}}

\begin{document}

%% ===========================================================================
%% Page 1 - the character sheet proper
%% ===========================================================================

\begin{tikzpicture}
  \node[panel, minimum width=\textwidth, inner sep=7pt] (hdr) %
    {\begin{minipage}{\dimexpr\textwidth-20pt\relax}
      {\fontsize{22}{24}\selectfont\bfseries\color{dndaccent}\dndhead{ {{ sheet.name }} }}\\[2pt]
      {\small\color{dndmuted} {{ sheet.classLine }}{% if sheet.raceName %} \textbullet\ {{ sheet.raceName }}{% endif %}{% if sheet.background %} \textbullet\ {{ sheet.background }}{% endif %}{% if sheet.alignment %} \textbullet\ {{ sheet.alignment }}{% endif %} }
    \end{minipage}};
\end{tikzpicture}\par\vspace{4pt}

\noindent
\stattile{0.155\textwidth}{Level}{ {{ sheet.level }} }{ {{ sheet.proficiencyStr }} proficiency }\hfill
\stattile{0.155\textwidth}{Hit Dice}{ {{ sheet.hitDice }} }{ {{ sheet.hitDiceUsed }} spent }\hfill
\stattile{0.155\textwidth}{Experience}{ {{ sheet.xp }} }{ next {{ sheet.nextLevelXp }} }\hfill
\stattile{0.155\textwidth}{Inspiration}{ {% if sheet.inspiration %}Yes{% else %}--{% endif %} }{ }\hfill
\stattile{0.155\textwidth}{Player}{ {\small {% if sheet.player %}{{ sheet.player }}{% else %}--{% endif %} } }{ }\hfill
\stattile{0.155\textwidth}{Size}{ {\small {{ sheet.size }} } }{ {% if sheet.darkvision %}darkvision {{ sheet.darkvision }} ft{% else %}no darkvision{% endif %} }
\par\vspace{7pt}

\noindent
%% --------------------------------------------------------------- column 1
\begin{minipage}[t]{\colA}\vspace*{0pt}
{% for a in sheet.abilities %}\abilitytile{ {{ a.short }} }{ {{ a.mod }} }{ {{ a.score }} }{% if forloop.Counter|divisibleby:2 %}\par\vspace{3pt}{% else %}\hfill{% endif %}
{% endfor %}
\vspace{4pt}

\sheetbox{\colAin}{Saving Throws}%
{\small{% for a in sheet.abilities %}\statline{ {% if a.saveProf %}\pipon{% else %}\pipoff{% endif %} }{ {{ a.name }} }{ {{ a.saveStr }} }
{% endfor %}}

\sheetbox{\colAin}{Skills}%
{\small{% for s in sheet.skills %}\statline{ {% if s.expertise %}\pipexp{% elif s.proficient %}\pipon{% else %}\pipoff{% endif %} }{ {{ s.name }} {\tiny\color{dndmuted} {{ s.short }} } }{ {{ s.mod }} }
{% endfor %}\boxrule
\statline{}{ Passive Perception }{ {{ sheet.passivePerception }} }
\statline{}{ Passive Insight }{ {{ sheet.passiveInsight }} }
\statline{}{ Passive Investigation }{ {{ sheet.passiveInvestigation }} }}

\end{minipage}\hfill
%% --------------------------------------------------------------- column 2
\begin{minipage}[t]{\colB}\vspace*{0pt}
\shieldtileX{0.29\colB}{Armor Class}{ {{ sheet.ac }} }{ }\hfill
\stattile{0.29\colB}{Initiative}{ {{ sheet.initiativeStr }} }{ }\hfill
\stattile{0.29\colB}{Speed}{ {{ sheet.speed }} }{ feet }
\par\vspace{3pt}
{\tiny\color{dndmuted} armour class from {{ sheet.acSource }} }
\par\vspace{4pt}

\sheetbox{\colBin}{Hit Points}%
{\fieldlabel{Maximum}\hfill\textbf{ {{ sheet.hpMax }} }\par
\fieldlabel{Current}\hfill\textbf{ {{ sheet.hpCurrent }} }\par
\fieldlabel{Temporary}\hfill\textbf{ {{ sheet.hpTemp }} }\par
\boxrule
\fieldlabel{Death Saves}\hfill {\small successes \pipoff\pipoff\pipoff\quad failures \pipoff\pipoff\pipoff}\par}

\sheetbox{\colBin}{Attacks}%
{\begin{tabularx}{\linewidth}{@{}Xll@{}}
{\scriptsize\dndhead{ATTACK}} & {\scriptsize\dndhead{BONUS}} & {\scriptsize\dndhead{DAMAGE}}\\[1pt]
\hline\\[-7pt]
{% for a in sheet.attacks %}{\small {{ a.name }} } & {\small {{ a.bonus }} } & {\small {{ a.damage }} {\tiny {{ a.type }} } }\\
{% endfor %}\end{tabularx}
{% for a in sheet.attacks %}{% if a.notes %}{\tiny\color{dndmuted} {{ a.name }}: {{ a.notes }} }\par
{% endif %}{% endfor %}}

\sheetbox{\colBin}{Equipment}%
{\begin{tabularx}{\linewidth}{@{}Xrc@{}}
{\scriptsize\dndhead{ITEM}} & {\scriptsize\dndhead{QTY}} & {\scriptsize\dndhead{WORN}}\\[1pt]
\hline\\[-7pt]
{% for g in sheet.equipment %}{% if forloop.Counter <= 20 %}{\small {{ g.name }} } & {\small {{ g.qty }} } & {% if g.equipped %}\pipon{% endif %}\\
{% endif %}{% endfor %}\end{tabularx}
{% if sheet.equipment|length > 20 %}{\tiny\color{dndmuted} the rest of your gear is listed in full overleaf}\par{% endif %}
\boxrule
{\small {{ sheet.money.cp }} cp \textbullet\ {{ sheet.money.sp }} sp \textbullet\ {{ sheet.money.ep }} ep \textbullet\ {{ sheet.money.gp }} gp \textbullet\ {{ sheet.money.pp }} pp}\par
{\tiny\color{dndmuted} carrying {{ sheet.weight }} lb of {{ sheet.carryCapacity }} lb capacity, push/drag/lift {{ sheet.pushDragLift }} lb}}
\end{minipage}\hfill
%% --------------------------------------------------------------- column 3
\begin{minipage}[t]{\colC}\vspace*{0pt}
{% if sheet.isCaster %}\sheetbox{\colCin}{Spellcasting}%
{\statline{}{ Ability }{ {{ sheet.castingAbility }} }
\statline{}{ Spell save DC }{ {{ sheet.spellSaveDc }} }
\statline{}{ Spell attack bonus }{ {{ sheet.spellAttackStr }} }
{% if sheet.preparedMax %}\statline{}{ Spells prepared }{ {{ sheet.spellsPrepared }}/{{ sheet.preparedMax }} }
{% endif %}{% if sheet.cantripsKnown %}\statline{}{ Cantrips known }{ {{ sheet.cantripsKnown }} }
{% endif %}{% if sheet.spellsKnown %}\statline{}{ Spells known }{ {{ sheet.spellsKnown }} }
{% endif %}{% if sheet.slots %}\boxrule
{% for s in sheet.slots %}{\scriptsize\dndhead{ {{ s.label }} } }\ {% for p in s.pips %}{% if p <= s.used %}\slotboxused{% else %}\slotbox{% endif %}\,{% endfor %}\par
{% endfor %}{% endif %}}
{% endif %}
\sheetbox{\colCin}{Personality}%
{ {% if sheet.personality %}\fieldbox{Traits}{\small\itshape {{ sheet.personality }} }{% endif %}
{% if sheet.ideals %}\fieldbox{Ideals}{\small\itshape {{ sheet.ideals }} }{% endif %}
{% if sheet.bonds %}\fieldbox{Bonds}{\small\itshape {{ sheet.bonds }} }{% endif %}
{% if sheet.flaws %}\fieldbox{Flaws}{\small\itshape {{ sheet.flaws }} }{% endif %}
{% if not sheet.personality and not sheet.ideals and not sheet.bonds and not sheet.flaws %}{\small\color{dndmuted} Not written yet.}{% endif %}}

\sheetbox{\colCin}{Proficiencies \& Languages}%
{\footnotesize
\fieldbox{Armor}{ {% if sheet.armorProficiencies %}{{ sheet.armorProficiencies|join:", " }}{% else %}none{% endif %} }
\fieldbox{Weapons}{ {% if sheet.weaponProficiencies %}{{ sheet.weaponProficiencies|join:", " }}{% else %}none{% endif %} }
\fieldbox{Tools}{ {% if sheet.toolProficiencies %}{{ sheet.toolProficiencies|join:", " }}{% else %}none{% endif %} }
\fieldbox{Languages}{ {% if sheet.languages %}{{ sheet.languages|join:", " }}{% else %}none{% endif %} }}

\sheetbox{\colCin}{Features \& Traits}%
{\scriptsize\raggedright
{% for t in sheet.traits %}\textbullet\ {{ t.name }}\par
{% endfor %}{% for f in sheet.features %}\textbullet\ {{ f.name }} {\color{dndmuted} {{ f.source }} }\par
{% endfor %}\boxrule
{\color{dndmuted} Full descriptions overleaf.}}

{% if sheet.age or sheet.height or sheet.weightStr or sheet.eyes or sheet.skin or sheet.hair or sheet.appearance %}\sheetbox{\colCin}{Appearance}%
{ {% if sheet.age %}\statline{}{ Age }{ {{ sheet.age }} }{% endif %}
{% if sheet.height %}\statline{}{ Height }{ {{ sheet.height }} }{% endif %}
{% if sheet.weightStr %}\statline{}{ Weight }{ {{ sheet.weightStr }} }{% endif %}
{% if sheet.eyes %}\statline{}{ Eyes }{ {{ sheet.eyes }} }{% endif %}
{% if sheet.skin %}\statline{}{ Skin }{ {{ sheet.skin }} }{% endif %}
{% if sheet.hair %}\statline{}{ Hair }{ {{ sheet.hair }} }{% endif %}
{% if sheet.appearance %}\par{\small\itshape {{ sheet.appearance }} }{% endif %}}
{% endif %}
\end{minipage}

%% ===========================================================================
%% Page 2 - features, traits and the character's story
%% ===========================================================================
\clearpage

\begin{tikzpicture}
  \node[panel, minimum width=\textwidth, inner sep=6pt] %
    {\begin{minipage}{\dimexpr\textwidth-20pt\relax}
      {\large\bfseries\color{dndaccent}\dndhead{FEATURES \& TRAITS}}\hfill
      {\small\color{dndmuted} {{ sheet.name }} \textbullet\ {{ sheet.classLine }} }
    \end{minipage}};
\end{tikzpicture}\par\vspace{7pt}

\begin{multicols}{2}
{% for t in sheet.traits %}\featureentry{ {{ t.name }} }{ {{ t.source }} }{ {{ t.text }} }
{% endfor %}{% for f in sheet.features %}\featureentry{ {{ f.name }} }{ {{ f.source }} }{ {{ f.text }} }
{% endfor %}\end{multicols}

{% if sheet.equipment|length > 20 %}
\vspace{6pt}
\sheetbox{\dimexpr\textwidth-12pt\relax}{Full Equipment List}%
{\footnotesize\begin{multicols}{3}
{% for g in sheet.equipment %}\textbullet\ {{ g.name }}{% if g.qty > 1 %} $\times${{ g.qty }}{% endif %}{% if g.equipped %} (worn){% endif %}\par
{% endfor %}\end{multicols}}
{% endif %}

{% if sheet.backstory or sheet.allies or sheet.treasure or sheet.notes %}
\vspace{6pt}
\begin{tikzpicture}
  \node[panel, minimum width=\textwidth, inner sep=6pt] %
    {\begin{minipage}{\dimexpr\textwidth-20pt\relax}
      {\large\bfseries\color{dndaccent}\dndhead{THE CHARACTER}}
    \end{minipage}};
\end{tikzpicture}\par\vspace{7pt}
{% if sheet.backstory %}\fieldbox{Backstory}{ {{ sheet.backstory }} }{% endif %}
{% if sheet.allies %}\fieldbox{Allies \& Organizations}{ {{ sheet.allies }} }{% endif %}
{% if sheet.treasure %}\fieldbox{Treasure}{ {{ sheet.treasure }} }{% endif %}
{% if sheet.notes %}\fieldbox{Notes}{ {{ sheet.notes }} }{% endif %}
{% endif %}

{% if sheet.isCaster %}
%% ===========================================================================
%% Page 3 - spellcasting
%% ===========================================================================
\clearpage

\begin{tikzpicture}
  \node[panel, minimum width=\textwidth, inner sep=6pt] %
    {\begin{minipage}{\dimexpr\textwidth-20pt\relax}
      {\large\bfseries\color{dndaccent}\dndhead{SPELLCASTING}}\hfill
      {\small\color{dndmuted} {{ sheet.castingAbilityName }} \textbullet\ save DC {{ sheet.spellSaveDc }} \textbullet\ attack {{ sheet.spellAttackStr }} }
    \end{minipage}};
\end{tikzpicture}\par\vspace{6pt}

{% if sheet.spellNotes %}{\small\color{dndmuted} {{ sheet.spellNotes }} }\par\vspace{4pt}{% endif %}

{% for lvl in sheet.spellLevels %}
\needspace{4\baselineskip}
{\bfseries\color{dndaccent}\dndhead{ {{ lvl.name }} }}%
{% if lvl.slots %}\quad{\small\color{dndmuted} {{ lvl.slots }} slot{% if lvl.slots > 1 %}s{% endif %}}\ {% for s in sheet.slots %}{% if s.level == lvl.level %}{% for p in s.pips %}{% if p <= s.used %}\slotboxused{% else %}\slotbox{% endif %}\,{% endfor %}{% endif %}{% endfor %}{% endif %}
\par\vspace{1pt}{\color{dndline}\hrule height 0.4pt}\vspace{3pt}

\begin{tabularx}{\textwidth}{@{}p{0.8em}p{9em}p{5.5em}p{6em}p{4.5em}X@{}}
{\scriptsize\dndhead{P}} & {\scriptsize\dndhead{SPELL}} & {\scriptsize\dndhead{TIME}} &
{\scriptsize\dndhead{RANGE}} & {\scriptsize\dndhead{COMP}} & {\scriptsize\dndhead{DURATION}}\\[1pt]
{% for sp in lvl.spells %}{% if lvl.level > 0 %}{% if sp.prepared %}\pipon{% else %}\pipoff{% endif %}{% endif %} &
{\small {{ sp.name }} } & {\tiny {{ sp.castingTime }} } & {\tiny {{ sp.range }} } &
{\tiny {{ sp.components }} } & {\tiny {{ sp.duration }}{% if sp.concentration %}, conc.{% endif %}{% if sp.ritual %}, ritual{% endif %} }\\
{% endfor %}\end{tabularx}
\vspace{6pt}
{% endfor %}

\vspace{4pt}
\begin{tikzpicture}
  \node[panel, minimum width=\textwidth, inner sep=6pt] %
    {\begin{minipage}{\dimexpr\textwidth-20pt\relax}
      {\large\bfseries\color{dndaccent}\dndhead{SPELL DESCRIPTIONS}}
    \end{minipage}};
\end{tikzpicture}\par\vspace{6pt}

\begin{multicols}{2}
{% for lvl in sheet.spellLevels %}{% for sp in lvl.spells %}\needspace{3\baselineskip}%
{\bfseries\color{dndaccent} {{ sp.name }} }\ {\tiny\color{dndmuted}\itshape {{ sp.school }}{% if sp.level %}, level {{ sp.level }}{% else %} cantrip{% endif %} }\par
{\tiny\color{dndmuted} {{ sp.castingTime }} \textbullet\ {{ sp.range }} \textbullet\ {{ sp.components }} \textbullet\ {{ sp.duration }} }\par
\vspace{1pt}{\small {{ sp.text }} }\par
{% if sp.higherLevel %}{\small\itshape At higher levels. {{ sp.higherLevel }} }\par{% endif %}
\vspace{5pt}
{% endfor %}{% endfor %}\end{multicols}
{% endif %}

{% if sheet.warnings %}
\vspace{6pt}
\sheetbox{\dimexpr\textwidth-16pt\relax}{Sheet Warnings}%
{ {% for w in sheet.warnings %}{\small\color{dndaccent} \textbullet\ {{ w }} }\par
{% endfor %}}
{% endif %}

\end{document}
{% endautoescape %}
