package dnd

/* SDOC: DnD
* Timeline Annotations

  The timeline works the shape of an evening out of the two logs and writes
  nothing back, which is what keeps it right after a session has been edited
  somewhere else. But a block it has found is worth naming: the flurry of
  rolls at 19:32 was *the ambush at the ford*, and the room it happened in is
  worth a line that the dice do not record.

  So an annotation is stored, and only the annotation. It is one entry under a
  =* Timeline= heading, stamped with the clock time of the block it belongs to
  and tagged with what kind of block that is:

  #+BEGIN_SRC org
  ,* Timeline
  ,** 19:32 Ambush at the ford                                        :fight:
     They came out of the reeds on both sides. Kael went down in the first
     round and we never did find the fourth one.
  ,** 21:10 The library                                                :note:
     Three floors of it, and every book about the same war.
  #+END_SRC

  Nothing about the block itself is written down - not how many rolls it held,
  not how long it ran - because all of that is worked out from the logs and
  would go stale the moment a roll was corrected. The annotation carries the
  anchor and the prose, and the timeline hangs it on whichever block that
  time now falls in.

  An annotation whose time no longer lands in any block is not thrown away. It
  is drawn as a beat of its own at the time it carries, so a note about the
  room survives the rolls that happened to be in it being deleted - and so
  that a moment nobody rolled for can be written down in the first place.
EDOC */

import (
	"fmt"
	"regexp"
	"strings"
)

// SessionTimelineHeading is the section annotations are kept under.
const SessionTimelineHeading = "Timeline"

// SessionMark is one annotation hung on the timeline.
//
// Time is the clock the block it belongs to starts at, and Kind is what sort
// of block that is - "fight", "note", "roll", "rolls" - written as an org tag
// on the entry. Together they are the anchor. Everything else about the block
// is derived and deliberately not stored here.
type SessionMark struct {
	Time  string `json:"time"`
	Kind  string `json:"kind"`
	Title string `json:"title"`
	Note  string `json:"note"`
}

// SessionRef names one entry of a session's logs by its position, along with
// the reading the page had of it. It is what a caller deleting several
// entries at once sends, and Was is checked exactly as the single deletes
// check it: a page holding stale numbers must not throw away the wrong line.
type SessionRef struct {
	Index int    `json:"index"`
	Was   string `json:"was"`
}

// reMarkHead reads the anchor and title back off an annotation's heading. The
// time leads, the tag trails, and whatever is between them is the title -
// which may be nothing at all, for a block that has prose and no name.
var reMarkHead = regexp.MustCompile(`^(\d{1,2}:\d{2})\s*(.*?)\s*(?::([A-Za-z][A-Za-z0-9_-]*):)?$`)

// reMarkTag matches the tag-shaped tail of a title, which cannot be written
// as-is: it would read back as the entry's kind. See markTitle.
var reMarkTag = regexp.MustCompile(`:[A-Za-z][A-Za-z0-9_-]*:$`)

// markTitle makes a title safe to sit in the heading. Runs of whitespace are
// collapsed the way they are in a table cell, and a title that happens to end
// in something tag-shaped loses its final colon - otherwise reading the entry
// back would take "the :trap:" for an entry of kind "trap". A rare loss of one
// character, and a deterministic one, which is what keeps the round trip
// honest.
func markTitle(title string) string {
	title = strings.Join(strings.Fields(title), " ")
	if reMarkTag.MatchString(title) {
		title = strings.TrimSuffix(title, ":")
	}
	return title
}

// markKind normalises the tag. An empty kind is allowed and means the mark is
// anchored on a time rather than on a particular sort of block.
func markKind(kind string) string {
	kind = strings.ToLower(strings.TrimSpace(kind))
	return slugStrip.ReplaceAllString(kind, "")
}

// markStamp normalises the anchor time to HH:MM.
func markStamp(at string) string {
	m := regexp.MustCompile(`^(\d{1,2}):(\d{2})`).FindStringSubmatch(strings.TrimSpace(at))
	if m == nil {
		return ""
	}
	h := m[1]
	if len(h) == 1 {
		h = "0" + h
	}
	return h + ":" + m[2]
}

