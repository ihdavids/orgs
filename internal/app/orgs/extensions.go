//lint:file-ignore ST1006 allow the use of self
package orgs

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ihdavids/orgs/internal/common"
	"gopkg.in/yaml.v2"
)

// StoredQuery is a named query expression that a user can save and recall.
type StoredQuery struct {
	Name  string `yaml:"name" json:"name"`
	Query string `yaml:"query" json:"query"`
}

// KanbanColumn is one column of a kanban board.
//
// Value is what puts a card in this column, read against whatever the board
// groups by: the value of the grouping property, the tag name, or the todo
// keyword. A column whose Value is empty is the one that catches the cards no
// other column claims.
type KanbanColumn struct {
	Value string `yaml:"value" json:"value"`
	// What the column is called on screen. Empty means show Value.
	Title string `yaml:"title" json:"title"`
	// A palette colour name, used for the column heading. Empty is the default.
	Color string `yaml:"color" json:"color"`
	// Work in progress limit. Zero means no limit; the board only warns.
	Limit int `yaml:"limit" json:"limit"`
	// Drawn folded to a spine until it is opened again.
	Collapsed bool `yaml:"collapsed" json:"collapsed"`
}

// KanbanBoard is one saved board: where its cards come from, what splits them
// into columns, and how they are drawn.
//
// Nothing here is derived from the org files - a board is a way of looking at
// them, and every card on it is worked out afresh from the query each time it
// is opened. The one exception is manual sorting, which needs somewhere to
// write the order down, and writes it to a numeric property on the heading
// itself (OrderKey) rather than keeping a list of headings here that would go
// stale the moment one of them was edited somewhere else.
type KanbanBoard struct {
	Name string `yaml:"name" json:"name"`
	// The cards. StoredQuery names one of the user's saved queries and wins
	// when it is set; Query is an expression written on the board itself.
	StoredQuery string `yaml:"storedQuery" json:"storedQuery"`
	Query       string `yaml:"query" json:"query"`
	// Archived headings are left out unless this is set.
	IncludeArchived bool `yaml:"includeArchived" json:"includeArchived"`
	// What splits the cards into columns: property, tag or status. GroupKey is
	// the property name when GroupBy is property, and is ignored otherwise.
	GroupBy  string `yaml:"groupBy" json:"groupBy"`
	GroupKey string `yaml:"groupKey" json:"groupKey"`
	Columns  []KanbanColumn `yaml:"columns" json:"columns"`
	// Keep a column for the cards no column claimed.
	ShowUnset  bool   `yaml:"showUnset" json:"showUnset"`
	UnsetTitle string `yaml:"unsetTitle" json:"unsetTitle"`
	// What colours a card: none, property, tag, status or priority. ColorKey is
	// the property name when ColorBy is property. Colors maps a value to a
	// palette colour name; a value with no entry is given one from its own
	// spelling, so a board is never uncoloured just because nobody said so.
	ColorBy  string            `yaml:"colorBy" json:"colorBy"`
	ColorKey string            `yaml:"colorKey" json:"colorKey"`
	Colors   map[string]string `yaml:"colors" json:"colors"`
	// Properties shown as chips on the front of a card.
	Badges []string `yaml:"badges" json:"badges"`
	// What the back of a card shows: body, properties or both.
	BackShows string `yaml:"backShows" json:"backShows"`
	// How the cards within a column are ordered: headline, priority, deadline,
	// scheduled, file or manual. OrderKey is the numeric property manual order
	// is written to.
	Sort     string `yaml:"sort" json:"sort"`
	OrderKey string `yaml:"orderKey" json:"orderKey"`
}

// UserExt holds per-user extension data.
type UserExt struct {
	StoredQueries    []StoredQuery            `yaml:"storedQueries" json:"storedQueries"`
	CaptureTemplates []common.CaptureTemplate `yaml:"captureTemplates" json:"captureTemplates"`
	KanbanBoards     []KanbanBoard            `yaml:"kanbanBoards" json:"kanbanBoards"`
}

// ExtensionsConfig is the root of the per-user extensions YAML file.
type ExtensionsConfig struct {
	mu    sync.RWMutex
	path  string
	Users map[string]*UserExt `yaml:"users"`
}

var extensions *ExtensionsConfig

// GetExtensions returns the singleton extensions config.
func GetExtensions() *ExtensionsConfig {
	return extensions
}

// extensionsPath derives the extensions file path from the main config path.
func extensionsPath() string {
	cfgPath := Conf().Config
	dir := filepath.Dir(cfgPath)
	base := filepath.Base(cfgPath)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	return filepath.Join(dir, name+"_extensions"+ext)
}

