//lint:file-ignore ST1006 allow the use of self
package orgs

import (
	"crypto/sha1"
	b64 "encoding/base64"
	"fmt"
	"io/fs"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	//"github.com/fsnotify/fsnotify"
	"github.com/dietsche/rfsnotify"
	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

type TableFile struct {
	Table *org.Table
	File  *common.OrgFile
}

type OrgDb struct {
	ByFile       map[string]*common.OrgFile
	ByHash       map[string]*org.Section
	ById         map[string]*org.Section
	ByCustomId   map[string]*org.Section
	ByHashToFile map[string]*common.OrgFile
	NamedTables  map[string][]*TableFile
	Tags         []string
	Filenames    []string
	ReloadIndex  uint64

	dblock      sync.RWMutex
	watcher     *rfsnotify.RWatcher
	watcherdone chan bool
}

func NewOrgDb() *OrgDb {
	var db *OrgDb = new(OrgDb)
	db.ByFile = make(map[string]*common.OrgFile)
	db.ByHashToFile = make(map[string]*common.OrgFile)
	db.ByHash = make(map[string]*org.Section)
	db.ById = make(map[string]*org.Section)
	db.ByCustomId = make(map[string]*org.Section)
	db.NamedTables = make(map[string][]*TableFile)
	db.ReloadIndex = 0
	return db
}

func (self *OrgDb) ConvertTargetToOlp(t *common.Target) error {
	_, sec := self.GetFromTarget(t, false)
	if sec != nil {
		t.Type = "file+olp"
		t.Id = common.BuildOutlinePath(sec, "::")
		return nil
	}
	return fmt.Errorf("failed to find target in database")
}

func (self *OrgDb) FileFromSection(v *org.Section) *common.OrgFile {
	self.dblock.RLock()
	defer self.dblock.RUnlock()
	if f, ok := self.ByHashToFile[v.Hash]; ok {
		return f
	}
	return nil
}

func GetProp(self *org.Section, name ...string) string {
	if self != nil && self.Headline != nil && self.Headline.Properties != nil && len(self.Headline.Properties.Properties) > 0 {
		for _, p := range self.Headline.Properties.Properties {
			k := p[0]
			for _, n := range name {
				if k == n {
					return p[1]
				}
			}
		}
	}
	return ""
}

func (self *OrgDb) RegisterSection(hash string, v *org.Section, d *common.OrgFile) {
	self.dblock.Lock()
	defer self.dblock.Unlock()
	self.ByHash[hash] = v
	self.ByHashToFile[hash] = d
	id := GetProp(v, "ID", "Id", "id")
	if id != "" {
		self.ById[id] = v
	}
	cid := GetProp(v, "CUSTOM_ID", "custom_id", "Custom_Id")
	if cid != "" {
		self.ByCustomId[cid] = v
	}
}

func (self *OrgDb) FindByAnyId(hash string) *org.Section {
	self.dblock.RLock()
	defer self.dblock.RUnlock()
	if v, ok := self.ByHash[hash]; ok {
		return v
	}
	if v, ok := self.ById[hash]; ok {
		return v
	}
	if v, ok := self.ByCustomId[hash]; ok {
		return v
	}
	return nil
}

func (self *OrgDb) FindByHash(hash string) *org.Section {
	self.dblock.RLock()
	defer self.dblock.RUnlock()
	if v, ok := self.ByHash[hash]; ok {
		return v
	}
	return nil
}

// Returns the next sibling after this node
func (self *OrgDb) NextSibling(hash string) *org.Section {
	v := self.FindByHash(hash)
	self.dblock.RLock()
	defer self.dblock.RUnlock()
	if v != nil {
		if v.Parent != nil {
			have := false
			for _, x := range v.Parent.Children {
				if x == v {
					have = true
				} else if have {
					return x
				}
			}
		}
	}
	return nil
}

func (self *OrgDb) PrevSibling(hash string) *org.Section {
	v := self.FindByHash(hash)
	self.dblock.RLock()
	defer self.dblock.RUnlock()
	var prev *org.Section = nil
	if v != nil {
		if v.Parent != nil {
			for _, x := range v.Parent.Children {
				if x == v {
					return prev
				}
				prev = x
			}
		}
	}
	return nil
}

func (self *OrgDb) LastChild(hash string) *org.Section {
	v := self.FindByHash(hash)
	self.dblock.RLock()
	defer self.dblock.RUnlock()
	if v != nil {
		if v.Children != nil {
			l := len(v.Children)
			if l > 0 {
				return v.Children[l-1]
			}
		}
	}
	return nil
}

func (self *OrgDb) _FindByFile(filename string) *common.OrgFile {
	self.dblock.RLock()
	defer self.dblock.RUnlock()
	if v, ok := self.ByFile[filename]; ok {
		return v
	}
	fn := filepath.Base(filename)
	for _, f := range self.Filenames {
		ff := filepath.Base(f)
		if ff == fn {
			return self.ByFile[f]
		}
	}
	return nil
}

func (self *OrgDb) FindByFile(filename string) *common.OrgFile {
	fn := self._FindByFile(filename)
	if fn == nil {
		fn = self._FindByFile(strings.ToLower(filename))
	}
	return fn
}

func GetConfig() *org.Configuration {
	return &org.Configuration{
		AutoLink:            true,
		MaxEmphasisNewLines: 1,
		DefaultSettings: map[string]string{
			"TODO":         Conf().Server.DefaultTodoStates,
			"EXCLUDE_TAGS": "noexport",
			"OPTIONS":      "toc:t <:t e:t f:t pri:t todo:t tags:t title:t ealb:nil",
		},
		Log:      log.New(os.Stderr, "orgs: ", 0),
		ReadFile: ioutil.ReadFile,
	}
}

func IsOrgFile(filename string) bool {
	return filepath.Ext(filename) == ".org"
}

func IsInGitDir(filename string) bool {
	return strings.Contains(filename, ".git")
}

func (self *OrgDb) ListFilesInDir(dirname string) []string {
	var files []string
	err := filepath.Walk(dirname,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			// We only add files, and we should only add org files as well
			// NOTE: This will need to be configurable eventually.
			if !info.IsDir() && IsOrgFile(path) {
				files = append(files, path)
			}
			return nil
		})
	if err != nil {
		log.Println(err)
	}
	return files
}

