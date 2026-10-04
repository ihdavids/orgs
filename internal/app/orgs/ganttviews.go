//lint:file-ignore ST1006 allow the use of self
package orgs

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ihdavids/orgs/internal/common"
)

// GanttMarkers are the vertical lines a gantt view draws across the chart: one
// for every heading a second saved query finds, at that heading's date. They
// say "the release", "the offsite", "code freeze" - moments the plan has to
// be read against rather than tasks in it.
type GanttMarkers struct {
	// A marker set's name and whether it is drawn. The main markers have
	// neither; sets (GanttView.MarkerSets) are extra queries drawn their own
	// way, often generated one per tag or property value.
	Label  string `yaml:"label,omitempty" json:"label,omitempty"`
	Hidden bool   `yaml:"hidden,omitempty" json:"hidden,omitempty"`

	// The saved query whose headings become lines, or a query written for
	// the markers (Query wins when both are set).
	StoredQuery string `yaml:"storedQuery,omitempty" json:"storedQuery,omitempty"`
	Query       string `yaml:"query,omitempty" json:"query,omitempty"`
	// Which of a heading's dates the line is drawn at: scheduled (its
	// SCHEDULED or plain timestamp), deadline, or either (scheduled first).
	DateKind string `yaml:"dateKind,omitempty" json:"dateKind,omitempty"`
	// How the lines are drawn: a css colour, solid / dashed / dotted / dashdot,
	// and a width in pixels.
	Color string  `yaml:"color,omitempty" json:"color,omitempty"`
	Style string  `yaml:"style,omitempty" json:"style,omitempty"`
	Width float64 `yaml:"width,omitempty" json:"width,omitempty"`
	// The symbol drawn on a marker's line when no rule says otherwise:
	// none, diamond, circle, square, triangle or star.
	Shape string `yaml:"shape,omitempty" json:"shape,omitempty"`
	// Rules giving a marker a shape (and optionally a colour of its own) from
	// what its heading says. The first that matches wins.
	Rules []GanttMarkerRule `yaml:"rules,omitempty" json:"rules,omitempty"`
}

// GanttMarkerRule matches a marker's heading by a property's value (When
// "value", Key and Value), by a property being there at all ("has", Key), by a
// tag ("tag", Key is the tag), by a tag pattern ("tagmatch", Key a regular
// expression), by todo keyword ("status", Key one or more keywords, comma
// separated), or with no key by having no date of its own ("unscheduled") or
// being finished ("done"). Matching lives in worg (ruleMatches in gantt.ts);
// the server only keeps the rules.
type GanttMarkerRule struct {
	When  string `yaml:"when" json:"when"`
	Key   string `yaml:"key" json:"key"`
	Value string `yaml:"value,omitempty" json:"value,omitempty"`
	Shape string `yaml:"shape" json:"shape"`
	// Empty keeps the markers' own colour.
	Color string `yaml:"color,omitempty" json:"color,omitempty"`
}

// GanttView is a gantt chart as somebody set it up: the query it draws and
// every choice made about drawing it, kept under a name so it can be gone
// back to. Like a kanban board it holds nothing about the headings themselves
// - every bar is worked out afresh from the query each time it is opened.
type GanttView struct {
	Name        string `yaml:"name" json:"name"`
	StoredQuery string `yaml:"storedQuery" json:"storedQuery"`
	// The query as edited on the chart, when it differs from the saved one;
	// wins over StoredQuery. A view needs one or the other.
	Query string `yaml:"query,omitempty" json:"query,omitempty"`
	// A second query: only the rows it also finds are drawn.
	Filter string `yaml:"filter,omitempty" json:"filter,omitempty"`
	// Overdue and slipping work highlighted.
	Health bool `yaml:"health,omitempty" json:"health,omitempty"`
	// Snapshots of the plan's dates, and the one drawn under the bars.
	Baselines []GanttBaseline `yaml:"baselines,omitempty" json:"baselines,omitempty"`
	Baseline  string          `yaml:"baseline,omitempty" json:"baseline,omitempty"`
	Renderer    string `yaml:"renderer,omitempty" json:"renderer,omitempty"`
	Title       string `yaml:"title,omitempty" json:"title,omitempty"`
	ColorBy     string `yaml:"colorBy,omitempty" json:"colorBy,omitempty"`
	ColorProp   string `yaml:"colorProp,omitempty" json:"colorProp,omitempty"`
	// The property that splits the chart into swim lanes. Empty means the
	// usual lanes: SECTION, or the heading the task sits under.
	LaneProp     string  `yaml:"laneProp,omitempty" json:"laneProp,omitempty"`
	SkipWeekends bool    `yaml:"skipWeekends" json:"skipWeekends"`
	CriticalPath bool    `yaml:"criticalPath" json:"criticalPath"`
	DayWidth     float64 `yaml:"dayWidth,omitempty" json:"dayWidth,omitempty"`
	// What stretch of calendar it opens on. A quarter is kept relative to
	// today (0 is this quarter), so a view of "next quarter" stays one; a
	// From/To range is kept as dates.
	Quarter *int   `yaml:"quarter,omitempty" json:"quarter,omitempty"`
	From    string `yaml:"from,omitempty" json:"from,omitempty"`
	To      string `yaml:"to,omitempty" json:"to,omitempty"`
	// Chart colours the user picked, by palette key. Keys left out follow the
	// app's light or dark mode.
	Colors    map[string]string `yaml:"colors,omitempty" json:"colors,omitempty"`
	ShutLanes []string          `yaml:"shutLanes,omitempty" json:"shutLanes,omitempty"`
	Markers   *GanttMarkers     `yaml:"markers,omitempty" json:"markers,omitempty"`
	// More marker queries, each with its own look.
	MarkerSets []GanttMarkers `yaml:"markerSets,omitempty" json:"markerSets,omitempty"`
	// How the bars of the headings each rule matches are drawn: outline,
	// fill and shape, each decided by the first matching rule that sets it.
	BarRules []GanttBarRule `yaml:"barRules,omitempty" json:"barRules,omitempty"`
	// What BarRules were called while they could only outline. Kept so a
	// view saved then still reads; worg reads it when BarRules is empty and
	// writes BarRules from then on.
	Outlines []GanttBarRule `yaml:"outlines,omitempty" json:"outlines,omitempty"`
}