// LoadExtensions loads or initializes the per-user extensions config.
func LoadExtensions() {
	path := extensionsPath()
	ec := &ExtensionsConfig{
		path:  path,
		Users: make(map[string]*UserExt),
	}
	data, err := os.ReadFile(path)
	if err == nil {
		if err := yaml.Unmarshal(data, ec); err != nil {
			fmt.Printf("Extensions: failed to parse %s: %v\n", path, err)
		}
	}
	if ec.Users == nil {
		ec.Users = make(map[string]*UserExt)
	}
	extensions = ec
	fmt.Printf("Extensions: loaded from %s (%d users)\n", path, len(ec.Users))
}

// save writes the extensions config back to disk. Must be called with mu held for writing.
func (self *ExtensionsConfig) save() error {
	data, err := yaml.Marshal(self)
	if err != nil {
		return fmt.Errorf("extensions: marshal error: %w", err)
	}
	if err := os.WriteFile(self.path, data, 0644); err != nil {
		return fmt.Errorf("extensions: write error: %w", err)
	}
	return nil
}

// getUser returns the UserExt for username, creating it if needed.
// Must be called with mu held.
func (self *ExtensionsConfig) getUser(username string) *UserExt {
	u, ok := self.Users[username]
	if !ok {
		u = &UserExt{}
		self.Users[username] = u
	}
	return u
}

// ---------------------------------------------------------------------------
// Stored Queries
// ---------------------------------------------------------------------------

func (self *ExtensionsConfig) GetStoredQueries(username string) []StoredQuery {
	self.mu.RLock()
	defer self.mu.RUnlock()
	u, ok := self.Users[username]
	if !ok {
		return []StoredQuery{}
	}
	return u.StoredQueries
}

func (self *ExtensionsConfig) GetStoredQuery(username, name string) *StoredQuery {
	self.mu.RLock()
	defer self.mu.RUnlock()
	u, ok := self.Users[username]
	if !ok {
		return nil
	}
	for i := range u.StoredQueries {
		if u.StoredQueries[i].Name == name {
			return &u.StoredQueries[i]
		}
	}
	return nil
}

func (self *ExtensionsConfig) SetStoredQuery(username string, sq StoredQuery) error {
	self.mu.Lock()
	defer self.mu.Unlock()
	u := self.getUser(username)
	for i := range u.StoredQueries {
		if u.StoredQueries[i].Name == sq.Name {
			u.StoredQueries[i].Query = sq.Query
			return self.save()
		}
	}
	u.StoredQueries = append(u.StoredQueries, sq)
	return self.save()
}

func (self *ExtensionsConfig) DeleteStoredQuery(username, name string) error {
	self.mu.Lock()
	defer self.mu.Unlock()
	u, ok := self.Users[username]
	if !ok {
		return fmt.Errorf("no extensions for user %s", username)
	}
	for i := range u.StoredQueries {
		if u.StoredQueries[i].Name == name {
			u.StoredQueries = append(u.StoredQueries[:i], u.StoredQueries[i+1:]...)
			return self.save()
		}
	}
	return fmt.Errorf("stored query %q not found", name)
}

// ---------------------------------------------------------------------------
// User Capture Templates
// ---------------------------------------------------------------------------

func (self *ExtensionsConfig) GetUserCaptureTemplates(username string) []common.CaptureTemplate {
	self.mu.RLock()
	defer self.mu.RUnlock()
	u, ok := self.Users[username]
	if !ok {
		return []common.CaptureTemplate{}
	}
	return u.CaptureTemplates
}

func (self *ExtensionsConfig) SetUserCaptureTemplate(username string, ct common.CaptureTemplate) error {
	self.mu.Lock()
	defer self.mu.Unlock()
	u := self.getUser(username)
	for i := range u.CaptureTemplates {
		if u.CaptureTemplates[i].Name == ct.Name {
			u.CaptureTemplates[i] = ct
			return self.save()
		}
	}
	u.CaptureTemplates = append(u.CaptureTemplates, ct)
	return self.save()
}

func (self *ExtensionsConfig) DeleteUserCaptureTemplate(username, name string) error {
	self.mu.Lock()
	defer self.mu.Unlock()
	u, ok := self.Users[username]
	if !ok {
		return fmt.Errorf("no extensions for user %s", username)
	}
	for i := range u.CaptureTemplates {
		if u.CaptureTemplates[i].Name == name {
			u.CaptureTemplates = append(u.CaptureTemplates[:i], u.CaptureTemplates[i+1:]...)
			return self.save()
		}
	}
	return fmt.Errorf("capture template %q not found", name)
}