func (self *OrgDb) ScanNode(v *org.Section, f *common.OrgFile) {
	//self.RegisterSection(v.Hash, v, f)
	for _, t := range v.Headline.Tags {
		if !contains(self.Tags, t) {
			self.Tags = append(self.Tags, t)
		}
	}
	for _, c := range v.Children {
		self.ScanNode(c, f)
	}
}

func (self *OrgDb) CleanupTableRefsForFile(filename string) {
	for k, v := range self.NamedTables {
		didChange := false
		i := 0
		for _, n := range v {
			if n.File.Filename != filename {
				v[i] = n
				i += 1
			} else {
				didChange = true
			}
		}
		if didChange {
			for j := i; j < len(v); j++ {
				v[j] = nil
			}
			v = v[:i]
			self.NamedTables[k] = v
		}
	}
}

func (self *OrgDb) ScanFile(f *common.OrgFile) {
	// Remove old named entries from the list
	self.CleanupTableRefsForFile(f.Filename)
	for _, v := range f.Doc.Outline.Children {
		self.ScanNode(v, f)
	}
	for name, node := range f.Doc.NamedNodes {
		if ntbl, ok := node.(*org.Table); ok {
			if tbl, ok := self.NamedTables[name]; ok {
				self.NamedTables[name] = append(tbl, &TableFile{Table: ntbl, File: f})

			} else {
				self.NamedTables[name] = []*TableFile{&TableFile{Table: ntbl, File: f}}
			}
		}
	}
}

func (self *OrgDb) GetNamedTable(name string, filename string) *TableFile {
	if list, ok := self.NamedTables[name]; ok {
		if len(list) <= 0 {
			return nil
		}
		// For first match in file first
		for _, v := range list {
			if filename == v.File.Filename {
				return v
			}
		}
		// Then return a table outside of this file if not present
		return list[0]
	}
	return nil
}

// Returns the list of named tables
func (self *OrgDb) GetNamedTables(name string) []*TableFile {
	if list, ok := self.NamedTables[name]; ok {
		if len(list) <= 0 {
			return nil
		}
		return list
	}
	return nil
}

func (self *OrgDb) GetTableNames() map[string][]*TableFile {
	return self.NamedTables
}

func (self *OrgDb) GetAllTags() []string {
	return self.Tags
}

