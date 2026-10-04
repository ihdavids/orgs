package kanban

// The org side of a board: which column a heading lands in, what colour it
// gets, how the cards in a column are ordered and what number a move writes.
//
// A port of worg/src/kanban.ts, and the two have to agree: the boards are the
// same boards (the per-user /ext/kanban/boards store), so a card in a different
// column here than in the browser, or a manual order that one of them reads
// differently, is a board that is wrong in one place. The scenarios are pinned
// in model_test.go and kanban.test.ts alike.

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// Column and Board are the server's KanbanColumn / KanbanBoard
// (internal/app/orgs/extensions.go), restated because this package cannot
// import that one. Every field is kept so that saving a board back writes
// what was read.
type Column struct {
	Value     string   `yaml:"value" json:"value"`
	Title     string   `yaml:"title" json:"title"`
	Color     string   `yaml:"color" json:"color"`
	Limit     int      `yaml:"limit" json:"limit"`
	Collapsed bool     `yaml:"collapsed" json:"collapsed"`
	Aliases   []string `yaml:"aliases" json:"aliases"`
}

type Board struct {
	Name            string            `yaml:"name" json:"name"`
	StoredQuery     string            `yaml:"storedQuery" json:"storedQuery"`
	Query           string            `yaml:"query" json:"query"`
	IncludeArchived bool              `yaml:"includeArchived" json:"includeArchived"`
	GroupBy         string            `yaml:"groupBy" json:"groupBy"`
	GroupKey        string            `yaml:"groupKey" json:"groupKey"`
	Columns         []Column          `yaml:"columns" json:"columns"`
	ShowUnset       bool              `yaml:"showUnset" json:"showUnset"`
	UnsetTitle      string            `yaml:"unsetTitle" json:"unsetTitle"`
	ColorBy         string            `yaml:"colorBy" json:"colorBy"`
	ColorKey        string            `yaml:"colorKey" json:"colorKey"`
	Colors          map[string]string `yaml:"colors" json:"colors"`
	Badges          []string          `yaml:"badges" json:"badges"`
	BackShows       string            `yaml:"backShows" json:"backShows"`
	Sort            string            `yaml:"sort" json:"sort"`
	OrderKey        string            `yaml:"orderKey" json:"orderKey"`
	Layout          string            `yaml:"layout" json:"layout"`
	ListFields      []string          `yaml:"listFields" json:"listFields"`
	LabelSource     string            `yaml:"labelSource" json:"labelSource"`
	LabelKey        string            `yaml:"labelKey" json:"labelKey"`
	LabelColors     map[string]string `yaml:"labelColors" json:"labelColors"`
	LabelOrder      []string          `yaml:"labelOrder" json:"labelOrder"`
	Hidden          []string          `yaml:"hidden" json:"hidden"`
	Folded          []string          `yaml:"folded" json:"folded"`
	HeaderKey       string            `yaml:"headerKey" json:"headerKey"`
	HeaderColors    map[string]string `yaml:"headerColors" json:"headerColors"`
}

type Card = common.Todo

type StoredQuery struct {
	Name  string `yaml:"name" json:"name"`
	Query string `yaml:"query" json:"query"`
}

// --- colour ---------------------------------------------------------------------

type Swatch struct{ Key, Label, Hex string }

var Palette = []Swatch{
	{"slate", "Slate", "#6b7280"},
	{"red", "Red", "#e5484d"},
	{"orange", "Orange", "#f76808"},
	{"amber", "Amber", "#f5a524"},
	{"lime", "Lime", "#8db421"},
	{"green", "Green", "#30a46c"},
	{"teal", "Teal", "#12a594"},
	{"cyan", "Cyan", "#0d9bbd"},
	{"blue", "Blue", "#3e63dd"},
	{"indigo", "Indigo", "#5b5bd6"},
	{"violet", "Violet", "#8e4ec6"},
	{"pink", "Pink", "#d6409f"},
}

func SwatchOf(key string) Swatch {
	for _, s := range Palette {
		if s.Key == key {
			return s
		}
	}
	return Palette[0]
}

// AutoSwatch is a stable colour for a value nobody chose one for: the same
// spelling always lands on the same hue, here and in the browser. The hash is
// worg's - UTF-16 code units, wrapped to 32 bits - so it has to be computed the
// way JavaScript computes it. Slate means "no value", so it is never handed out.
func AutoSwatch(value string) Swatch {
	v := strings.ToLower(strings.TrimSpace(value))
	if v == "" {
		return Palette[0]
	}
	var h uint32
	for _, u := range utf16.Encode([]rune(v)) {
		h = h*31 + uint32(u)
	}
	return Palette[1+int(h%uint32(len(Palette)-1))]
}