// ---------------------------------------------------------------------------
// REST Handlers — Stored Queries
// ---------------------------------------------------------------------------

/* SDOC: API
* GET /ext/queries — List Stored Queries
	Returns all stored queries for the authenticated user. Stored queries are named
	search expressions that users can save for quick recall. They are persisted in the
	per-user extensions YAML file.

	*Method:* =GET=

	*Parameters:* None (user identity is derived from the auth token).

	*Response:* A JSON array of =StoredQuery= objects:
	#+BEGIN_SRC json
	[{"name": "My Tasks", "query": "TODO=\"TODO\"+HOME"}]
	#+END_SRC
	Returns an empty array if the user has no stored queries.

	*Errors:* =401= if not authenticated.
	EDOC */
func RequestStoredQueries(w http.ResponseWriter, r *http.Request) {
	username := GetUsername(r)
	if username == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(GetExtensions().GetStoredQueries(username))
}

/* SDOC: API
* GET /ext/query — Get a Stored Query by Name
	Returns a single stored query for the authenticated user, looked up by name.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter | Type   | Required | Description                           |
	|-----------+--------+----------+---------------------------------------|
	| =name=    | string | yes      | The name of the stored query.         |

	*Response:* A =StoredQuery= JSON object: ={"name": "...", "query": "..."}=.

	*Errors:*
	- =401= if not authenticated.
	- =400= if =name= is missing.
	- =404= if the named query does not exist.
	EDOC */
func RequestStoredQuery(w http.ResponseWriter, r *http.Request) {
	username := GetUsername(r)
	if username == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: "missing name parameter"})
		return
	}
	sq := GetExtensions().GetStoredQuery(username, name)
	if sq == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: fmt.Sprintf("stored query %q not found", name)})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sq)
}

/* SDOC: API
* POST /ext/query — Create or Update a Stored Query
	Creates a new stored query or updates an existing one (matched by name) for the
	authenticated user. The query is persisted to the extensions YAML file.

	*Method:* =POST=

	*Request Body (JSON):*
	| Field   | Type   | Required | Description                                  |
	|---------+--------+----------+----------------------------------------------|
	| =name=  | string | yes      | The name for the stored query.               |
	| =query= | string | yes      | The search expression to save.               |

	*Response:* A =ResultMsg= JSON object confirming the save.

	*Errors:*
	- =401= if not authenticated.
	- =400= if =name= or =query= is empty.
	EDOC */
func PostStoredQuery(w http.ResponseWriter, r *http.Request) {
	username := GetUsername(r)
	if username == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	body, _ := io.ReadAll(r.Body)
	var sq StoredQuery
	if err := json.Unmarshal(body, &sq); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	if sq.Name == "" || sq.Query == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: "name and query are required"})
		return
	}
	if err := GetExtensions().SetStoredQuery(username, sq); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(common.ResultMsg{Ok: true, Msg: fmt.Sprintf("stored query %q saved", sq.Name)})
}

/* SDOC: API
* DELETE /ext/query — Delete a Stored Query
	Deletes a stored query by name for the authenticated user. The change is
	persisted to the extensions YAML file.

	*Method:* =DELETE=

	*Query Parameters:*
	| Parameter | Type   | Required | Description                           |
	|-----------+--------+----------+---------------------------------------|
	| =name=    | string | yes      | The name of the stored query to delete.|

	*Response:* A =ResultMsg= JSON object confirming the deletion.

	*Errors:*
	- =401= if not authenticated.
	- =400= if =name= is missing.
	- =404= if the named query does not exist.
	EDOC */
func DeleteStoredQuery(w http.ResponseWriter, r *http.Request) {
	username := GetUsername(r)
	if username == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: "missing name parameter"})
		return
	}
	if err := GetExtensions().DeleteStoredQuery(username, name); err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(common.ResultMsg{Ok: true, Msg: fmt.Sprintf("stored query %q deleted", name)})
}

// ---------------------------------------------------------------------------
// REST Handlers — User Capture Templates
// ---------------------------------------------------------------------------

/* SDOC: API
* GET /ext/capture/templates — List User Capture Templates
	Returns the per-user capture templates for the authenticated user. These are in
	addition to the server-wide templates defined in config. Per-user templates are
	stored in the extensions YAML file.

	*Method:* =GET=

	*Parameters:* None (user identity is derived from the auth token).

	*Response:* A JSON array of =CaptureTemplate= objects. Returns an empty array if
	the user has no custom templates.

	*Errors:* =401= if not authenticated.
	EDOC */
func RequestUserCaptureTemplates(w http.ResponseWriter, r *http.Request) {
	username := GetUsername(r)
	if username == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(GetExtensions().GetUserCaptureTemplates(username))
}