func (self *OrgDb) LoadFile(filename string, allowOutsideFiles ...bool) {
	defer func() {
		if err := recover(); err != nil {
			log.Println("PANIC PARSING ORG FILE:", filename, err)
		}
	}()
	doChecks := true
	if len(allowOutsideFiles) > 0 && allowOutsideFiles[0] {
		doChecks = false
	}
	// We expressely forbid trying to load anything that is not an org file.
	// That may bite us in the but later. We also DO NOT want to parse things
	// in our ignore directory, the notification system is indescriminate at the moment.
	if doChecks && (!IsOrgFile(filename) || IsInGitDir(filename)) {
		return
	}
	Conf().Out.Infof("LOAD FILE: %s\n", filename)
	if r, err := os.Open(filename); err == nil {
		d := GetConfig().Parse(r, filename)
		r.Close()
		ofile := new(common.OrgFile)
		ofile.Filename = filename
		ofile.Doc = d
		self.dblock.Lock()
		self.ByFile[filename] = ofile
		// Unique append to our filenames list.
		// NOTE: Reload, it's important to try to maintain the ordering of this list
		//       as it can impact the ordering of todos on requery
		have := false
		for _, f := range self.Filenames {
			if f == filename {
				have = true
				break
			}
		}
		if !have {
			self.Filenames = append(self.Filenames, filename)
		}
		self.ScanFile(ofile)
		// We increment this with each reload to tell if the DB is dirty or not.
		self.ReloadIndex += 1
		self.dblock.Unlock()
	} else {
		fmt.Println("****** Failed to parse file {}", filename)
	}
}

func (self *OrgDb) Close() {
	self.watcher.Close()
}

func (self *OrgDb) Watch() {
	var err error
	self.watcher, err = rfsnotify.NewWatcher()
	if err != nil {
		log.Fatal("NewWatcher failed: ", err)
	}

	self.watcherdone = make(chan bool)
	go func() {
		defer close(self.watcherdone)

		for {
			select {
			case event, ok := <-self.watcher.Events:
				if !ok {
					return
				}
				//log.Printf("EVENT %s %s\n", event.Name, event.Op)
				self.LoadFile(event.Name)
			case err, ok := <-self.watcher.Errors:
				if !ok {
					return
				}
				log.Println("!!!!!!!!!!!!!!! error:", err)
			}
		}

	}()

	var dirs []string = Conf().Server.OrgDirs
	for _, dir := range dirs {
		fmt.Printf("WATCHING: %s\n", dir)
		err = self.watcher.AddRecursive(dir)
		if err != nil {
			if Conf().Server.CanFailWatch {
				log.Println("Watcher add failed:", err)
			} else {
				log.Fatal("Watcher add failed:", err)
			}
		}
	}
}

func (self *OrgDb) RebuildDb() {
	var dirs []string = Conf().Server.OrgDirs
	for _, dir := range dirs {
		files := self.ListFilesInDir(dir)
		for _, file := range files {
			// fmt.Println("Loading: ", file)
			self.LoadFile(file)
		}
	}
}

func (self *OrgDb) GetFiles() []string {
	var filenames []string
	self.dblock.RLock()
	filenames = self.Filenames
	self.dblock.RUnlock()
	return filenames
}

// AVOID: Deprecated avoid using this externally use FindByFile instead
func (self *OrgDb) GetFile(filename string) *common.OrgFile {
	var file *common.OrgFile
	self.dblock.RLock()
	file = self.ByFile[filename]
	self.dblock.RUnlock()
	return file
}

// TARGETTING APIs
/*
type Target struct {
	Filename string
	Id       string
	Line     int
	// File+ types use file and id fields except for line
	// id, customid and hash all just use the id field
	Type string // file+headline, id, customid, hash, file+line
}
*/
// TARGET TYPES
// file              "path/to/file"                               - Text will be placed at the beginning or end of that file.
// id                "id of existing org entry"                   - Filing as child of this entry, or in the body of the entry.
// customid          "customid of existing org entry"             - Filing as child of this entry, or in the body of the entry.
// file+headline     "filename" "node headline"                   - Fast configuration if the target heading is unique in the file.
// file+olp          "filename" "Level 1 heading" "Level 2" ...   - For non-unique headings, the full path is safer.
// file+regexp       "filename" "regexp to find location"         - Use a regular expression to position point.
// file+olp+datetree "filename" [ "Level 1 heading" ...]          - This target83 creates a heading in a date tree84 for today’s date. If the optional outline path is given, the tree will be built under the node it is pointing to, instead of at top level. Check out the :time-prompt and :tree-type properties below for additional options.
// clock                                                          - insert at position of active clock
// hash              dynamically assigned hash                    - during a run of the server nodes are dynamically assigned a hash
//                                                                  use the current dynamic hash to id the node
type EvalFunc func(n *org.Section) bool

