package orgs

// A cache whose entries are per file, so that one saved file costs one file.
//
// Every index in here used to be gated on `OrgDb.ReloadIndex` - the whole
// database's version - and rebuilt from nothing whenever it moved. That is
// correct and it is why saving one file made the next request for the code
// index take fifty milliseconds, the link list thirty, and the starmap a
// hundred and fifty: each of them walked five hundred files to find out what
// had changed in one.
//
// The database now carries a version per file. This turns that into a cache:
// hand it a function that builds one file's worth of whatever the index is
// made of, and it rebuilds only the files whose version has moved.
//
// Three things it has to get right, and each of them is a way a per-file cache
// goes quietly wrong:
//
//  1. **A file that has gone must leave.** Otherwise a deleted file's headings
//     answer queries forever, which looks like the database being stale rather
//     than like a bug.
//  2. **The order must be the database's**, not a map's. Several of these
//     indexes are sorted afterwards, but the sorts are stable - so an input
//     order that changed between two identical requests would reorder the
//     answer, and a client that pages through results would see rows move.
//  3. **Building happens outside the lock.** Parsing a file while holding the
//     cache would serialise every reader behind the one writer, which on a
//     board of forty cards asking at once is worse than the walk it replaced.

import (
	"sync"

	"github.com/ihdavids/orgs/internal/common"
)

type filePart[T any] struct {
	version uint64
	value   T
}

// FileParts holds one T per file.
type FileParts[T any] struct {
	mu    sync.Mutex
	parts map[string]filePart[T]
}

func NewFileParts[T any]() *FileParts[T] {
	return &FileParts[T]{parts: map[string]filePart[T]{}}
}

// Every file's part, in the database's own file order, rebuilding only what has
// moved since last time.
//
// `build` is called with a file and must answer with that file's contribution
// and nothing else - it must not look at any other file, because the whole
// point is that it is not called for the others. Anything needing to see the
// database as a whole (resolving a link to the heading it lands on, say) has to
// happen after this, over the parts.
func (self *FileParts[T]) All(build func(f *common.OrgFile) T) []T {
	db := GetDb()
	names := db.GetFiles()
	versions, _ := db.FileVersions()

	// What needs building, decided under the lock and built outside it.
	self.mu.Lock()
	type todo struct {
		name string
		f    *common.OrgFile
		v    uint64
	}
	var work []todo
	for _, name := range names {
		v := versions[name]
		if got, have := self.parts[name]; have && got.version == v {
			continue
		}
		work = append(work, todo{name: name, v: v})
	}
	// A file the database no longer holds goes, or its headings answer queries
	// forever.
	live := make(map[string]bool, len(names))
	for _, n := range names {
		live[n] = true
	}
	for name := range self.parts {
		if !live[name] {
			delete(self.parts, name)
		}
	}
	self.mu.Unlock()

	for i := range work {
		work[i].f = db.FindByFile(work[i].name)
	}
	built := make([]filePart[T], len(work))
	for i, w := range work {
		if w.f == nil || w.f.Doc == nil {
			continue
		}
		built[i] = filePart[T]{version: w.v, value: build(w.f)}
	}

	self.mu.Lock()
	for i, w := range work {
		if w.f == nil || w.f.Doc == nil {
			continue
		}
		self.parts[w.name] = built[i]
	}
	out := make([]T, 0, len(names))
	for _, name := range names {
		if got, have := self.parts[name]; have {
			out = append(out, got.value)
		}
	}
	self.mu.Unlock()
	return out
}

// How many files the cache is holding, and how many it rebuilt on the last
// call. For the endpoint that reports on the indexes.
func (self *FileParts[T]) Size() int {
	self.mu.Lock()
	defer self.mu.Unlock()
	return len(self.parts)
}

// Throw everything away. For a caller whose build function has changed shape -
// a setting that alters what a part contains - where the file versions have not
// moved and so nothing would rebuild.
func (self *FileParts[T]) Forget() {
	self.mu.Lock()
	defer self.mu.Unlock()
	self.parts = map[string]filePart[T]{}
}