/* SDOC: API
* POST /ext/capture/template — Create or Update a User Capture Template
	Creates a new per-user capture template or updates an existing one (matched by name).
	The template is persisted to the extensions YAML file.

	*Method:* =POST=

	*Request Body (JSON):* A =CaptureTemplate= object.
	| Field      | Type   | Required | Description                                              |
	|------------+--------+----------+----------------------------------------------------------|
	| =name=     | string | yes      | The template name (used for matching and display).       |
	| =type=     | string | no       | Entry type (e.g. =entry=).                               |
	| =target=   | Target | no       | Where captured content should be filed.                  |
	| =template= | string | no       | Template text suggestion for the calling program.        |

	*Response:* A =ResultMsg= JSON object confirming the save.

	*Errors:*
	- =401= if not authenticated.
	- =400= if =name= is empty.
	EDOC */
func PostUserCaptureTemplate(w http.ResponseWriter, r *http.Request) {
	username := GetUsername(r)
	if username == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	body, _ := io.ReadAll(r.Body)
	var ct common.CaptureTemplate
	if err := json.Unmarshal(body, &ct); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	if ct.Name == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: "name is required"})
		return
	}
	if err := GetExtensions().SetUserCaptureTemplate(username, ct); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(common.ResultMsg{Ok: true, Msg: fmt.Sprintf("capture template %q saved", ct.Name)})
}

/* SDOC: API
* DELETE /ext/capture/template — Delete a User Capture Template
	Deletes a per-user capture template by name. The change is persisted to the
	extensions YAML file.

	*Method:* =DELETE=

	*Query Parameters:*
	| Parameter | Type   | Required | Description                                   |
	|-----------+--------+----------+-----------------------------------------------|
	| =name=    | string | yes      | The name of the capture template to delete.   |

	*Response:* A =ResultMsg= JSON object confirming the deletion.

	*Errors:*
	- =401= if not authenticated.
	- =400= if =name= is missing.
	- =404= if the named template does not exist.
	EDOC */
func DeleteUserCaptureTemplate(w http.ResponseWriter, r *http.Request) {
	username := GetUsername(r)
	if username == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: "missing name parameter"})
		return
	}
	if err := GetExtensions().DeleteUserCaptureTemplate(username, name); err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(common.ResultMsg{Ok: true, Msg: fmt.Sprintf("capture template %q deleted", name)})
}

// ---------------------------------------------------------------------------
// Kanban Boards
// ---------------------------------------------------------------------------

func (self *ExtensionsConfig) GetKanbanBoards(username string) []KanbanBoard {
	self.mu.RLock()
	defer self.mu.RUnlock()
	u, ok := self.Users[username]
	if !ok || u.KanbanBoards == nil {
		return []KanbanBoard{}
	}
	return u.KanbanBoards
}

// SetKanbanBoard adds a board or replaces the one that already has its name.
func (self *ExtensionsConfig) SetKanbanBoard(username string, b KanbanBoard) error {
	self.mu.Lock()
	defer self.mu.Unlock()
	u := self.getUser(username)
	for i := range u.KanbanBoards {
		if u.KanbanBoards[i].Name == b.Name {
			u.KanbanBoards[i] = b
			return self.save()
		}
	}
	u.KanbanBoards = append(u.KanbanBoards, b)
	return self.save()
}

// SetKanbanBoards replaces every board the user has in one write. Renaming a
// board and reordering the tabs both change a list rather than one entry, and
// doing them as a delete plus an add would leave the boards half written if the
// second call never arrived.
func (self *ExtensionsConfig) SetKanbanBoards(username string, boards []KanbanBoard) error {
	self.mu.Lock()
	defer self.mu.Unlock()
	u := self.getUser(username)
	if boards == nil {
		boards = []KanbanBoard{}
	}
	u.KanbanBoards = boards
	return self.save()
}

func (self *ExtensionsConfig) DeleteKanbanBoard(username, name string) error {
	self.mu.Lock()
	defer self.mu.Unlock()
	u, ok := self.Users[username]
	if !ok {
		return fmt.Errorf("no extensions for user %s", username)
	}
	for i := range u.KanbanBoards {
		if u.KanbanBoards[i].Name == name {
			u.KanbanBoards = append(u.KanbanBoards[:i], u.KanbanBoards[i+1:]...)
			return self.save()
		}
	}
	return fmt.Errorf("kanban board %q not found", name)
}

// ---------------------------------------------------------------------------
// REST Handlers — Kanban Boards
// ---------------------------------------------------------------------------

