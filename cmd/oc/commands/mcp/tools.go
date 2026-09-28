package mcp

// The tools, and the one place that says which of them write.
//
// Each tool is one request to the orgs server. Nothing here reimplements
// anything the server does: a tool that needed logic of its own would be a
// second answer to a question the server already answers, and the two would
// disagree within the month.
//
// Two rules about the descriptions. They are read by a model choosing between
// a dozen tools, so each says what the tool is *for* and, where two tools look
// alike, which one to reach for - org_search against org_find is the pair that
// matters. And the result of every tool carries the heading's `Hash`, because
// that is what every write here is addressed by; a tool that answered without
// one would leave the model nothing to act on.

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
)

type Tool struct {
	Name string
	// One line, for `orgs mcp -list`.
	Short string
	// What the model reads.
	Description string
	// JSON Schema for the arguments.
	Schema map[string]interface{}
	// Whether calling it changes an org file. -read-only drops these.
	Writes bool
	Run    func(core *commands.Core, args map[string]interface{}) (interface{}, error)
}

// Tools is the table. readOnly drops every tool that writes, which is the
// thing to reach for when pointing an agent at notes you would rather it did
// not edit - it is a smaller surface than trusting the agent not to.
func Tools(readOnly bool) []Tool {
	out := []Tool{}
	for _, t := range allTools {
		if readOnly && t.Writes {
			continue
		}
		out = append(out, t)
	}
	return out
}

// ---------------------------------------------------------------------------
// Reading arguments
//
// JSON numbers arrive as float64 and a model will sometimes send a number as a
// string, so these take either rather than failing on the spelling.
// ---------------------------------------------------------------------------

func str(args map[string]interface{}, key string) string {
	if v, ok := args[key]; ok && v != nil {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	}
	return ""
}

func need(args map[string]interface{}, key string) (string, error) {
	if v := str(args, key); v != "" {
		return v, nil
	}
	return "", fmt.Errorf("%s is required", key)
}

func num(args map[string]interface{}, key string, def int) int {
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return def
}

func yes(args map[string]interface{}, key string) bool {
	switch v := args[key].(type) {
	case bool:
		return v
	case string:
		return v == "true" || v == "t" || v == "1"
	}
	return false
}

// Schema helpers, so a tool's shape is one line rather than five nested maps.
func obj(props map[string]interface{}, required ...string) map[string]interface{} {
	if required == nil {
		required = []string{}
	}
	return map[string]interface{}{
		"type": "object", "properties": props, "required": required,
	}
}
func sprop(desc string) map[string]interface{} {
	return map[string]interface{}{"type": "string", "description": desc}
}
func nprop(desc string) map[string]interface{} {
	return map[string]interface{}{"type": "number", "description": desc}
}
func bprop(desc string) map[string]interface{} {
	return map[string]interface{}{"type": "boolean", "description": desc}
}

// Trimming a listing before it goes to a model is not tidiness, it is the
// difference between an answer and a context window. Every list tool takes a
// limit and says when it used one.
func capped[T any](rows []T, limit int) (interface{}, bool) {
	if limit > 0 && len(rows) > limit {
		return rows[:limit], true
	}
	return rows, false
}

func withNote(rows interface{}, trimmed bool, total int) interface{} {
	if !trimmed {
		return rows
	}
	return map[string]interface{}{
		"results": rows,
		"note":    fmt.Sprintf("trimmed to the first of %d; raise limit or narrow the query to see more", total),
	}
}

// ---------------------------------------------------------------------------

