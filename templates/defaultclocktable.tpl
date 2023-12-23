{%table -%}
{%headers -%}
    {%h%}Title{%endh%}{%h%}Local{%endh%}{%h%}Total{%endh%}
{%- endheaders%}
{%- for i in data -%}
    {%- row -%}
        {%- c%}{{i.title}}{%endc%}{%c%}{{i.local}}{%endc%}{%c%}{{i.total}}{%- endc%}
    {%- endrow%}
{%- endfor %}
{%- endtable%}