func (self *OrgDb) EvalForNodes(nodes []*org.Section, eval EvalFunc) *org.Section {
	for _, n := range nodes {
		if eval(n) {
			return n
		}
		if x := self.EvalForNodes(n.Children, eval); x != nil {
			return x
		}
	}
	return nil
}
func FindChildByName(cur []*org.Section, name string) *org.Section {
	for _, c := range cur {
		title := common.GetSectionTitle(c)
		if title == name {
			return c
		}
	}
	return nil
}

// Create a DateTree path from the current time.
func DateTreeGenerate(curTime *time.Time) (*time.Time, []string) {
	var tree []string
	if curTime == nil {
		ct := time.Now()
		curTime = &ct
	}
	// Todo make this formatting configurable for those that want it
	year := curTime.Format(Conf().DateTreeYearFormat)
	tree = append(tree, year)
	month := curTime.Format(Conf().DateTreeMonthFormat)
	tree = append(tree, month)
	day := curTime.Format(Conf().DateTreeDayFormat)
	tree = append(tree, day)
	return curTime, tree
}

func FindInsertYear(cur []*org.Section, tm *time.Time) int {
	//for i, c := range cur {
	//}
	return -1
}

func (self *OrgDb) GetFilepath(file string) string {
	if filepath.IsAbs(file) {
		return file
	}
	// This may not be a good choice, but it's probably the best we can do
	root := Conf().Server.OrgDirs[0]
	file = filepath.Join(root, file)
	return file
}

func (self *OrgDb) ReloadFile(fname string) *common.OrgFile {
	self.LoadFile(fname, true)
	file := self.FindByFile(fname)
	return file
}

func (self *OrgDb) CreateOrgFile(fname string, title string) *common.OrgFile {
	if _, err := os.Stat(fname); err != nil {
		template := Conf().NewFileTemplate
		var context map[string]interface{} = make(map[string]interface{})
		username := ""
		if usr, ok := user.Current(); ok == nil {
			username = usr.Username
		}
		author := Conf().Author
		if author == "" {
			author = username
		}
		context["day_page_title"] = title
		context["title"] = title
		context["author"] = Conf().Author
		data := Conf().PlugManager.Tempo.RenderTemplate(template, context)
		fmt.Printf("WRITING ORG FILE %s\n", fname)
		ioutil.WriteFile(fname, []byte(data), fs.ModePerm)
		return self.ReloadFile(fname)
	} else {
		fmt.Printf("RELOAD ATTEMPT: %s\n", fname)
		return self.ReloadFile(fname)
	}
	return nil
}

func (self *OrgDb) FindNodeByHeadline(file *common.OrgFile, target *common.Target) *org.Section {
	node := self.EvalForNodes(file.Doc.Outline.Children, func(n *org.Section) bool {
		var title string = common.GetSectionTitle(n)
		// Hard level requirement in search
		if target.Lvl > 0 {
			return title == target.Id && target.Lvl == n.Headline.Lvl
		}
		return title == target.Id
	})
	return node
}

func (self *OrgDb) AppendNodeToFile(file *common.OrgFile, target *common.Target) (*common.OrgFile, *org.Section) {
	sec := new(org.Section)
	sec.Children = []*org.Section{}
	row := 0
	lvl := 1
	title := target.Id
	p := org.Pos{Row: row, Col: 0}
	ep := org.Pos{Row: row, Col: p.Col + len(title)}
	txt := org.Text{Pos: p, EndPos: ep, Content: title}
	h := org.Headline{}
	sec.Headline = &h
	sec.Headline.Title = []org.Node{txt}
	sec.Headline.Lvl = lvl

	tHash := sha1.New()
	tHash.Write([]byte(title))
	sec.Hash = b64.StdEncoding.EncodeToString(tHash.Sum(nil))
	res := common.ResultMsg{}
	InsertSection(file, sec, file.Doc.Outline.Section, &res)
	file = self.ReloadFile(file.Filename)
	node := self.FindNodeByHeadline(file, target)
	return file, node
}