var allTools = []Tool{
	// -- reading ------------------------------------------------------------
	{
		Name:  "org_search",
		Short: "query headings by expression",
		Description: "Search the parsed org database for headings matching an expression. " +
			"This is the one that understands org: keywords, tags, properties, dates, outline. " +
			"Expression functions include IsTask(), IsTodo(), IsProject(), IsStatus(\"NEXT\"), " +
			"IsArchived(), IsPriority(\"A\"), HasTags(\"work\"), HasProperty(\"EFFORT\"), " +
			"MatchHeadline(re), MatchContent(re), MatchProperty(NAME, re), Today(), ThisWeek(), " +
			"OnDate(\"2006 02 01\"), IsRecord(), InCollection(\"contact\"), HasBacklinks(), " +
			"BacklinkCount(), LinksTo(re), HasBrokenLinks(). Combine with && || ! and parens. " +
			"Use org_find instead when looking for a phrase that could be anywhere in the text.",
		Schema: obj(map[string]interface{}{
			"query": sprop("the expression, e.g. IsTask() && IsStatus(\"NEXT\")"),
			"limit": nprop("most headings to return (default 100)"),
		}, "query"),
		Run: func(core *commands.Core, args map[string]interface{}) (interface{}, error) {
			q, err := need(args, "query")
			if err != nil {
				return nil, err
			}
			todos, err := commands.SendReceiveGetErr[common.Todos](core, "search", map[string]string{"query": q})
			if err != nil {
				return nil, err
			}
			rows, trimmed := capped([]common.Todo(todos), num(args, "limit", 100))
			return withNote(rows, trimmed, len(todos)), nil
		},
	},
	{
		Name:  "org_find",
		Short: "regular expression over the raw text",
		Description: "Search the raw text of every org file with a regular expression (Go syntax) and " +
			"return the lines that matched. Knows nothing about headings, which is exactly why it " +
			"finds text written in a property drawer, a table, a source block or the middle of a " +
			"paragraph. Use org_search instead when the question is about keywords, tags or dates.",
		Schema: obj(map[string]interface{}{
			"pattern":    sprop("the regular expression"),
			"ignoreCase": bprop("match without regard to case"),
			"file":       sprop("only search this file (basename or path)"),
			"limit":      nprop("most matching lines to return (default 100)"),
		}, "pattern"),
		Run: func(core *commands.Core, args map[string]interface{}) (interface{}, error) {
			p, err := need(args, "pattern")
			if err != nil {
				return nil, err
			}
			ps := map[string]string{"q": p, "max": "100"}
			if yes(args, "ignoreCase") {
				ps["ignoreCase"] = "t"
			}
			if f := str(args, "file"); f != "" {
				ps["file"] = f
			}
			res, err := commands.SendReceiveGetErr[common.FileSearchResult](core, "files/search", ps)
			if err != nil {
				return nil, err
			}
			if !res.Ok {
				return nil, fmt.Errorf("%s is not a valid pattern: %s", p, res.Msg)
			}
			// Flattened: the server groups by file because a panel draws it
			// that way, and a model reading two levels to reach a line is two
			// levels of nothing.
			type hit struct {
				Filename string
				Line     int
				Text     string
			}
			hits := []hit{}
			for _, f := range res.Files {
				for _, m := range f.Matches {
					hits = append(hits, hit{f.Filename, m.Line, strings.TrimRight(m.Text, "\r\n")})
				}
			}
			rows, trimmed := capped(hits, num(args, "limit", 100))
			return withNote(rows, trimmed, res.Total), nil
		},
	},
	{
		Name:        "org_heading",
		Short:       "one heading's body",
		Description: "The text written under one heading, as it is in the file, addressed by the Hash that org_search returns. Child headings are not part of it.",
		Schema: obj(map[string]interface{}{
			"hash": sprop("the heading's Hash, from org_search"),
		}, "hash"),
		Run: func(core *commands.Core, args map[string]interface{}) (interface{}, error) {
			h, err := need(args, "hash")
			if err != nil {
				return nil, err
			}
			res, err := commands.SendReceiveGetErr[struct {
				Ok     bool
				Msg    string
				Text   string
				Audio  string
				Image  string
				Images []string
			}](core, "body/"+commands.HashPath(h), nil)
			if err != nil {
				return nil, err
			}
			if !res.Ok && res.Msg != "" {
				return nil, fmt.Errorf("%s", res.Msg)
			}
			return res, nil
		},
	},
	{
		Name:        "org_files",
		Short:       "the files the server watches",
		Description: "Every org file this server has loaded, as absolute paths.",
		Schema:      obj(map[string]interface{}{}),
		Run: func(core *commands.Core, args map[string]interface{}) (interface{}, error) {
			return commands.SendReceiveGetErr[common.FileList](core, "files", nil)
		},
	},
	{
		Name:        "org_file_headings",
		Short:       "the outline of one file",
		Description: "Every heading in one org file, in document order, with its keyword, tags, line and Hash. The way to read a file's shape before changing anything in it.",
		Schema: obj(map[string]interface{}{
			"filename": sprop("the file (basename or path)"),
		}, "filename"),
		Run: func(core *commands.Core, args map[string]interface{}) (interface{}, error) {
			f, err := need(args, "filename")
			if err != nil {
				return nil, err
			}
			return commands.SendReceiveGetErr[common.Todos](core, "filecontents/headings",
				map[string]string{"filename": f})
		},
	},
	{
		Name:        "org_status_keywords",
		Short:       "the todo keywords",
		Description: "The todo keywords this server accepts, split the way org splits them: active before the pipe, finished after it, in the order they were configured. Ask before setting a keyword rather than guessing one.",
		Schema: obj(map[string]interface{}{
			"hash": sprop("optional: ask for one heading, whose own file may declare its own #+TODO:"),
		}),
		Run: func(core *commands.Core, args map[string]interface{}) (interface{}, error) {
			path := "status"
			if h := str(args, "hash"); h != "" {
				path = "status/" + commands.HashPath(h)
			}
			return commands.SendReceiveGetErr[interface{}](core, path, nil)
		},
	},
	{
		Name:  "org_links",
		Short: "every link, and where it goes",
		Description: "Every link written anywhere in the org files - external ones included, which is mostly the point: a ticket or a document pasted into a heading has no other index. " +
			"Each says what it points at, what service that is, and which heading it was written in.",
		Schema: obj(map[string]interface{}{
			"service": sprop("only links to this service, e.g. GitHub, Jira, Google Docs"),
			"file":    sprop("only links written in this file"),
			"limit":   nprop("most links to return (default 100)"),
		}),
		Run: func(core *commands.Core, args map[string]interface{}) (interface{}, error) {
			ps := map[string]string{}
			if v := str(args, "service"); v != "" {
				ps["service"] = v
			}
			if v := str(args, "file"); v != "" {
				ps["file"] = v
			}
			list, err := commands.SendReceiveGetErr[common.LinkList](core, "links/all", ps)
			if err != nil {
				return nil, err
			}
			rows, trimmed := capped(list.Links, num(args, "limit", 100))
			return withNote(rows, trimmed, list.Total), nil
		},
	},
	{
		Name:        "org_backlinks",
		Short:       "what points at a file",
		Description: "The links pointing into one org file, the links it writes out, and the ones it writes back into itself. What to ask before moving or deleting anything.",
		Schema: obj(map[string]interface{}{
			"filename": sprop("the file (basename or path)"),
		}, "filename"),
		Run: func(core *commands.Core, args map[string]interface{}) (interface{}, error) {
			f, err := need(args, "filename")
			if err != nil {
				return nil, err
			}
			res, err := commands.SendReceiveGetErr[common.Backlinks](core, "links", map[string]string{"filename": f})
			if err != nil {
				return nil, err
			}
			if !res.Ok {
				return nil, fmt.Errorf("%s", res.Msg)
			}
			return res, nil
		},
	},
	{
		Name:  "org_code_list",
		Short: "the source blocks",
		Description: "Every #+BEGIN_SRC block, read: language, name, switches, header arguments and variables. " +
			"A variable that names a table or another block in the file comes back resolved - what it points at, what kind of thing that is, and where it lives - so a block that cannot run is visible as one.",
		Schema: obj(map[string]interface{}{
			"lang":  sprop("only blocks in this language"),
			"q":     sprop("only blocks matching this text"),
			"limit": nprop("most blocks to return (default 50)"),
		}),
		Run: func(core *commands.Core, args map[string]interface{}) (interface{}, error) {
			ps := map[string]string{}
			if v := str(args, "lang"); v != "" {
				ps["lang"] = strings.ToLower(v)
			}
			if v := str(args, "q"); v != "" {
				ps["q"] = v
			}
			idx, err := commands.SendReceiveGetErr[common.CodeIndex](core, "code", ps)
			if err != nil {
				return nil, err
			}
			rows, trimmed := capped(idx.Blocks, num(args, "limit", 50))
			return withNote(rows, trimmed, idx.Total), nil
		},
	},
	{
		Name:        "org_records",
		Short:       "contacts, equipment, anything kept as records",
		Description: "Records - headings that stand for one thing each, with their fields in a property drawer. A collection is named by the RECORD property: \"contact\", \"equipment\", whatever is being kept. Omit type to search every collection.",
		Schema: obj(map[string]interface{}{
			"type":  sprop("the collection, e.g. contact; omit for all of them"),
			"q":     sprop("text to search for"),
			"limit": nprop("most records to return (default 50)"),
		}),
		Run: func(core *commands.Core, args map[string]interface{}) (interface{}, error) {
			ps := map[string]string{"type": str(args, "type"), "body": "1"}
			if v := str(args, "q"); v != "" {
				ps["q"] = v
			}
			recs, err := commands.SendReceiveGetErr[[]common.Record](core, "records", ps)
			if err != nil {
				return nil, err
			}
			rows, trimmed := capped(recs, num(args, "limit", 50))
			return withNote(rows, trimmed, len(recs)), nil
		},
	},
	{
		Name:        "org_capture_templates",
		Short:       "what org_capture can file under",
		Description: "The capture templates this server is configured with, and where each one files what it captures. Read this before org_capture: the template decides which file and which heading a new note lands under.",
		Schema:      obj(map[string]interface{}{}),
		Run: func(core *commands.Core, args map[string]interface{}) (interface{}, error) {
			return commands.SendReceiveGetErr[[]common.CaptureTemplate](core, "capture/templates", nil)
		},
	},

	// -- writing ------------------------------------------------------------
	{
		Name:   "org_capture",
		Short:  "file a new heading",
		Writes: true,
		Description: "File a new heading through a capture template. The template decides where it lands; " +
			"call org_capture_templates first to see which templates exist and what each one is for.",
		Schema: obj(map[string]interface{}{
			"template": sprop("the template's name, from org_capture_templates"),
			"headline": sprop("the heading text"),
			"content":  sprop("the body under it, optional"),
			"tags":     map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "tags, optional"},
			"priority": sprop("A, B or C, optional"),
		}, "template", "headline"),
		Run: func(core *commands.Core, args map[string]interface{}) (interface{}, error) {
			t, err := need(args, "template")
			if err != nil {
				return nil, err
			}
			h, err := need(args, "headline")
			if err != nil {
				return nil, err
			}
			tags := []string{}
			if raw, ok := args["tags"].([]interface{}); ok {
				for _, v := range raw {
					if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
						tags = append(tags, strings.TrimSpace(s))
					}
				}
			}
			req := common.Capture{Template: t, NewNode: common.NewNode{
				Headline: h, Content: str(args, "content"),
				Tags: tags, Priority: str(args, "priority"),
			}}
			return post[common.Capture, common.ResultMsg](core, "capture", &req)
		},
	},
	{
		Name:        "org_set_status",
		Short:       "change a heading's keyword",
		Writes:      true,
		Description: "Change one heading's todo keyword - TODO to DONE, and so on. Ask org_status_keywords which keywords that heading's own file allows rather than guessing; a file may declare its own with #+TODO:.",
		Schema: obj(map[string]interface{}{
			"hash":   sprop("the heading's Hash, from org_search"),
			"status": sprop("the new keyword, e.g. DONE"),
		}, "hash", "status"),
		Run: func(core *commands.Core, args map[string]interface{}) (interface{}, error) {
			h, err := need(args, "hash")
			if err != nil {
				return nil, err
			}
			s, err := need(args, "status")
			if err != nil {
				return nil, err
			}
			req := common.TodoItemChange{Hash: h, Value: s}
			return post[common.TodoItemChange, common.Result](core, "status/change", &req)
		},
	},
	{
		Name:        "org_set_property",
		Short:       "write a property",
		Writes:      true,
		Description: "Write one property on one heading, creating the :PROPERTIES: drawer when it has none. An empty value removes the property, and the drawer with it when it was the last one.",
		Schema: obj(map[string]interface{}{
			"hash":  sprop("the heading's Hash, from org_search"),
			"name":  sprop("the property name, e.g. EFFORT"),
			"value": sprop("the value; empty removes the property"),
		}, "hash", "name"),
		Run: func(core *commands.Core, args map[string]interface{}) (interface{}, error) {
			h, err := need(args, "hash")
			if err != nil {
				return nil, err
			}
			n, err := need(args, "name")
			if err != nil {
				return nil, err
			}
			req := common.TodoPropertyChange{Hash: h, Name: n, Value: str(args, "value")}
			return post[common.TodoPropertyChange, common.Result](core, "property", &req)
		},
	},
	{
		Name:        "org_rename_heading",
		Short:       "change a heading's text",
		Writes:      true,
		Description: "Change the text of one heading. The keyword, priority and tags are left where they are - only the title changes.",
		Schema: obj(map[string]interface{}{
			"hash":     sprop("the heading's Hash, from org_search"),
			"headline": sprop("the new text"),
		}, "hash", "headline"),
		Run: func(core *commands.Core, args map[string]interface{}) (interface{}, error) {
			h, err := need(args, "hash")
			if err != nil {
				return nil, err
			}
			v, err := need(args, "headline")
			if err != nil {
				return nil, err
			}
			req := common.TodoItemChange{Hash: h, Value: v}
			return post[common.TodoItemChange, common.Result](core, "headline/change", &req)
		},
	},
	{
		Name:   "org_set_date",
		Short:  "schedule, deadline or timestamp",
		Writes: true,
		Description: "Write a SCHEDULED, DEADLINE, CLOSED or plain timestamp on one heading. " +
			"The value is an org date as it is written in a file: \"<2026-01-01 Mon>\", or empty to clear it.",
		Schema: obj(map[string]interface{}{
			"hash":  sprop("the heading's Hash, from org_search"),
			"name":  sprop("SCHEDULED, DEADLINE, CLOSED or TIMESTAMP"),
			"value": sprop("an org date like <2026-01-01 Mon>; empty clears it"),
		}, "hash", "name"),
		Run: func(core *commands.Core, args map[string]interface{}) (interface{}, error) {
			h, err := need(args, "hash")
			if err != nil {
				return nil, err
			}
			n, err := need(args, "name")
			if err != nil {
				return nil, err
			}
			req := common.TodoDateChange{Hash: h, Name: strings.ToUpper(n), Value: str(args, "value")}
			return post[common.TodoDateChange, common.Result](core, "date/change", &req)
		},
	},
	{
		Name:   "org_code_run",
		Short:  "run a source block",
		Writes: true,
		Description: "Run one source block and return what it produced. Identify it by the filename and the Id " +
			"that org_code_list gives. This executes a program on the server and is off unless babel.enable is " +
			"set there; the refusal says so.",
		Schema: obj(map[string]interface{}{
			"filename": sprop("the file the block is in, from org_code_list"),
			"id":       nprop("the block's Id within that file, from org_code_list"),
		}, "filename", "id"),
		Run: func(core *commands.Core, args map[string]interface{}) (interface{}, error) {
			f, err := need(args, "filename")
			if err != nil {
				return nil, err
			}
			if _, ok := args["id"]; !ok {
				return nil, fmt.Errorf("id is required - org_code_list gives it")
			}
			req := common.CodeRun{Filename: f, Id: num(args, "id", 0)}
			return post[common.CodeRun, common.CodeResult](core, "code/run", &req)
		},
	},
	{
		Name:        "org_record_add",
		Short:       "add a record",
		Writes:      true,
		Description: "Add one record to a collection - a contact, a piece of equipment, anything kept as a list. Field names carry their own meaning: EMAIL_WORK is an email labelled work, PHONE_MOBILE a phone labelled mobile, BOUGHT_DATE a date called bought.",
		Schema: obj(map[string]interface{}{
			"type":   sprop("the collection, e.g. contact"),
			"name":   sprop("the record's name, which becomes the heading"),
			"fields": map[string]interface{}{"type": "object", "description": "property name to value, e.g. {\"EMAIL\": \"a@b\"}", "additionalProperties": map[string]interface{}{"type": "string"}},
			"notes":  sprop("a line about it, optional"),
			"file":   sprop("org file to write to when the collection has no home yet"),
		}, "type", "name"),
		Run: func(core *commands.Core, args map[string]interface{}) (interface{}, error) {
			t, err := need(args, "type")
			if err != nil {
				return nil, err
			}
			n, err := need(args, "name")
			if err != nil {
				return nil, err
			}
			fields := map[string]string{}
			if raw, ok := args["fields"].(map[string]interface{}); ok {
				for k, v := range raw {
					fields[strings.ToUpper(k)] = fmt.Sprintf("%v", v)
				}
			}
			req := common.RecordNew{Type: t, Name: n, Fields: fields,
				Notes: str(args, "notes"), Filename: str(args, "file")}
			return post[common.RecordNew, common.Record](core, "record", &req)
		},
	},
	{
		Name:        "org_record_update",
		Short:       "change a record's fields",
		Writes:      true,
		Description: "Write fields on one record, addressed by its Hash from org_records. A field set to an empty string is removed. RECORD and ADDED are refused - one is the record's identity and the other is when it started.",
		Schema: obj(map[string]interface{}{
			"hash": sprop("the record's Hash, from org_records"),
			"set":  map[string]interface{}{"type": "object", "description": "property name to value; empty value removes it", "additionalProperties": map[string]interface{}{"type": "string"}},
		}, "hash", "set"),
		Run: func(core *commands.Core, args map[string]interface{}) (interface{}, error) {
			h, err := need(args, "hash")
			if err != nil {
				return nil, err
			}
			set := map[string]string{}
			if raw, ok := args["set"].(map[string]interface{}); ok {
				for k, v := range raw {
					if v == nil {
						set[strings.ToUpper(k)] = ""
						continue
					}
					set[strings.ToUpper(k)] = fmt.Sprintf("%v", v)
				}
			}
			if len(set) == 0 {
				return nil, fmt.Errorf("set is required and must name at least one field")
			}
			req := common.RecordUpdate{Hash: h, Set: set}
			return post[common.RecordUpdate, common.ResultMsg](core, "record/update", &req)
		},
	},
}

// Every write goes through here, which is where -dry-run turns into something
// the model can read. SendReceivePostErr refuses and answers with ErrDryRun;
// left as an error that would read as a failure, and an agent retries
// failures. Saying what would have happened is the honest answer.
func post[REQ any, RESP any](core *commands.Core, path string, req *REQ) (interface{}, error) {
	res, err := commands.SendReceivePostErr[REQ, RESP](core, path, req)
	if err == commands.ErrDryRun {
		return map[string]interface{}{
			"Ok":  false,
			"Msg": "this server is being run with -dry-run: nothing was written, and nothing will be",
		}, nil
	}
	if err != nil {
		return nil, err
	}
	return res, nil
}