/* SDOC: API
* GET /ext/kanban/boards — List Kanban Boards
	Returns every kanban board the authenticated user has saved, in the order they
	are shown as tabs. A board says where its cards come from (a stored query name or
	a query expression), what splits them into columns, and how they are drawn. It
	holds no cards itself - those are worked out from the query each time the board
	is opened. Boards live in the per-user extensions YAML file.

	*Method:* =GET=

	*Parameters:* None (user identity is derived from the auth token).

	*Response:* A JSON array of =KanbanBoard= objects:
	#+BEGIN_SRC json
	[{"name": "Notes", "storedQuery": "All Notes", "groupBy": "property",
	  "groupKey": "STAGE", "columns": [{"value": "idea", "title": "Ideas"}],
	  "colorBy": "property", "colorKey": "AREA", "sort": "manual",
	  "orderKey": "KANBAN"}]
	#+END_SRC
	Returns an empty array if the user has no boards.

	*Errors:* =401= if not authenticated.
	EDOC */
func RequestKanbanBoards(w http.ResponseWriter, r *http.Request) {
	username := GetUsername(r)
	if username == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(GetExtensions().GetKanbanBoards(username))
}

/* SDOC: API
* POST /ext/kanban/boards — Replace Every Kanban Board
	Writes the user's whole board list at once. This is what renaming a board and
	reordering the tabs go through: both change the list rather than one entry, and
	doing them as a delete followed by an add would leave the boards half written if
	the second call never arrived.

	*Method:* =POST=

	*Request Body (JSON):* An array of =KanbanBoard= objects, in tab order. An empty
	array removes every board.

	*Response:* A =ResultMsg= JSON object confirming the save.

	*Errors:*
	- =401= if not authenticated.
	- =400= if the body is not an array of boards, or a board has no name.
	EDOC */
func PostKanbanBoards(w http.ResponseWriter, r *http.Request) {
	username := GetUsername(r)
	if username == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	body, _ := io.ReadAll(r.Body)
	var boards []KanbanBoard
	if err := json.Unmarshal(body, &boards); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	seen := map[string]bool{}
	for _, b := range boards {
		if b.Name == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: "every board needs a name"})
			return
		}
		if seen[b.Name] {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: fmt.Sprintf("two boards named %q", b.Name)})
			return
		}
		seen[b.Name] = true
	}
	if err := GetExtensions().SetKanbanBoards(username, boards); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(common.ResultMsg{Ok: true, Msg: fmt.Sprintf("%d kanban board(s) saved", len(boards))})
}

/* SDOC: API
* POST /ext/kanban/board — Create or Update One Kanban Board
	Creates a board or replaces the one that already has its name, leaving the rest
	of the user's boards alone. Use =POST /ext/kanban/boards= to rename a board or
	change the order of the tabs.

	*Method:* =POST=

	*Request Body (JSON):* A =KanbanBoard= object. Only =name= is required; a board
	with no query shows no cards, and one with no columns offers to fill them in
	from whatever the query finds.

	*Response:* A =ResultMsg= JSON object confirming the save.

	*Errors:*
	- =401= if not authenticated.
	- =400= if the body is not a board, or =name= is empty.
	EDOC */
func PostKanbanBoard(w http.ResponseWriter, r *http.Request) {
	username := GetUsername(r)
	if username == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	body, _ := io.ReadAll(r.Body)
	var b KanbanBoard
	if err := json.Unmarshal(body, &b); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	if b.Name == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: "name is required"})
		return
	}
	if err := GetExtensions().SetKanbanBoard(username, b); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(common.ResultMsg{Ok: true, Msg: fmt.Sprintf("kanban board %q saved", b.Name)})
}

/* SDOC: API
* DELETE /ext/kanban/board — Delete a Kanban Board
	Deletes one board by name. Only the board goes - a board is a way of looking at
	the org files and owns nothing in them, so no heading is touched.

	*Method:* =DELETE=

	*Query Parameters:*
	| Parameter | Type   | Required | Description                        |
	|-----------+--------+----------+------------------------------------|
	| =name=    | string | yes      | The name of the board to delete.   |

	*Response:* A =ResultMsg= JSON object confirming the deletion.

	*Errors:*
	- =401= if not authenticated.
	- =400= if =name= is missing.
	- =404= if no board has that name.
	EDOC */
func DeleteKanbanBoard(w http.ResponseWriter, r *http.Request) {
	username := GetUsername(r)
	if username == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: "missing name parameter"})
		return
	}
	if err := GetExtensions().DeleteKanbanBoard(username, name); err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(common.ResultMsg{Ok: true, Msg: fmt.Sprintf("kanban board %q deleted", name)})
}
