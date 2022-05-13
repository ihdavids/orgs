package orgs

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
	"github.com/niklasfasching/go-org/org"
)

type OrgFile struct {
	filename string
	doc      *org.Document
}

type OrgDb struct {
	ByFile    map[string]OrgFile
	Filenames []string

	watcher     *fsnotify.Watcher
	watcherdone chan bool
}

func NewOrgDb() *OrgDb {
	var db *OrgDb = new(OrgDb)
	db.ByFile = make(map[string]OrgFile)
	return db
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
		ofile := OrgFile{
			filename: filename,
			doc:      d,
		}
		self.ByFile[filename] = ofile
		self.Filenames = append(self.Filenames, filename)
	} else {
		fmt.Println("Failed to parse file {}", filename)
	}
}

func (self *OrgDb) Close() {
	self.watcher.Close()
}

func (self *OrgDb) Watch() {
	var err error
	self.watcher, err = fsnotify.NewWatcher()
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
				log.Printf("%s %s\n", event.Name, event.Op)
			case err, ok := <-self.watcher.Errors:
				if !ok {
					return
				}
				log.Println("error:", err)
			}
		}

	}()

	// TODO: Make this our config directories
	err = self.watcher.Add("./")
	if err != nil {
		log.Fatal("Add failed:", err)
	}
}

func (self *OrgDb) RebuildDb() {
	var dirs []string = Conf().OrgDirs
	for _, dir := range dirs {
		files := self.ListFilesInDir(dir)
		for _, file := range files {
			self.LoadFile(file)
		}
	}
}

func (self *OrgDb) GetFiles() []string {
	return self.Filenames
}
