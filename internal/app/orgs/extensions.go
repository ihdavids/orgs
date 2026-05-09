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

// UserExt holds per-user extension data.
type UserExt struct {
	StoredQueries    []StoredQuery            `yaml:"storedQueries" json:"storedQueries"`
	CaptureTemplates []common.CaptureTemplate `yaml:"captureTemplates" json:"captureTemplates"`
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

func RequestStoredQueries(w http.ResponseWriter, r *http.Request) {
	username := GetUsername(r)
	if username == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(GetExtensions().GetStoredQueries(username))
}

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

func RequestUserCaptureTemplates(w http.ResponseWriter, r *http.Request) {
	username := GetUsername(r)
	if username == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(GetExtensions().GetUserCaptureTemplates(username))
}

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