// ParseMarks reads the annotations back out of a session file.
func ParseMarks(text string) []SessionMark {
	lines := splitLines(text)
	start, end := findSection(lines, SessionTimelineHeading)
	out := []SessionMark{}
	if start < 0 {
		return out
	}
	var cur *SessionMark
	body := []string{}
	flush := func() {
		if cur == nil {
			return
		}
		cur.Note = strings.TrimRight(strings.Join(orgStripBreaks(body), "\n"), "\n")
		out = append(out, *cur)
		cur = nil
		body = nil
	}
	for i := start; i < end; i++ {
		line := lines[i]
		if lvl, title := headingOf(line); lvl == 2 {
			flush()
			m := reMarkHead.FindStringSubmatch(strings.TrimSpace(title))
			if m == nil {
				// Not an annotation. Anything else written in this section is
				// somebody's own, and is left alone rather than read wrongly.
				continue
			}
			cur = &SessionMark{Time: m[1], Title: m[2], Kind: strings.ToLower(m[3])}
			continue
		} else if lvl > 2 {
			body = append(body, strings.Repeat("*", lvl-2)+" "+title)
			continue
		}
		if cur != nil {
			body = append(body, unindent(line))
		}
	}
	flush()
	return out
}

// markHeading renders one annotation's heading.
func markHeading(m SessionMark) string {
	head := "** " + m.Time
	if m.Title != "" {
		head += " " + m.Title
	}
	if m.Kind != "" {
		head += " :" + m.Kind + ":"
	}
	return head
}

// SetMark writes an annotation, replacing the one already on that anchor.
//
// The anchor is the time and the kind together, so naming a fight twice
// rewrites the name rather than leaving two of them. A new annotation is
// appended to the end of the section the way notes and rolls are appended to
// theirs: the order entries sit in the file is the order they were written,
// and the timeline sorts by the clock anyway.
//
// A mark with neither a title nor a note is refused. Rubbing an annotation
// out is deleting it, which is DeleteMark.
func SetMark(text string, m SessionMark) (string, error) {
	m.Time = markStamp(m.Time)
	if m.Time == "" {
		return text, fmt.Errorf("an annotation needs the time of the block it belongs to")
	}
	m.Kind = markKind(m.Kind)
	m.Title = markTitle(m.Title)
	if strings.TrimSpace(m.Title) == "" && strings.TrimSpace(m.Note) == "" {
		return text, fmt.Errorf("an annotation needs a name or something to say")
	}
	block := append([]string{markHeading(m)}, renderNoteBody(m.Note)...)
	block = trimTrailingBlanks(block)
	block = append(block, "")

	lines := splitLines(text)
	start, end := findSection(lines, SessionTimelineHeading)
	if start < 0 {
		return insertTimelineSection(lines, block), nil
	}
	if at, stop, ok := findMarkEntry(lines, start, end, m.Time, m.Kind); ok {
		out := append([]string{}, lines[:at]...)
		out = append(out, block...)
		out = append(out, lines[stop:]...)
		return strings.Join(out, "\n"), nil
	}
	body := trimTrailingBlanks(lines[start:end])
	out := append([]string{}, lines[:start]...)
	out = append(out, body...)
	out = append(out, block...)
	out = append(out, lines[end:]...)
	return strings.Join(out, "\n"), nil
}

// DeleteMark takes one annotation off the timeline. An anchor that carries no
// annotation is refused rather than quietly doing nothing, so a page working
// from a stale reading hears about it.
func DeleteMark(text, at, kind string) (string, error) {
	at, kind = markStamp(at), markKind(kind)
	if at == "" {
		return text, fmt.Errorf("which annotation? pass the time it is anchored on")
	}
	lines := splitLines(text)
	start, end := findSection(lines, SessionTimelineHeading)
	if start < 0 {
		return text, fmt.Errorf("this session has no annotations")
	}
	from, stop, ok := findMarkEntry(lines, start, end, at, kind)
	if !ok {
		return text, fmt.Errorf("there is no annotation on %s", at)
	}
	out := append([]string{}, lines[:from]...)
	out = append(out, lines[stop:]...)
	return strings.Join(dropEmptyTimeline(out), "\n"), nil
}