var PriorityColor = map[string]string{"A": "#e5484d", "B": "#f5a524", "C": "#3e63dd"}

var DefaultHeaderKeys = []string{"COLOUR", "COLOR"}

const (
	HeaderNoneDark  = "#4b5563"
	HeaderNoneLight = "#9ca3af"
)

var colorWords = map[string]string{
	"grey": "slate", "gray": "slate", "silver": "slate",
	"purple": "violet", "magenta": "pink", "fuchsia": "pink", "rose": "pink",
	"yellow": "amber", "gold": "amber", "mustard": "amber",
	"turquoise": "teal", "aqua": "cyan", "sky": "cyan",
	"navy": "indigo", "royal": "indigo",
	"maroon": "red", "crimson": "red", "scarlet": "red",
	"olive": "lime", "emerald": "green", "forest": "green",
}

func prop(c *Card, name string) string {
	if name == "" || c.Props == nil {
		return ""
	}
	if v, ok := c.Props[name]; ok {
		return v
	}
	for k, v := range c.Props {
		if strings.EqualFold(k, name) {
			return v
		}
	}
	return ""
}

// HeaderValueOf is what a card says about the thin bar across its top.
func HeaderValueOf(c *Card, b *Board) string {
	if k := strings.TrimSpace(b.HeaderKey); k != "" {
		return strings.TrimSpace(prop(c, k))
	}
	for _, k := range DefaultHeaderKeys {
		if v := strings.TrimSpace(prop(c, k)); v != "" {
			return v
		}
	}
	return ""
}

func HeaderColorOf(c *Card, b *Board, dark bool) string {
	v := HeaderValueOf(c, b)
	if v == "" {
		if dark {
			return HeaderNoneDark
		}
		return HeaderNoneLight
	}
	if m := b.HeaderColors[v]; m != "" {
		if strings.HasPrefix(m, "#") {
			return m
		}
		return SwatchOf(m).Hex
	}
	return ColorFromWord(v)
}

// ColorFromWord reads a value as a colour: a palette name, a word people use
// for one, or a hex code. Anything else gets a stable colour of its own.
func ColorFromWord(value string) string {
	v := strings.ToLower(strings.TrimSpace(value))
	if len(v) == 7 && v[0] == '#' && isHex(v[1:]) {
		return v
	}
	if len(v) == 4 && v[0] == '#' && isHex(v[1:]) {
		return "#" + string([]byte{v[1], v[1], v[2], v[2], v[3], v[3]})
	}
	named := v
	if w, ok := colorWords[v]; ok {
		named = w
	}
	for _, p := range Palette {
		if p.Key == named || strings.ToLower(p.Label) == named {
			return p.Hex
		}
	}
	return AutoSwatch(value).Hex
}

func isHex(s string) bool {
	for _, r := range s {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return false
		}
	}
	return true
}

// SplitLabels reads a property holding several labels: commas or spaces.
func SplitLabels(v string) []string {
	return strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' })
}

