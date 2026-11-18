package orgs

// A hash a client holds keeps finding its heading after the file is edited.
//
// A heading's hash is one running sha1 over the file's path and every title
// before it in the file (go-org's headline.go), so renaming, adding or removing
// a heading changes the hash of every heading after it (**Traps: hashes**). A
// client that read the outline before that - worg's open file, a VS Code
// panel, the `orgs search | orgs tag` pipe, a request that writes twice -
// then asks for a hash nobody has any more, and gets "no heading with that
// hash" for a heading that is still there.
//
// So when a file is read again, its old headings are matched to its new ones
// and every hash that changed is remembered, old to new. FindByHash follows
// that when a hash is not in the index. The hash itself is unchanged, so
// worg's ClientHash and every saved hash keep working.
//
// Matching is deliberately narrow, because forwarding to the wrong heading is
// worse than not forwarding at all: the request would write into it. A heading
// is matched only when the match cannot be anything else -
//
//   - by its :ID:, when exactly one heading in the new file carries it;
//   - else among its siblings, by title, when the title is unique among the
//     siblings on both sides;
//   - else renamed in place: the same row and level, when neither title occurs
//     among the siblings on the other side.
//
// Children are matched under a matched parent, so renaming a heading keeps
// its whole subtree reachable. Two siblings with one title, a heading moved
// to another parent without an ID, a heading deleted: none of those is
// forwarded, and the lookup misses as it always did.

import (
	"strings"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// More remembered moves than this and the table starts again. A server would
// need thousands of edits without a restart to get there, and losing the table
// costs only what not having it costs.
const maxMovedHashes = 20000

// How many reloads a hash is followed through. Each reload of a file adds at
// most one step to a heading's chain.
const maxHashHops = 64

// hashMoves is old hash -> new hash for each heading of the old parse that is
// unambiguously a heading of the new one and whose hash changed.
func hashMoves(old, nu []*org.Section) map[string]string {
	out := map[string]string{}
	idCount := map[string]int{}
	byId := map[string]*org.Section{}
	var walk func(secs []*org.Section)
	walk = func(secs []*org.Section) {
		for _, s := range secs {
			if s == nil || s.Headline == nil {
				continue
			}
			if id := strings.TrimSpace(GetProp(s, "ID", "Id", "id")); id != "" {
				idCount[id]++
				byId[id] = s
			}
			walk(s.Children)
		}
	}
	walk(nu)
	claimed := map[*org.Section]bool{}
	matchSections(old, nu, idCount, byId, claimed, out)
	return out
}

func matchSections(old, nu []*org.Section, idCount map[string]int, byId map[string]*org.Section, claimed map[*org.Section]bool, out map[string]string) {
	oldTitles := titleCounts(old)
	newTitles := titleCounts(nu)
	for _, o := range old {
		if o == nil || o.Headline == nil {
			continue
		}
		var m *org.Section
		if id := strings.TrimSpace(GetProp(o, "ID", "Id", "id")); id != "" && idCount[id] == 1 && !claimed[byId[id]] {
			m = byId[id]
		}
		t := sectionTitle(o)
		if m == nil && oldTitles[t] == 1 && newTitles[t] == 1 {
			for _, n := range nu {
				if n != nil && n.Headline != nil && !claimed[n] && sectionTitle(n) == t {
					m = n
					break
				}
			}
		}
		if m == nil && newTitles[t] == 0 {
			for _, n := range nu {
				if n == nil || n.Headline == nil || claimed[n] ||
					n.Headline.Pos.Row != o.Headline.Pos.Row || n.Headline.Lvl != o.Headline.Lvl {
					continue
				}
				if oldTitles[sectionTitle(n)] == 0 {
					m = n
				}
				break
			}
		}
		if m == nil {
			continue
		}
		claimed[m] = true
		if o.Hash != "" && m.Hash != "" && o.Hash != m.Hash {
			out[o.Hash] = m.Hash
		}
		matchSections(o.Children, m.Children, idCount, byId, claimed, out)
	}
}

func titleCounts(secs []*org.Section) map[string]int {
	out := map[string]int{}
	for _, s := range secs {
		if s != nil && s.Headline != nil {
			out[sectionTitle(s)]++
		}
	}
	return out
}

// noteMovedHashes remembers the hashes a re-read of a file changed. Called with
// the write lock held, before the old parse is forgotten.
func (self *OrgDb) noteMovedHashes(old, nu *common.OrgFile) {
	if old == nil || old.Doc == nil || old.Doc.Outline.Section == nil ||
		nu == nil || nu.Doc == nil || nu.Doc.Outline.Section == nil {
		return
	}
	moves := hashMoves(old.Doc.Outline.Section.Children, nu.Doc.Outline.Section.Children)
	if len(moves) == 0 {
		return
	}
	if self.movedHash == nil || len(self.movedHash)+len(moves) > maxMovedHashes {
		self.movedHash = map[string]string{}
	}
	for from, to := range moves {
		self.movedHash[from] = to
	}
}

// movedTo is the section a hash from an older parse now names, or nil.
// Called with the read lock held.
func (self *OrgDb) movedTo(hash string) *org.Section {
	for hop := 0; hop < maxHashHops; hop++ {
		next, ok := self.movedHash[hash]
		if !ok {
			return nil
		}
		if v, ok := self.ByHash[next]; ok {
			return v
		}
		hash = next
	}
	return nil
}
