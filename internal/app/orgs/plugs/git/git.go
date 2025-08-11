package git

// SETUP
// Will use the first directory in your OrgDirs if a directory is not provided
// Can only sync one dir.
// plugins:
//   - name: git
//     freq: 300
//     gitpath: "C:/Program Files/Git/bin/git.exe"

/* SDOC: Pollers

* Git
  Syncs the first directory found in your OrgDirs with git automatically
  Can currently only sync one dir.

	#+BEGIN_SRC yaml
  - name: "git"
    freq: 300
    gitpath: "C:/Program Files/Git/bin/git.exe"
	#+END_SRC

EDOC */

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os/exec"
	"path/filepath"

	"github.com/ihdavids/orgs/internal/common"
)

type Git struct {
	Name        string
	GitPath     string
	OrgsSyncDir string
	ok          bool
}

func (self *Git) haveChanges() bool {
	cmd := exec.Command("%s diff --exit-code", self.GitPath)
	if err := cmd.Run(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			if exitError.ExitCode() > 0 {
				return true
			}
		}
	}
	return false
}

func countLines(r io.Reader) (int, error) {
	var count int
	var read int
	var err error
	var target []byte = []byte("\n")

	buffer := make([]byte, 32*1024)

	for {
		read, err = r.Read(buffer)
		if err != nil {
			break
		}

		count += bytes.Count(buffer[:read], target)
	}

	if err == io.EOF {
		return count, nil
	}

	return count, err
}

func (self *Git) localChanges() bool {
	// git ls-files --other --exclude-standard --directory | wc -l
	cmd := exec.Command(self.GitPath, "ls-files", "--other", "--exclude-standard", "--directory")
	if out, err := cmd.Output(); err == nil {
		if lc, e := countLines(bytes.NewReader(out)); e != nil {
			return lc > 0
		}
	}
	return false
}

func (self *Git) gitAddAll() bool {
	cmd := exec.Command(self.GitPath, "add", "--all")
	cmd.Dir = self.OrgsSyncDir
	var err error = nil
	if err = cmd.Run(); err == nil {
		return true
	} else {
		fmt.Printf("Git Add All Failed: %s\n", err)
	}
	return false
}

func (self *Git) gitCommit(msg string) bool {
	cmd := exec.Command(self.GitPath, "commit", "-m", fmt.Sprintf("\"%s\"", msg))
	cmd.Dir = self.OrgsSyncDir
	var err error = nil
	if err = cmd.Run(); err == nil {
		return true
	} else {
		fmt.Printf("Git commit error: %s\n", err)
	}
	return false
}

func (self *Git) gitPull() bool {
	cmd := exec.Command(self.GitPath, "pull", "--no-edit")
	cmd.Dir = self.OrgsSyncDir
	var err error = nil
	if err = cmd.Run(); err == nil {
		return true
	} else {
		fmt.Printf("Git pull error: %v\n", err)
	}
	return false
}

func (self *Git) gitPush() bool {
	cmd := exec.Command(self.GitPath, "push")
	cmd.Dir = self.OrgsSyncDir
	if err := cmd.Run(); err == nil {
		return true
	}
	return false
}

func (self *Git) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *Git) Update(db common.ODb) {
	if self.ok {
		fmt.Printf("Git Update...\n")
		modified := self.haveChanges()
		localChanges := self.localChanges()
		if modified || localChanges {
			fmt.Printf("Git found local changes, checking in...\n")
			self.gitAddAll()
			self.gitCommit("auto checkin from orgs")
		}
		fmt.Printf("Git pull...\n")
		self.gitPull()
		if modified || localChanges {
			fmt.Printf("Git have changes pushing...\n")
			self.gitPush()
		}
	} else {
		fmt.Printf("Git update skipped, not okay... is path setup correctly?\n")
	}
}

func (self *Git) Startup(freq int, manager *common.PluginManager, opts *common.PluginOpts) {
	gitPath, err := exec.LookPath(self.GitPath)
	if err != nil {
		log.Printf("Failed to find git during git startup! ABORT")
		log.Fatal(err)
		self.ok = false
	} else {
		self.GitPath = filepath.FromSlash(gitPath)
		fmt.Printf("Git module okay: %s\n", self.GitPath)
		if self.OrgsSyncDir == "" {
			self.OrgsSyncDir = manager.OrgDirs[0]
			self.OrgsSyncDir = filepath.FromSlash(self.OrgsSyncDir)
			fmt.Printf("Git sync dir set to: %s\n", self.OrgsSyncDir)
		} else {
			self.OrgsSyncDir = filepath.FromSlash(self.OrgsSyncDir)
		}
		self.ok = true
	}
}

// init function is called at boot
func init() {
	common.AddPoller("git", func() common.Poller {
		return &Git{GitPath: "git"}
	})
}