// LabelsOf is the labels a card carries, in the board's order, with anything
// the board never named after the ones it did.
func LabelsOf(c *Card, b *Board) []string {
	var have []string
	switch b.LabelSource {
	case "tags":
		have = c.Tags
	case "property":
		have = SplitLabels(prop(c, strings.ToUpper(b.LabelKey)))
	default:
		return nil
	}
	set := map[string]bool{}
	for _, v := range have {
		set[v] = true
	}
	out := []string{}
	for _, v := range b.LabelOrder {
		if set[v] {
			out = append(out, v)
		}
	}
	for _, v := range have {
		if !contains(b.LabelOrder, v) && !contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

func LabelColorOf(value string, b *Board) string {
	if n := b.LabelColors[value]; n != "" {
		if strings.HasPrefix(n, "#") {
			return n
		}
		return SwatchOf(n).Hex
	}
	return AutoSwatch(value).Hex
}

// LabelsInCards is every label worth offering: the board's own set, then
// anything the cards wear.
func LabelsInCards(cards []Card, b *Board) []string {
	out := append([]string{}, b.LabelOrder...)
	if b.LabelSource != "" && b.LabelSource != "none" {
		for i := range cards {
			for _, v := range LabelsOf(&cards[i], b) {
				if !contains(out, v) {
					out = append(out, v)
				}
			}
		}
	}
	return out
}

func ColorValueOf(c *Card, b *Board) string {
	switch b.ColorBy {
	case "property":
		return prop(c, b.ColorKey)
	case "status":
		return c.Status
	case "priority":
		return c.Priority
	case "tag":
		for _, t := range c.Tags {
			if b.Colors[t] != "" {
				return t
			}
		}
		if len(c.Tags) > 0 {
			return c.Tags[0]
		}
	}
	return ""
}

// CardSwatch is the colour-by stripe, or nil when the board colours nothing.
func CardSwatch(c *Card, b *Board) *Swatch {
	if b.ColorBy == "" || b.ColorBy == "none" {
		return nil
	}
	v := ColorValueOf(c, b)
	if v == "" {
		return nil
	}
	if n := b.Colors[v]; n != "" {
		s := SwatchOf(n)
		return &s
	}
	s := AutoSwatch(v)
	return &s
}

// --- grouping ---------------------------------------------------------------------

const Unset = ""

func GroupValuesOf(c *Card, b *Board) []string {
	switch b.GroupBy {
	case "property":
		if v := prop(c, b.GroupKey); v != "" {
			return []string{v}
		}
		return nil
	case "tag":
		return c.Tags
	}
	if c.Status != "" {
		return []string{c.Status}
	}
	return nil
}

func sameValue(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

// ColumnValues is every spelling a column answers to, its own first.
func ColumnValues(col Column) []string {
	out := []string{col.Value}
	for _, a := range col.Aliases {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		dup := false
		for _, x := range out {
			if sameValue(x, a) {
				dup = true
			}
		}
		if !dup {
			out = append(out, a)
		}
	}
	return out
}

// ColumnOf is the column a card belongs in among the columns on show, asked in
// board order, or "" when none claims it.
func ColumnOf(c *Card, b *Board, cols []Column) string {
	values := GroupValuesOf(c, b)
	for _, col := range cols {
		if col.Value == "" {
			continue
		}
		for _, claim := range ColumnValues(col) {
			for _, v := range values {
				if sameValue(v, claim) {
					return col.Value
				}
			}
		}
	}
	return Unset
}

// CanonicalValue says IN-PROGRESS, INPROGRESS, in_progress and "In Progress"
// are one state.
func CanonicalValue(v string) string {
	var sb strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(v)) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func DefaultUnsetTitle(b *Board) string {
	switch b.GroupBy {
	case "property":
		if b.GroupKey != "" {
			return "No " + b.GroupKey
		}
		return "No value"
	case "tag":
		return "Untagged"
	}
	return "No status"
}

func UnsetColumn(b *Board) Column {
	t := b.UnsetTitle
	if t == "" {
		t = DefaultUnsetTitle(b)
	}
	return Column{Value: Unset, Title: t, Color: "slate"}
}

func ColumnTitle(col Column) string {
	if col.Title != "" {
		return col.Title
	}
	return col.Value
}

var (
	doneHues   = []string{"green", "teal", "lime"}
	activeHues = []string{"blue", "indigo", "violet", "pink", "amber", "orange", "red", "cyan"}
)

// DefaultStatusColumns is the columns a status board shows when it has none
// of its own: the states in play, then strays the query uses, then the
// finished states - spellings of one word folded into one column.
func DefaultStatusColumns(active, done, discovered []string) []Column {
	seen := map[string][]string{}
	take := func(list []string) []string {
		prim := []string{}
		for _, raw := range list {
			v := strings.TrimSpace(raw)
			if v == "" {
				continue
			}
			k := CanonicalValue(v)
			if had, ok := seen[k]; ok {
				dup := false
				for _, x := range had {
					if sameValue(x, v) {
						dup = true
					}
				}
				if !dup {
					seen[k] = append(had, v)
				}
				continue
			}
			seen[k] = []string{v}
			prim = append(prim, v)
		}
		return prim
	}
	a := take(active)
	d := take(done)
	extra := take(discovered)
	col := func(v, color string) Column {
		al := []string{}
		for _, x := range seen[CanonicalValue(v)] {
			if !sameValue(x, v) {
				al = append(al, x)
			}
		}
		return Column{Value: v, Color: color, Aliases: al}
	}
	out := []Column{}
	for i, v := range a {
		out = append(out, col(v, activeHues[i%len(activeHues)]))
	}
	for i, v := range extra {
		out = append(out, col(v, activeHues[(len(a)+i)%len(activeHues)]))
	}
	for i, v := range d {
		out = append(out, col(v, doneHues[i%len(doneHues)]))
	}
	return out
}

func StatusesInCards(cards []Card) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, c := range cards {
		v := strings.TrimSpace(c.Status)
		if v == "" || seen[strings.ToLower(v)] {
			continue
		}
		seen[strings.ToLower(v)] = true
		out = append(out, v)
	}
	return out
}

// DiscoverValues is the column values the query turned up that no column
// claims yet, commonest first.
func DiscoverValues(cards []Card, b *Board) []string {
	counts := map[string]int{}
	order := []string{}
	for i := range cards {
		for _, v := range GroupValuesOf(&cards[i], b) {
			if strings.TrimSpace(v) == "" {
				continue
			}
			if counts[v] == 0 {
				order = append(order, v)
			}
			counts[v]++
		}
	}
	have := map[string]bool{}
	for _, c := range b.Columns {
		for _, v := range ColumnValues(c) {
			have[strings.ToLower(strings.TrimSpace(v))] = true
		}
	}
	out := []string{}
	for _, v := range order {
		if !have[strings.ToLower(strings.TrimSpace(v))] {
			out = append(out, v)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if counts[out[i]] != counts[out[j]] {
			return counts[out[i]] > counts[out[j]]
		}
		return out[i] < out[j]
	})
	return out
}

// --- ordering ----------------------------------------------------------------------

var Sorts = []string{"manual", "priority", "deadline", "scheduled", "headline", "file"}

func priorityRank(p string) int {
	c := strings.ToUpper(strings.TrimSpace(p))
	if c == "" {
		return 99
	}
	i := strings.Index("ABCDEFG", c)
	if i < 0 {
		return 98
	}
	return i
}

func dateRank(d *org.OrgDate) int64 {
	if !IsSet(d) {
		return math.MaxInt64
	}
	return d.Start.UnixNano()
}

// IsSet is whether a date was written: go-org writes year one for none.
func IsSet(d *org.OrgDate) bool {
	return d != nil && !d.Start.IsZero() && d.Start.Year() > 1
}

func OrderKey(b *Board) string {
	if b.OrderKey != "" {
		return b.OrderKey
	}
	return "KANBAN"
}

// OrderOf is a card's manual order, or false when it has none.
func OrderOf(c *Card, b *Board) (float64, bool) {
	raw := strings.TrimSpace(prop(c, OrderKey(b)))
	if raw == "" {
		return 0, false
	}
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// SortCards orders one column as the board asks. Stable, so a refresh does not
// reshuffle what the sort cannot tell apart.
func SortCards(cards []Card, b *Board) []Card {
	out := append([]Card{}, cards...)
	sort.SliceStable(out, func(i, j int) bool {
		a, c := &out[i], &out[j]
		switch b.Sort {
		case "headline":
			return a.Headline < c.Headline
		case "priority":
			ra, rc := priorityRank(a.Priority), priorityRank(c.Priority)
			if ra != rc {
				return ra < rc
			}
			return a.Headline < c.Headline
		case "deadline":
			return dateRank(a.Deadline) < dateRank(c.Deadline)
		case "scheduled":
			return dateRank(a.Date) < dateRank(c.Date)
		case "file":
			if a.Filename != c.Filename {
				return a.Filename < c.Filename
			}
			return a.LineNum < c.LineNum
		}
		// Manual: a card nobody has placed waits at the end.
		ao, aok := OrderOf(a, b)
		co, cok := OrderOf(c, b)
		switch {
		case !aok && !cok:
			return false
		case !aok:
			return false
		case !cok:
			return true
		}
		return ao < co
	})
	return out
}

// OrderBetween is the value for a card put between two others; false means the
// gap has run out of room and the column has to be renumbered.
func OrderBetween(prev, next *float64) (float64, bool) {
	switch {
	case prev == nil && next == nil:
		return 0, true
	case prev == nil:
		return *next - 1, true
	case next == nil:
		return *prev + 1, true
	}
	mid := (*prev + *next) / 2
	if mid <= *prev || mid >= *next {
		return 0, false
	}
	return mid, true
}

// FormatOrder keeps a written order short enough to read in a drawer.
func FormatOrder(n float64) string {
	if n == math.Trunc(n) {
		return strconv.FormatInt(int64(n), 10)
	}
	s := strconv.FormatFloat(n, 'f', 6, 64)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	return s
}

func Renumber(count int) []float64 {
	out := make([]float64, count)
	for i := range out {
		out[i] = float64(i * 100)
	}
	return out
}

// OrderWrite is one property write a move needs.
type OrderWrite struct {
	Hash  string
	Value float64
}

// PlanOrder is what a card moved to position `at` in `list` (the column without
// it) has to write: its own midpoint normally, the whole column when the gap
// has run out or a card above has no number to be below.
func PlanOrder(list []Card, at int, moving *Card, b *Board) []OrderWrite {
	if at < 0 {
		at = 0
	}
	if at > len(list) {
		at = len(list)
	}
	var prev, next *Card
	if at > 0 {
		prev = &list[at-1]
	}
	if at < len(list) {
		next = &list[at]
	}
	var po, no *float64
	prevMissing := false
	if prev != nil {
		if v, ok := OrderOf(prev, b); ok {
			po = &v
		} else {
			prevMissing = true
		}
	}
	if next != nil {
		if v, ok := OrderOf(next, b); ok {
			no = &v
		}
	}
	if !prevMissing {
		if v, ok := OrderBetween(po, no); ok {
			return []OrderWrite{{moving.Hash, v}}
		}
	}
	after := append([]Card{}, list[:at]...)
	after = append(after, *moving)
	after = append(after, list[at:]...)
	nums := Renumber(len(after))
	writes := []OrderWrite{}
	for i := range after {
		v, ok := OrderOf(&after[i], b)
		if after[i].Hash == moving.Hash || !ok || v != nums[i] {
			writes = append(writes, OrderWrite{after[i].Hash, nums[i]})
		}
	}
	return writes
}

// --- searching -------------------------------------------------------------------

// Haystack is everything about a card the search looks through - not what the
// card is showing, or a folded card would hide what somebody searched for.
func Haystack(c *Card) string {
	props := []string{}
	for k, v := range c.Props {
		props = append(props, k+" "+v)
	}
	sort.Strings(props)
	return strings.ToLower(fmt.Sprintf("%s %s %s %s %s", c.Headline, c.Status,
		strings.Join(c.Tags, " "), strings.Join(props, " "), c.Filename))
}

func SearchTerms(text string) []string { return strings.Fields(strings.ToLower(text)) }

// CardMatches: every word has to turn up somewhere.
func CardMatches(c *Card, terms []string) bool {
	if len(terms) == 0 {
		return true
	}
	hay := Haystack(c)
	for _, t := range terms {
		if !strings.Contains(hay, t) {
			return false
		}
	}
	return true
}

// --- boards ----------------------------------------------------------------------

var DefaultListFields = []string{"priority", "deadline", "tags"}

func NewBoard(name string) Board {
	return Board{
		Name: name, GroupBy: "status", ShowUnset: true, ColorBy: "none",
		Colors: map[string]string{}, HeaderColors: map[string]string{},
		LabelSource: "none", LabelColors: map[string]string{}, Layout: "board",
		ListFields: append([]string{}, DefaultListFields...), BackShows: "both",
		Sort: "manual", OrderKey: "KANBAN", Columns: []Column{},
	}
}

// Normalize fills in what a hand-edited board left out, the way worg's
// normalizeBoard does, so a board missing a field still opens.
func Normalize(b *Board) {
	if b.GroupBy != "property" && b.GroupBy != "tag" {
		b.GroupBy = "status"
	}
	if !contains([]string{"property", "tag", "status", "priority"}, b.ColorBy) {
		b.ColorBy = "none"
	}
	if b.LabelSource != "tags" && b.LabelSource != "property" {
		b.LabelSource = "none"
	}
	if b.Layout != "list" {
		b.Layout = "board"
	}
	if !contains([]string{"body", "properties", "both"}, b.BackShows) {
		b.BackShows = "both"
	}
	if !contains(Sorts, b.Sort) {
		b.Sort = "manual"
	}
	if b.OrderKey == "" {
		b.OrderKey = "KANBAN"
	}
	if b.ListFields == nil {
		b.ListFields = append([]string{}, DefaultListFields...)
	}
	if b.Folded == nil {
		b.Folded = []string{}
		for _, c := range b.Columns {
			if c.Collapsed {
				b.Folded = append(b.Folded, c.Value)
			}
		}
	}
	if b.Columns == nil {
		b.Columns = []Column{}
	}
	for _, m := range []*map[string]string{&b.Colors, &b.HeaderColors, &b.LabelColors} {
		if *m == nil {
			*m = map[string]string{}
		}
	}
}

func UniqueName(base string, taken []string) string {
	if !contains(taken, base) {
		return base
	}
	for i := 2; ; i++ {
		n := fmt.Sprintf("%s %d", base, i)
		if !contains(taken, n) {
			return n
		}
	}
}

// BoardQuery is the expression a board runs: its saved query when it names
// one, its own otherwise, and archived headings left out unless it asked.
// Parenthesised, so a query with an `||` at the top is not split by the filter.
func BoardQuery(b *Board, stored []StoredQuery) string {
	q := b.Query
	if b.StoredQuery != "" {
		q = ""
		for _, s := range stored {
			if s.Name == b.StoredQuery {
				q = s.Query
			}
		}
	}
	if b.IncludeArchived {
		return q
	}
	return "!IsArchived() && (" + q + ")"
}

// BoardQueryProblem is why a board cannot be run, or "". An empty expression
// is everything to the server, which is never what a board meant.
func BoardQueryProblem(b *Board, stored []StoredQuery) string {
	if b.StoredQuery != "" {
		for _, s := range stored {
			if s.Name == b.StoredQuery {
				if strings.TrimSpace(s.Query) == "" {
					return fmt.Sprintf("The saved query %q is empty.", b.StoredQuery)
				}
				return ""
			}
		}
		return fmt.Sprintf("The saved query %q is not there any more. Pick another, or write one.", b.StoredQuery)
	}
	if strings.TrimSpace(b.Query) == "" {
		return "A board needs a query: pick a saved one, or write an expression."
	}
	return ""
}

// --- the list layout ---------------------------------------------------------------

type ListField struct {
	Key, Label string
	Width      int
	Right      bool
}

var listFields = []ListField{
	{Key: "status", Label: "Keyword", Width: 12},
	{Key: "priority", Label: "Pri", Width: 4},
	{Key: "deadline", Label: "Deadline", Width: 11},
	{Key: "scheduled", Label: "Scheduled", Width: 11},
	{Key: "tags", Label: "Tags", Width: 16},
	{Key: "file", Label: "File", Width: 16},
}

const PropField = "prop:"

func ListFieldOf(key string) ListField {
	k := strings.TrimSpace(key)
	if strings.HasPrefix(k, PropField) {
		n := strings.ToUpper(k[len(PropField):])
		return ListField{Key: PropField + n, Label: n, Width: 12}
	}
	for _, f := range listFields {
		if f.Key == k {
			return f
		}
	}
	return ListField{Key: k, Label: k, Width: 12}
}

// ListFieldsOf is the fields a board's list lays out, the keyword left out
// because it has a column of its own at the front of every row.
func ListFieldsOf(b *Board) []ListField {
	out := []ListField{}
	seen := map[string]bool{}
	for _, k := range b.ListFields {
		f := ListFieldOf(k)
		if seen[f.Key] || f.Key == "status" {
			continue
		}
		seen[f.Key] = true
		out = append(out, f)
	}
	return out
}

func ListFieldText(c *Card, key string) string {
	k := strings.TrimSpace(key)
	if strings.HasPrefix(k, PropField) {
		return strings.TrimSpace(prop(c, strings.ToUpper(k[len(PropField):])))
	}
	switch k {
	case "status":
		return c.Status
	case "priority":
		return c.Priority
	case "deadline":
		return dayText(c.Deadline)
	case "scheduled":
		return dayText(c.Date)
	case "tags":
		return strings.Join(c.Tags, " ")
	case "file":
		return baseName(c.Filename)
	}
	return ""
}

func dayText(d *org.OrgDate) string {
	if !IsSet(d) {
		return ""
	}
	return d.Start.Format("2006-01-02")
}

// DateLabel says a date the way a card does: today and tomorrow by name, the
// weekday within the week, this year without the year.
func DateLabel(t time.Time, now time.Time) string {
	day := func(x time.Time) time.Time { y, m, d := x.Date(); return time.Date(y, m, d, 0, 0, 0, 0, time.Local) }
	days := int(math.Round(day(t).Sub(day(now)).Hours() / 24))
	switch {
	case days == 0:
		return "today"
	case days == 1:
		return "tomorrow"
	case days == -1:
		return "yesterday"
	case days > 1 && days < 7:
		return t.Format("Mon")
	case t.Year() == now.Year():
		return t.Format("Jan 2")
	}
	return t.Format("Jan 2 2006")
}

func baseName(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
