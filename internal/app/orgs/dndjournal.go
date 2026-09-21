//lint:file-ignore ST1006 allow the use of self
package orgs

// ----------------------------------------------------------------------------
// The undo journal
//
// undo.go in the engine works the last change out of the character's own
// history tables, which is exact, survives a restart and works on a file
// somebody else changed. It cannot see anything that leaves no history -
// taking an inventory line off the sheet as a correction, rewriting a note,
// throwing a roll away - because for those there is deliberately nothing
// written down.
//
// So every write also leaves a copy of the file as it stood behind it. The
// journal is memory only and goes when the server does, which is the right
// shape for an undo buffer: it is a convenience for the last few minutes of
// play, not a record. When it has nothing to say, undo falls back to reading
// the history, and between the two there is very little the button cannot
// take back.
//
// The one rule that matters: an entry is only good while the file is still
// exactly as it was left. Anything else - another window, the command line,
// a text editor - and putting the old text back would throw that away, so
// the entry is dropped instead.
// ----------------------------------------------------------------------------

import (
	"os"
	"sync"
	"time"
)

// dndJournalMax is how far back undo can reach. Deep enough to cover a
// fumbled minute at the table, shallow enough that a long session does not
// sit on a pile of old copies of every file.
const dndJournalMax = 30

// dndChange is one file as it stood before a change, and as it stood after.
type dndChange struct {
	Path   string
	Before string
	After  string
	What   string
	When   time.Time
}

var (
	dndJournalLock sync.Mutex
	dndJournal     []dndChange
	// dndQuiet suppresses the journal while undo is putting a file back. An
	// undo is not a change to be taken back, it is a change going away.
	dndQuiet bool
)

// dndRemember files one change away. A write that changed nothing is not a
// change, and is not remembered.
func dndRemember(path, before, after, what string) {
	if before == after {
		return
	}
	dndJournalLock.Lock()
	defer dndJournalLock.Unlock()
	if dndQuiet {
		return
	}
	dndJournal = append(dndJournal, dndChange{
		Path: path, Before: before, After: after, What: what, When: time.Now(),
	})
	if len(dndJournal) > dndJournalMax {
		dndJournal = dndJournal[len(dndJournal)-dndJournalMax:]
	}
}

// dndLastChange is the newest entry whose file is still exactly as the
// journal left it. Anything that has been changed since is dropped on the
// way past: it cannot be put back without throwing away whatever did the
// changing, and quietly doing that would be worse than not offering.
func dndLastChange() (dndChange, bool) {
	dndJournalLock.Lock()
	defer dndJournalLock.Unlock()
	for len(dndJournal) > 0 {
		ch := dndJournal[len(dndJournal)-1]
		data, err := os.ReadFile(ch.Path)
		if err == nil && string(data) == ch.After {
			return ch, true
		}
		dndJournal = dndJournal[:len(dndJournal)-1]
	}
	return dndChange{}, false
}

// dndDropChange takes the newest entry off, which is what undoing it does.
func dndDropChange() {
	dndJournalLock.Lock()
	defer dndJournalLock.Unlock()
	if len(dndJournal) > 0 {
		dndJournal = dndJournal[:len(dndJournal)-1]
	}
}

// dndPutBack restores one file and tells the database about it, without
// filing the restore itself away as something else to undo.
func dndPutBack(ch dndChange) error {
	dndJournalLock.Lock()
	dndQuiet = true
	dndJournalLock.Unlock()
	defer func() {
		dndJournalLock.Lock()
		dndQuiet = false
		dndJournalLock.Unlock()
	}()
	if err := os.WriteFile(ch.Path, []byte(ch.Before), 0644); err != nil {
		return err
	}
	GetDb().ReloadFile(ch.Path)
	return nil
}

// dndForgetFile drops every entry for one file. Nothing calls this yet; it
// is here for the day something deletes a character sheet outright, when
// offering to put half of it back would be worse than offering nothing.
func dndForgetFile(path string) {
	dndJournalLock.Lock()
	defer dndJournalLock.Unlock()
	keep := dndJournal[:0]
	for _, ch := range dndJournal {
		if ch.Path != path {
			keep = append(keep, ch)
		}
	}
	dndJournal = keep
}
