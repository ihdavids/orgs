
#+JIRA_QUERY:     {{query}}
#+JIRA_FIELDS:    {{fields}}
#+GENERATED_FROM: {{endpoint}}
#+GENERATED_ON:   {{datetime}}

{% for i in data.Issues %}* {% if i.Fields.status %}{{ i.Fields.status.name | orgStatus }} {%endif%}{{ i.Fields.summary | trim | ljust:75 }}:JIRA:
   :PROPERTIES:
    :CUSTOM_ID:    {{ i.Key }}
    :CREATED:      {{ i.Fields.created | age }}
    :UPDATED:      {{ i.Fields.updated | age }}
    :PROJECT:      {{ i.Fields.project.name }}
    :PROJECTKEY:   {{ i.Fields.project.key }}
    :PROJECTCAT:   {{ i.Fields.project.projectCategory.name }}{%if i.Fields.project.projectCategory.description%} :: {{ i.Fields.project.projectCategory.description }}{%endif%}
    :PROJECTTYP:   {{ i.Fields.project.projectTypeKey }}
    :ASSIGNED:     {{i.Fields.assignee.displayName | ljust:25}} [{{i.Fields.assignee.emailAddress}}]
    :REPORTER:     {{i.Fields.reporter.displayName | ljust:25}} [{{i.Fields.reporter.emailAddress}}]
    :PRIORITY:     {{i.Fields.priority.name | ljust:25}}
    :ISSUETYPE:    {{i.Fields.issuetype.name | ljust:25}} {% if i.Fields.customfield_10020 %}
    :SPRINT:       {%for f in i.Fields.customfield_10020%}{{f.name}} {%endfor%}{%endif%}{% if i.Fields.customfield_10131 %}
    :DEVFACING:    T{%endif%}{%if i.Fields.labels%}
    :LABELS:       {%for lbl in i.Fields.labels%}{{lbl}} {%endfor%}{%endif%}{% if i.Fields.timeoriginalestimate %}
    :EFFORT:       {{i.Fields.timeoriginalestimate | orgEffort }} {%endif%}
    :STATUS:       {{i.Fields.status.name}} 
    :LINK:         [[{{endpoint}}/browse/{{i.Key}}][{{i.Key}}]]
   :END:
   {{i.Fields.description|orgCleanup|orgIndent:1}}
   {% if i.Fields.comment.comments %}
** Comments {% for cmt in i.Fields.comment.comments%}
*** {{cmt.author.displayName}}
    :PROPERTIES:
      :CREATED: {{cmt.created|age}}
    :END:
    {{cmt.body|orgWordWrap: 100|orgIndent: 1}}
   {%endfor%}
   {%endif%}
{% endfor %}