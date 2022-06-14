package orgs

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	//"github.com/fsnotify/fsnotify"
	"github.com/dietsche/rfsnotify"
	"github.com/ihdavids/go-org/org"
)

type OrgFile struct {
	filename string
	doc      *org.Document
}

type OrgDb struct {
	ByFile      map[string]*OrgFile
	ByHash      map[string]*org.Section
	Filenames   []string
	ReloadIndex uint64

	dblock      sync.RWMutex
	watcher     *rfsnotify.RWatcher
	watcherdone chan bool
}

func NewOrgDb() *OrgDb {
	var db *OrgDb = new(OrgDb)
	db.ByFile = make(map[string]*OrgFile)
	db.ByHash = make(map[string]*org.Section)
	db.ReloadIndex = 0
	return db
}

func (self *OrgDb) RegisterSection(hash string, v *org.Section) {
	self.ByHash[hash] = v
}

func (self *OrgDb) FindByHash(hash string) *org.Section {
	if v, ok := self.ByHash[hash]; ok {
		return v
	}
	return nil
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
			if !info.IsDir() && filepath.Ext(path) == ".org" {
				files = append(files, path)
			}
			return nil
		})
	if err != nil {
		log.Println(err)
	}
	return files
}

func (self *OrgDb) LoadFile(filename string) {
	if r, err := os.Open(filename); err == nil {
		d := org.New().Parse(r, filename)
		ofile := new(OrgFile)
		ofile.filename = filename
		ofile.doc = d
		self.dblock.Lock()
		self.ByFile[filename] = ofile
		self.Filenames = append(self.Filenames, filename)
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
	fmt.Printf("WATCHER STARTED")
	self.watcher, err = rfsnotify.NewWatcher()
	if err != nil {
		log.Fatal("NewWatcher failed: ", err)
	}

	fmt.Printf("WATCHER CREATING CHANNEL")
	self.watcherdone = make(chan bool)
	go func() {
		defer close(self.watcherdone)

		for {
			select {
			case event, ok := <-self.watcher.Events:
				if !ok {
					fmt.Printf("WATCHER KILLED")
					fmt.Printf("WATCHER KILLED")
					fmt.Printf("WATCHER KILLED")
					fmt.Printf("WATCHER KILLED")
					fmt.Printf("WATCHER KILLED")
					fmt.Printf("WATCHER KILLED")
					return
				}
				log.Printf("EVENT %s %s\n", event.Name, event.Op)
				self.LoadFile(event.Name)
			case err, ok := <-self.watcher.Errors:
				if !ok {
					fmt.Printf("WATCHER PPPPPP")
					fmt.Printf("WATCHER PPPPPP")
					fmt.Printf("WATCHER PPPPPP")
					fmt.Printf("WATCHER PPPPPP")
					fmt.Printf("WATCHER PPPPPP")
					return
				}
				log.Println("!!!!!!!!!!!!!!! error:", err)
			}
		}

	}()

	var dirs []string = Conf().OrgDirs
	for _, dir := range dirs {
		fmt.Printf("WATCHING: %s", dir)
		err = self.watcher.AddRecursive(dir)
		if err != nil {
			log.Fatal("Watcher add failed:", err)
		}
	}
}

func (self *OrgDb) RebuildDb() {
	var dirs []string = Conf().OrgDirs
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

func (self *OrgDb) GetFile(filename string) *OrgFile {
	var file *OrgFile
	self.dblock.RLock()
	file = self.ByFile[filename]
	self.dblock.RUnlock()
	return file
}

var odb *OrgDb = nil

func GetDb() *OrgDb {
	if odb == nil {
		odb = NewOrgDb()
		odb.RebuildDb()
	}
	return odb
}