// Find an olp path, if allowed to create recursively add nodes and reload file to make full olp path
func (self *OrgDb) FindByOlp(target *common.Target, allowCreate bool) (*common.OrgFile, *org.Section) {
	file := self.FindByFile(target.Filename)
	if file == nil && allowCreate {
		fname := self.GetFilepath(target.Filename)
		file = self.CreateOrgFile(fname, "")
	}
	olp := strings.Split(target.Id, "::")
	if len(olp) == 0 {
		return file, file.Doc.Outline.Section
	}
	cur := file.Doc.Outline.Children
	var parent *org.Section
	var sec *org.Section = nil
	for _, name := range olp {
		sec = FindChildByName(cur, name)
		if sec == nil {
			if allowCreate {
				self.AppendNodeAtNode(file, target, parent, name)
				// Start over, the file got rewritten!
				return self.FindByOlp(target, allowCreate)
			} else {
				return nil, nil
			}
		} else {
			cur = sec.Children
			parent = sec
		}
	}
	return file, sec
}

func (self *OrgDb) AppendNodeAtNode(file *common.OrgFile, target *common.Target, parent *org.Section, name string) bool {
	sec := new(org.Section)
	sec.Children = []*org.Section{}
	row := 0
	lvl := 1
	if parent != nil {
		lvl = parent.Headline.Lvl + 1
	}
	title := name
	p := org.Pos{Row: row, Col: 0}
	ep := org.Pos{Row: row, Col: p.Col + len(title)}
	txt := org.Text{Pos: p, EndPos: ep, Content: title}
	h := org.Headline{}
	sec.Headline = &h
	sec.Headline.Title = []org.Node{txt}
	sec.Headline.Lvl = lvl

	tHash := sha1.New()
	tHash.Write([]byte(title))
	sec.Hash = b64.StdEncoding.EncodeToString(tHash.Sum(nil))
	res := common.ResultMsg{}
	InsertSection(file, sec, parent, &res)
	self.ReloadFile(file.Filename)

	return res.Ok
}

func (self *OrgDb) FindDateTree(target *common.Target, tree []string, parent *org.Section, allowCreate bool) (*common.OrgFile, *org.Section) {
	file := self.FindByFile(target.Filename)
	if file == nil && allowCreate {
		fname := self.GetFilepath(target.Filename)
		file = self.CreateOrgFile(fname, "")
	}
	cur := parent.Children
	sec := parent
	prv := parent
	for _, name := range tree {
		prv = sec
		sec = FindChildByName(cur, name)
		if sec == nil {
			if allowCreate {
				self.AppendNodeAtNode(file, target, prv, name)
				// Start over, the file got rewritten!
				return self.GetFromTarget(target, allowCreate)
			}
			return nil, nil
		} else {
			cur = sec.Children
		}
	}
	return file, sec
}

func (self *OrgDb) GetOrCreateFile(target *common.Target, allowCreate bool) *common.OrgFile {
	file := self.FindByFile(target.Filename)
	if file == nil && allowCreate {
		fname := self.GetFilepath(target.Filename)
		file = self.CreateOrgFile(fname, "")
	}
	return file
}

func DiffRows(a, b org.Pos) int {
	return b.Row - a.Row
}

func GetSubObject(cur org.Node, row int, typeId org.NodeType) org.Node {

	for _, c := range cur.GetChildren() {
		if row < c.GetPos().Row {
			return nil
		}
		if row >= c.GetPos().Row && row <= c.GetEnd().Row {
			if c.GetType() == typeId {
				return c
			}
		} else if typeId == org.TableNode && c.GetType() == typeId && row >= c.GetEnd().Row {
			if tbl, ok := c.(*org.Table); ok && tbl.Formulas != nil && len(tbl.Formulas.Keywords) > 0 {
				for _, frml := range tbl.Formulas.Keywords {
					if row >= frml.GetPos().Row && row <= frml.GetEnd().Row {
						return c
					}
				}
			}
		}

	}
	return nil
}