// GanttBarRule matches a bar's heading the way GanttMarkerRule matches a
// marker's (any of the same tests). Each part is optional: Color
// outlines it Width pixels wide (zero is two); Fill is solid, hatched,
// crosshatch, dotted, striped or hollow; Shape is bar, pill, wavy, diamond,
// circle or star.
// GanttBaseline is the plan's dates written down at a moment. Tasks are found
// again by hash, or by file and headline when the hash has moved.
type GanttBaseline struct {
	Name  string              `yaml:"name" json:"name"`
	Taken string              `yaml:"taken" json:"taken"`
	Tasks []GanttBaselineTask `yaml:"tasks" json:"tasks"`
}

type GanttBaselineTask struct {
	Hash     string `yaml:"hash" json:"hash"`
	Headline string `yaml:"headline" json:"headline"`
	Filename string `yaml:"filename" json:"filename"`
	Start    string `yaml:"start" json:"start"`
	End      string `yaml:"end" json:"end"`
}

type GanttBarRule struct {
	When  string  `yaml:"when" json:"when"`
	Key   string  `yaml:"key" json:"key"`
	Value string  `yaml:"value,omitempty" json:"value,omitempty"`
	Color string  `yaml:"color,omitempty" json:"color,omitempty"`
	Width float64 `yaml:"width,omitempty" json:"width,omitempty"`
	Fill  string  `yaml:"fill,omitempty" json:"fill,omitempty"`
	Shape string  `yaml:"shape,omitempty" json:"shape,omitempty"`
	// The preset that added this rule, so turning it off takes it back.
	Preset string `yaml:"preset,omitempty" json:"preset,omitempty"`
}

func (self *ExtensionsConfig) GetGanttViews(username string) []GanttView {
	self.mu.RLock()
	defer self.mu.RUnlock()
	if u, ok := self.Users[username]; ok && u.GanttViews != nil {
		return u.GanttViews
	}
	return []GanttView{}
}

func (self *ExtensionsConfig) SetGanttViews(username string, views []GanttView) error {
	self.mu.Lock()
	defer self.mu.Unlock()
	if views == nil {
		views = []GanttView{}
	}
	self.getUser(username).GanttViews = views
	return self.save()
}

/* SDOC: API
* GET /ext/gantt/views — List Saved Gantt Views
	Every gantt chart setup the user has saved: the query it draws, the
	renderer, colours, swim lane property, calendar window and marker lines.

	*Method:* =GET=

	*Response:* A JSON array of =GanttView= objects.
	EDOC */
func RequestGanttViews(w http.ResponseWriter, r *http.Request) {
	username := GetUsername(r)
	if username == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(GetExtensions().GetGanttViews(username))
}

/* SDOC: API
* POST /ext/gantt/views — Replace Every Saved Gantt View
	Writes the user's whole list of gantt views at once, which is what saving,
	renaming and deleting a view all go through.

	*Method:* =POST=

	*Request Body (JSON):* An array of =GanttView= objects.

	*Response:* A =ResultMsg=. Refused when a view has no name, two share one,
	or a view has neither a saved query nor a query of its own.
	EDOC */
func PostGanttViews(w http.ResponseWriter, r *http.Request) {
	username := GetUsername(r)
	if username == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	body, _ := io.ReadAll(r.Body)
	var views []GanttView
	if err := json.Unmarshal(body, &views); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	seen := map[string]bool{}
	for _, v := range views {
		msg := ""
		switch {
		case strings.TrimSpace(v.Name) == "":
			msg = "every view needs a name"
		case seen[v.Name]:
			msg = fmt.Sprintf("two views named %q", v.Name)
		case strings.TrimSpace(v.StoredQuery) == "" && strings.TrimSpace(v.Query) == "":
			msg = fmt.Sprintf("view %q has no query", v.Name)
		}
		if msg != "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: msg})
			return
		}
		seen[v.Name] = true
	}
	if err := GetExtensions().SetGanttViews(username, views); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	json.NewEncoder(w).Encode(common.ResultMsg{Ok: true, Msg: "saved"})
}
