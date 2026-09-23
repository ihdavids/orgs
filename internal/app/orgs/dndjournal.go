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
// Seq numbers the entry so that one taken out of the middle - which is what a
// per-file undo does - can be found again without counting from either end.
type dndChange struct {
	Seq    uint64
	Path   string
	Before string
	After  string
	What   string
	When   time.Time
}

var (
	dndJournalLock sync.Mutex
	dndJournal     []dndChange
	dndJournalSeq  uint64
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
	dndJournalSeq++
	dndJournal = append(dndJournal, dndChange{
		Seq: dndJournalSeq, Path: path, Before: before, After: after,
		What: what, When: time.Now(),
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

// dndLastChangeTo is dndLastChange narrowed to one file, which is what the
// timeline's own undo asks: the session drawer is looking at one evening and
// should offer to take back the last thing that happened to it, not the last
// thing that happened anywhere.
//
// Only the newest entry for that file is ever offered. An older one could not
// be put back without throwing away the newer changes stacked on top of it,
// and the same guard applies as everywhere else here - the file must still be
// exactly as the entry left it. When it is not, every entry for that file is
// dropped: an older one cannot match a file a newer one does not.
func dndLastChangeTo(path string) (dndChange, bool) {
	dndJournalLock.Lock()
	defer dndJournalLock.Unlock()
	for i := len(dndJournal) - 1; i >= 0; i-- {
		if dndJournal[i].Path != path {
			continue
		}
		data, err := os.ReadFile(path)
		if err == nil && string(data) == dndJournal[i].After {
			return dndJournal[i], true
		}
		keep := dndJournal[:0]
		for _, ch := range dndJournal {
			if ch.Path != path {
				keep = append(keep, ch)
			}
		}
		dndJournal = keep
		return dndChange{}, false
	}
	return dndChange{}, false
}

// dndDropSeq takes one named entry off wherever it sits, which is what
// undoing a per-file change does.
func dndDropSeq(seq uint64) {
	dndJournalLock.Lock()
	defer dndJournalLock.Unlock()
	keep := dndJournal[:0]
	for _, ch := range dndJournal {
		if ch.Seq != seq {
			keep = append(keep, ch)
		}
	}
	dndJournal = keep
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
