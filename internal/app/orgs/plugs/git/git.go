package git

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os/exec"

	"github.com/ihdavids/orgs/internal/app/orgs/plugs"
)

type Git struct {
	Name    string
	GitPath string
	ok      bool
}

func (self *Git) haveChanges() bool {
	cmd := exec.Command("git diff --exit-code")
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
	cmd := exec.Command("git ls-files --other --exclude-standard --directory")
	if out, err := cmd.Output(); err == nil {
		if lc, e := countLines(bytes.NewReader(out)); e != nil {
			return lc > 0
		}
	}
	return false
}

func (self *Git) gitAddAll() bool {
	cmd := exec.Command("git add --all")
	if err := cmd.Run(); err == nil {
		return true
	}
	return false
}

func (self *Git) gitCommit(msg string) bool {
	cmd := exec.Command(fmt.Sprintf("git commit -m \"%s\"", msg))
	if err := cmd.Run(); err == nil {
		return true
	}
	return false
}

func (self *Git) gitPull() bool {
	cmd := exec.Command(fmt.Sprintf("git pull --no-edit"))
	var err error = nil
	if err = cmd.Run(); err == nil {
		return true
	}
	fmt.Printf("Error: %v\n", err)
	return false
}

func (self *Git) gitPush() bool {
	cmd := exec.Command(fmt.Sprintf("git push"))
	if err := cmd.Run(); err == nil {
		return true
	}
	return false
}

func (self *Git) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *Git) Update(db plugs.ODb) {
	fmt.Printf("Git Update...\n")
	if self.ok {
		modified := self.haveChanges()
		localChanges := self.localChanges()
		if modified || localChanges {
			self.gitAddAll()
			self.gitCommit("auto checkin from orgs")
		}
		fmt.Printf("Git pull...\n")
		self.gitPull()
		if modified || localChanges {
			self.gitPush()
		}
	} else {
		fmt.Printf("Git not okay...\n")
	}
}

func (self *Git) Startup(freq int, manager *plugs.PluginManager, opts *plugs.PluginOpts) {
	_, err := exec.LookPath(self.GitPath)
	if err != nil {
		log.Printf("Failed to find git during git startup! ABORT")
		log.Fatal(err)
		self.ok = false
	} else {
		self.ok = true
	}
}

// init function is called at boot
func init() {
	plugs.AddPoller("git", func() plugs.Poller {
		return &Git{GitPath: "git"}
	})
}