func (self *OrgDb) GetFromPreciseTarget(target *common.PreciseTarget, typeId org.NodeType) (*common.OrgFile, *org.Section, org.Node) {
	ofile, sec := self.GetFromTarget(&target.Target, false)
	var node org.Node = nil
	if sec != nil {
		if target.Row > sec.Headline.GetEnd().Row {
			return nil, nil, nil
		}
		node = sec.Headline
		for _, c := range sec.Headline.Children {
			if target.Row < c.GetPos().Row {
				return ofile, sec, node
			} else if target.Row >= c.GetPos().Row && target.Row <= c.GetEnd().Row {
				if c.GetType() == typeId {
					return ofile, sec, c
				}
				node = GetSubObject(c, target.Row, typeId)
				if node != nil {
					return ofile, sec, node
				}
				// Include formulas in tables
			} else if typeId == org.TableNode && c.GetType() == typeId && target.Row >= c.GetEnd().Row {
				if tbl, ok := c.(*org.Table); ok && tbl.Formulas != nil && len(tbl.Formulas.Keywords) > 0 {
					for _, frml := range tbl.Formulas.Keywords {
						if target.Row >= frml.GetPos().Row && target.Row <= frml.GetEnd().Row {
							return ofile, sec, c
						}
					}
				}
			}
		}
	}
	return nil, nil, nil
}

func (self *OrgDb) GetFromTarget(target *common.Target, allowCreate bool) (*common.OrgFile, *org.Section) {
	switch target.Type {
	case "file":
		file := self.GetOrCreateFile(target, allowCreate)
		if file == nil {
			return nil, nil
		}
		sec := file.Doc.Outline.Section
		if sec != nil && sec.Headline == nil {
			sec.Headline = &org.Headline{}
			if file.Doc.Outline.Children != nil && len(file.Doc.Outline.Children) > 0 {
				sec.Headline.EndPos = file.Doc.Outline.Children[len(file.Doc.Outline.Children)-1].Headline.EndPos
				sec.Headline.Pos = file.Doc.Outline.Children[len(file.Doc.Outline.Children)-1].Headline.Pos
			}
		}
		return file, file.Doc.Outline.Section
	case "hash":
		if f, ok := self.ByHash[target.Id]; ok {
			return self.FileFromSection(f), f
		}
		return nil, nil
	case "id":
		if f, ok := self.ById[target.Id]; ok {
			return self.FileFromSection(f), f
		}
		return nil, nil
	case "customid":
		if f, ok := self.ByCustomId[target.Id]; ok {
			return self.FileFromSection(f), f
		}
		return nil, nil
	case "file+olp":
		return self.FindByOlp(target, allowCreate)
	case "file+olp+datetree":
		file, sec := self.FindByOlp(target, allowCreate)
		if sec != nil {
			// Now do the datetree!
			_, dt := DateTreeGenerate(nil)
			return self.FindDateTree(target, dt, sec, allowCreate)
		}
		return file, sec
	case "file+datetree":
		_, dt := DateTreeGenerate(nil)
		file := self.GetOrCreateFile(target, allowCreate)
		if file == nil {
			return nil, nil
		}
		sec := file.Doc.Outline.Section
		return self.FindDateTree(target, dt, sec, allowCreate)
	case "file+headline":
		file := self.FindByFile(target.Filename)
		if file == nil && allowCreate {
			fname := self.GetFilepath(target.Filename)
			file = self.CreateOrgFile(fname, "")
		}
		if file != nil {
			node := self.FindNodeByHeadline(file, target)
			if node == nil && allowCreate {
				file, node = self.AppendNodeToFile(file, target)
			}
			return file, node
		}
		return nil, nil
	case "file+regexp":
		file := self.FindByFile(target.Filename)
		if file == nil && allowCreate {
			fname := self.GetFilepath(target.Filename)
			file = self.CreateOrgFile(fname, "")
		}
		re, err := regexp.Compile(target.Id)
		if err != nil {
			return nil, nil
		}
		node := self.EvalForNodes(file.Doc.Outline.Children, func(n *org.Section) bool {
			var title string = common.GetSectionTitle(n)
			if re.MatchString(title) {
				return true
			}
			for _, s := range n.Headline.Children {
				if re.MatchString(s.String()) {
					return true
				}
			}
			return false
		})
		return file, node
	}
	return nil, nil
}

var odb *OrgDb = nil

func GetDb() *OrgDb {
	if odb == nil {
		odb = NewOrgDb()
		odb.RebuildDb()
	}
	return odb
}