// dropEmptyTimeline takes the section away when the last annotation in it
// goes, the way the roll table goes when its last row does: a "* Timeline"
// with nothing under it reads as something somebody emptied rather than
// something nothing has been written to yet.
//
// A section holding anything else - somebody's own heading, a line of prose -
// is left exactly where it is. Only the annotations here are this code's to
// take away.
func dropEmptyTimeline(lines []string) []string {
	for i, l := range lines {
		lvl, title := headingOf(l)
		if lvl != 1 || !strings.EqualFold(stripTags(title), SessionTimelineHeading) {
			continue
		}
		j := i + 1
		for j < len(lines) {
			if l2, _ := headingOf(lines[j]); l2 > 0 && l2 <= lvl {
				break
			}
			if strings.TrimSpace(lines[j]) != "" {
				return lines
			}
			j++
		}
		out := append([]string{}, lines[:i]...)
		// The blank line that held the sections either side of it apart is
		// part of how the file reads, not part of the section.
		if i > 0 && j < len(lines) && strings.TrimSpace(lines[i-1]) != "" {
			out = append(out, "")
		}
		return append(out, lines[j:]...)
	}
	return lines
}

// DropMark is DeleteMark for a caller that does not mind either way - taking
// a block off the timeline takes its annotation with it, and a block that
// never had one is not a problem worth refusing over.
func DropMark(text, at, kind string) string {
	out, err := DeleteMark(text, at, kind)
	if err != nil {
		return text
	}
	return out
}

// findMarkEntry locates the annotation on one anchor and returns the half open
// line range it occupies.
func findMarkEntry(lines []string, start, end int, at, kind string) (int, int, bool) {
	heads := []int{}
	for i := start; i < end; i++ {
		if lvl, _ := headingOf(lines[i]); lvl == 2 {
			heads = append(heads, i)
		}
	}
	for n, i := range heads {
		_, title := headingOf(lines[i])
		m := reMarkHead.FindStringSubmatch(strings.TrimSpace(title))
		if m == nil || markStamp(m[1]) != at || strings.ToLower(m[3]) != kind {
			continue
		}
		stop := end
		if n+1 < len(heads) {
			stop = heads[n+1]
		}
		return i, stop, true
	}
	return -1, -1, false
}

// insertTimelineSection writes the section a file does not have yet. It goes
// above the notes, so the file reads as who played, the shape of the evening,
// what was written down, and what was rolled.
func insertTimelineSection(lines []string, block []string) string {
	at := -1
	for i, l := range lines {
		lvl, title := headingOf(l)
		if lvl != 1 {
			continue
		}
		if strings.EqualFold(stripTags(title), SessionNotesHeading) ||
			strings.EqualFold(stripTags(title), SessionRollsHeading) {
			at = i
			break
		}
	}
	section := append([]string{"* " + SessionTimelineHeading}, block...)
	if at < 0 {
		out := trimTrailingBlanks(lines)
		if len(out) > 0 {
			out = append(out, "")
		}
		return strings.Join(append(out, section...), "\n")
	}
	out := append([]string{}, lines[:at]...)
	out = append(out, section...)
	out = append(out, lines[at:]...)
	return strings.Join(out, "\n")
}

// ----------------------------------------------------------------------------
// Taking a whole block off the timeline
// ----------------------------------------------------------------------------

// DeleteEntries takes several rolls and notes out of a session in one go.
//
// It exists because a block on the timeline is not one line of the file: a
// combat block is a run of rolls with the notes written during it hanging off
// it, and throwing the block away means throwing all of them away together.
// Doing that one call at a time would leave a half-deleted fight behind the
// moment one of them was refused, and would fill the undo journal with a
// dozen entries for what the table thinks of as one act.
//
// Entries are removed from the back forwards, so the shifting that deleting
// causes never reaches an index that has not been used yet. Each one is found
// and checked exactly as the single deletes find and check it - a refusal
// anywhere refuses the lot, and the text handed in is returned unchanged.
func DeleteEntries(text string, rolls, notes []SessionRef) (string, error) {
	out := text
	for _, ref := range descending(rolls) {
		next, err := DeleteRoll(out, ref.Index, ref.Was)
		if err != nil {
			return text, err
		}
		out = next
	}
	for _, ref := range descending(notes) {
		next, err := DeleteNote(out, ref.Index, ref.Was)
		if err != nil {
			return text, err
		}
		out = next
	}
	if out == text {
		return text, fmt.Errorf("nothing was named to delete")
	}
	return out, nil
}

// descending sorts references back to front and drops repeats, so that the
// same line named twice is not deleted twice - which would take an innocent
// neighbour with it.
func descending(refs []SessionRef) []SessionRef {
	out := []SessionRef{}
	seen := map[int]bool{}
	for _, r := range refs {
		if r.Index < 0 || seen[r.Index] {
			continue
		}
		seen[r.Index] = true
		out = append(out, r)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Index > out[j-1].Index; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
