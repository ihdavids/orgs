package orgs

// Importing contacts from a vCard file (Apple Contacts, Google Contacts,
// Outlook) into the contact records.
//
// vcard.go reads the file; this decides, for each person in it, whether they
// are somebody the address book already has, and what to write. The rules:
//
//  1. A person is matched by the card's UID (kept on the record as VCARD_UID
//     once imported), then by any email address, then by name ignoring case.
//     The UID comes first because it is the only thing that survives a person
//     changing their name and their address at once.
//  2. A file that holds somebody twice is one person: the cards are folded
//     together before anything is matched.
//  3. merge (the default) adds the people who are new, and for the ones that
//     match fills in only what the record lacks. A record's value is never
//     changed. A second email or phone number is not a conflict - it is
//     filed beside the first (EMAIL_WORK_2) - but a different birthday or
//     company is, and is reported as kept rather than written. skip leaves
//     matched records alone; overwrite writes the card's values over them.
//     Nothing ever removes a field, renames a record, or replaces its notes:
//     a note is appended, and only when the record does not already say it.
//  4. Running the same import twice changes nothing the second time, which is
//     what makes it safe to re-export from the phone and import again.
//  5. Every write is a line edit through the record code (UpdateRecord, and
//     recordHeadingLines for the new ones) so the history in each record's
//     LOGBOOK says what came from the import.
//
// New people are written in one splice at the end of the collection's
// container rather than one AddRecord each: AddRecord re-reads the collections
// and the file per call, and an address book of five hundred people made that
// minutes rather than a second.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/ihdavids/orgs/internal/common"
)

func registerContactImportRoutes(api *mux.Router) {
	api.HandleFunc("/contacts/import", PostContactImport).Methods("POST")
}

// One import at a time: two running together would both decide that the same
// new person needs adding.
var contactImportMu sync.Mutex

// What the import says about itself in each record's history.
const vcardVia = "from vCard import"

// ----------------------------------------------------------------------------
// The plan
// ----------------------------------------------------------------------------

// What will be done for one person.
type contactPlan struct {
	item common.ContactImportItem
	c    *vcContact
	rec  *common.Record
	// The fields to write: every field for a new person, the missing or
	// overwritten ones for a match.
	set   map[string]string
	order []string
	tags  []string
	notes string
	// Save c.Photo and link it from IMAGE.
	photo bool
}

// Finding a person in the address book.
type contactIndex struct {
	byUid   map[string]*common.Record
	byEmail map[string]*common.Record
	byName  map[string]*common.Record
}

func indexContacts(recs []*common.Record) *contactIndex {
	ix := &contactIndex{
		byUid: map[string]*common.Record{}, byEmail: map[string]*common.Record{},
		byName: map[string]*common.Record{},
	}
	for _, r := range recs {
		for _, f := range r.Fields {
			switch {
			case strings.EqualFold(f.Key, vcUidField):
				if _, have := ix.byUid[f.Value]; !have && f.Value != "" {
					ix.byUid[f.Value] = r
				}
			case f.Kind == "email":
				if e := normValue("email", f.Value); e != "" {
					if _, have := ix.byEmail[e]; !have {
						ix.byEmail[e] = r
					}
				}
			}
		}
		if n := strings.ToLower(strings.TrimSpace(r.Name)); n != "" {
			if _, have := ix.byName[n]; !have {
				ix.byName[n] = r
			}
		}
	}
	return ix
}

func emailsOf(c *vcContact) []string {
	out := []string{}
	for _, k := range c.Keys {
		if compareKind(k) == "email" {
			if e := normValue("email", c.Fields[k]); e != "" {
				out = append(out, e)
			}
		}
	}
	return out
}

// match finds the record a contact is, and says how.
func (ix *contactIndex) match(c *vcContact) (*common.Record, string, string) {
	if c.Uid != "" {
		if r := ix.byUid[c.Uid]; r != nil {
			return r, "uid", "matched by vCard UID"
		}
	}
	for _, e := range emailsOf(c) {
		if r := ix.byEmail[e]; r != nil {
			return r, "email", "matched by email " + e
		}
	}
	if r := ix.byName[strings.ToLower(strings.TrimSpace(c.Name))]; r != nil {
		return r, "name", "matched by name"
	}
	return nil, "", ""
}

// foldDuplicates makes one contact of the cards that are the same person: the
// same UID, an email in common, or the same name.
func foldDuplicates(cs []*vcContact) ([]*vcContact, []int) {
	out := []*vcContact{}
	first := []int{}
	seen := map[string]int{}
	for i, c := range cs {
		keys := []string{}
		if c.Uid != "" {
			keys = append(keys, "uid:"+c.Uid)
		}
		for _, e := range emailsOf(c) {
			keys = append(keys, "email:"+e)
		}
		if n := strings.ToLower(strings.TrimSpace(c.Name)); n != "" {
			keys = append(keys, "name:"+n)
		}
		at := -1
		for _, k := range keys {
			if j, ok := seen[k]; ok {
				at = j
				break
			}
		}
		if at < 0 {
			at = len(out)
			out = append(out, c)
			first = append(first, i+1)
		} else {
			out[at].absorb(c)
		}
		for _, k := range keys {
			if _, ok := seen[k]; !ok {
				seen[k] = at
			}
		}
	}
	return out, first
}

// The kinds a person has several of, so a new one is filed beside the old
// rather than being a conflict with it.
var manyKinds = map[string]bool{"email": true, "tel": true, "url": true, "postal": true}

// planImport decides what to do with each person. Pure but for photoOf, which
// reads an existing record's picture so an overwrite of the same photo is not
// a change (nil reads nothing).
func planImport(contacts []*vcContact, existing []*common.Record, mode string, photoOf func(*common.Record) []byte) []*contactPlan {
	folded, cardNo := foldDuplicates(contacts)
	ix := indexContacts(existing)

	plans := []*contactPlan{}
	byRec := map[string]*contactPlan{}
	for i, c := range folded {
		p := &contactPlan{c: c, item: common.ContactImportItem{
			Card: cardNo[i], Cards: c.Cards, Name: c.Name, Warnings: c.Warnings,
		}}
		if strings.TrimSpace(c.Name) == "" {
			p.item.Action = "skip"
			p.item.Reason = "the card has no name, company or email to file it under"
			plans = append(plans, p)
			continue
		}
		rec, how, why := ix.match(c)
		if rec != nil {
			// Two people in the file that turn out to be one record are one
			// person: the second is folded into the first's plan.
			if earlier := byRec[rec.Hash]; earlier != nil {
				earlier.c.absorb(c)
				continue
			}
			p.rec = rec
			p.item.Match, p.item.Reason = how, why
			p.item.Hash, p.item.Filename, p.item.LineNum = rec.Hash, rec.Filename, rec.LineNum
			byRec[rec.Hash] = p
		}
		plans = append(plans, p)
	}

	for _, p := range plans {
		if p.item.Action != "" {
			continue
		}
		p.item.Name = p.c.Name
		p.item.Cards = p.c.Cards
		if p.rec == nil {
			planAdd(p)
		} else {
			planMatch(p, mode, photoOf)
		}
	}
	return plans
}

func planAdd(p *contactPlan) {
	c := p.c
	p.item.Action = "add"
	p.item.Reason = "new"
	p.set = map[string]string{}
	for _, k := range c.Keys {
		p.set[k] = c.Fields[k]
		p.order = append(p.order, k)
		p.item.Changes = append(p.item.Changes, common.ContactImportChange{Key: k, Value: c.Fields[k], How: "add"})
	}
	if c.Photo != nil {
		p.photo = true
		p.item.Changes = append(p.item.Changes, common.ContactImportChange{
			Key: "IMAGE", Value: "photo (" + c.Photo.Ext + ", " + photoSize(c.Photo) + ")", How: "add",
		})
	}
	p.tags = append(p.tags, c.Tags...)
	p.item.Tags = p.tags
	p.notes = c.Notes
	p.item.Notes = c.Notes != ""
}

func planMatch(p *contactPlan, mode string, photoOf func(*common.Record) []byte) {
	c, rec := p.c, p.rec
	if mode == "skip" {
		p.item.Action = "skip"
		p.item.Reason = "already there, " + p.item.Reason
		return
	}
	have := map[string]string{}
	for _, f := range rec.Fields {
		have[strings.ToUpper(f.Key)] = f.Value
	}
	p.set = map[string]string{}
	kept := 0
	for _, k := range c.Keys {
		v := c.Fields[k]
		if hasValue(have, k, v) {
			continue
		}
		old, taken := have[k]
		switch {
		case !taken:
			p.put(have, k, v, "", "add")
		case manyKinds[compareKind(k)] && hasValue(c.Fields, k, old):
			// The record's value is in the card too, under another name: it
			// is one of several addresses, and replacing it would lose it.
			p.put(have, freeKey(have, k), v, "", "add")
		case mode == "overwrite":
			p.put(have, k, v, old, "replace")
		case manyKinds[compareKind(k)]:
			// Another address, not a different answer to the same question.
			p.put(have, freeKey(have, k), v, "", "add")
		case k == vcUidField:
			// Matched some other way and carrying another UID: the record's
			// own stays, and nobody needs telling.
		default:
			kept++
			p.item.Changes = append(p.item.Changes, common.ContactImportChange{Key: k, Value: v, Old: old, How: "kept"})
		}
	}

	if c.Photo != nil {
		img := ""
		for k, v := range have {
			if compareKind(k) == "image" {
				img = v
				break
			}
		}
		desc := "photo (" + c.Photo.Ext + ", " + photoSize(c.Photo) + ")"
		switch {
		case img == "":
			p.photo = true
			p.item.Changes = append(p.item.Changes, common.ContactImportChange{Key: "IMAGE", Value: desc, How: "add"})
		case mode == "overwrite":
			var now []byte
			if photoOf != nil {
				now = photoOf(rec)
			}
			if !samePhoto(now, c.Photo.Data) {
				p.photo = true
				p.item.Changes = append(p.item.Changes, common.ContactImportChange{Key: "IMAGE", Value: desc, Old: img, How: "replace"})
			}
		}
	}

	for _, t := range c.Tags {
		if !vcHasTag(rec.Tags, t) && !vcHasTag(p.tags, t) {
			p.tags = append(p.tags, t)
		}
	}
	p.item.Tags = p.tags
	if c.Notes != "" && !notesContain(rec.Notes, c.Notes) {
		p.notes = c.Notes
		p.item.Notes = true
	}

	if len(p.set) == 0 && !p.photo && len(p.tags) == 0 && p.notes == "" {
		p.item.Action = "skip"
		if kept > 0 {
			p.item.Reason = fmt.Sprintf("nothing new (%d %s differently and %s kept), %s",
				kept, vcPlural(kept, "field says it", "fields say it"), vcPlural(kept, "is", "are"), p.item.Reason)
		} else {
			p.item.Reason = "nothing new, " + p.item.Reason
		}
		return
	}
	p.item.Action = "update"
}

func (p *contactPlan) put(have map[string]string, k, v, old, how string) {
	p.set[k] = v
	p.order = append(p.order, k)
	have[k] = v
	p.item.Changes = append(p.item.Changes, common.ContactImportChange{Key: k, Value: v, Old: old, How: how})
}

func vcPlural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// ----------------------------------------------------------------------------
// Doing it
// ----------------------------------------------------------------------------

func importMode(m string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(m)) {
	case "", "merge":
		return "merge", nil
	case "skip":
		return "skip", nil
	case "overwrite", "update", "replace":
		return "overwrite", nil
	}
	return "", fmt.Errorf("no import mode %q: merge, skip or overwrite", m)
}

// ImportContacts reads a vCard file into a collection, or on a dry run says
// what it would do.
func ImportContacts(req *common.ContactImportRequest) (common.ContactImportResult, error) {
	mode, err := importMode(req.Mode)
	if err != nil {
		return common.ContactImportResult{}, err
	}
	rtype := strings.TrimSpace(req.Type)
	if rtype == "" {
		rtype = "contact"
	}
	res := common.ContactImportResult{Ok: true, DryRun: req.DryRun, Mode: mode, Type: rtype}

	cards := contactsFromVCard(req.Text)
	if len(cards) == 0 {
		return res, fmt.Errorf("there are no contacts in that - a vCard file starts with BEGIN:VCARD")
	}
	res.Cards = len(cards)

	contactImportMu.Lock()
	defer contactImportMu.Unlock()

	plans := planImport(cards, allRecords(rtype, true), mode, recordPhoto)

	adds := 0
	for _, p := range plans {
		if p.item.Action == "add" {
			adds++
		}
	}
	var target importTargetAt
	if adds > 0 {
		target, err = importTarget(rtype, req.TargetHash, req.Filename)
		if err != nil {
			return res, err
		}
		res.Target = target.filename
	}

	if !req.DryRun {
		// Matches first: they find their record by hash, and new headings
		// spliced in above a record can change the hashes after them.
		for _, p := range plans {
			if p.item.Action == "update" {
				if err := applyUpdate(p); err != nil {
					p.item.Action, p.item.Reason = "error", err.Error()
				}
			}
		}
		if adds > 0 {
			// Measured again: the updates above may have added lines to
			// records inside the container, and the row found before them
			// would put the new people in the middle of somebody else.
			target, err = importTarget(rtype, req.TargetHash, req.Filename)
			if err == nil {
				err = applyAdds(plans, rtype, target)
			}
			if err != nil {
				for _, p := range plans {
					if p.item.Action == "add" {
						p.item.Action, p.item.Reason = "error", err.Error()
					}
				}
			}
		}
	}

	for _, p := range plans {
		switch p.item.Action {
		case "add":
			res.Added++
		case "update":
			res.Updated++
		case "skip":
			res.Skipped++
		case "error":
			res.Failed++
		}
		res.Items = append(res.Items, p.item)
	}
	return res, nil
}

// Where new people go.
type importTargetAt struct {
	filename string
	// The row the new headings go after, and the level they are written at.
	row, lvl int
}

// importTarget is AddRecord's choice of place, for many records at once: the
// heading named, the end of the file named, or the collection's container.
func importTarget(rtype, targetHash, filename string) (importTargetAt, error) {
	parent := targetHash
	if parent == "" && filename == "" {
		for _, c := range allCollections() {
			if strings.EqualFold(c.Type, rtype) && c.Hash != "" {
				parent = c.Hash
				break
			}
		}
		if parent == "" {
			return importTargetAt{}, fmt.Errorf("no %q collection yet to add people to - make one, or say which file to add them to", rtype)
		}
	}
	if parent != "" {
		sec := GetDb().FindByHash(parent)
		if sec == nil || sec.Headline == nil {
			return importTargetAt{}, fmt.Errorf("no heading with that hash")
		}
		f := GetDb().FileFromSection(sec)
		if f == nil || f.Doc == nil {
			return importTargetAt{}, fmt.Errorf("could not find the file that heading is in")
		}
		lines, err := ganttFileLines(f.Doc.Path)
		if err != nil {
			return importTargetAt{}, err
		}
		return importTargetAt{
			filename: f.Doc.Path, lvl: sec.Headline.Lvl + 1,
			row: subtreeEndRow(lines, sec.Headline.Pos.Row, sec.Headline.Lvl, sec.Headline.Pos.Row),
		}, nil
	}
	f := GetDb().FindByFile(filename)
	if f == nil || f.Doc == nil {
		return importTargetAt{}, fmt.Errorf("no file called %q", filename)
	}
	lines, err := ganttFileLines(f.Doc.Path)
	if err != nil {
		return importTargetAt{}, err
	}
	return importTargetAt{filename: f.Doc.Path, lvl: 1, row: len(lines) - 1}, nil
}

var addedLineRe = regexp.MustCompile(`^\s*- Added \[[^\]]+\]$`)

// applyAdds writes every new person in one splice.
func applyAdds(plans []*contactPlan, rtype string, at importTargetAt) error {
	lines, err := ganttFileLines(at.filename)
	if err != nil {
		return err
	}
	order := []string{}
	for _, c := range allCollections() {
		if strings.EqualFold(c.Type, rtype) {
			order = append(order, c.Fields...)
			break
		}
	}
	block := []string{}
	for _, p := range plans {
		if p.item.Action != "add" {
			continue
		}
		fields := map[string]string{}
		for k, v := range p.set {
			fields[k] = v
		}
		if p.photo {
			link, err := savePhoto(at.filename, p.c.Name, p.c.Photo)
			if err != nil {
				p.item.Warnings = append(p.item.Warnings, "the photo could not be saved: "+err.Error())
			} else {
				fields["IMAGE"] = link
			}
		}
		// The collection's order first, then the card's.
		heading := recordHeadingLines(at.lvl, p.c.Name, rtype, fields, append(append([]string{}, order...), p.order...), p.notes, p.tags)
		for i, l := range heading {
			if addedLineRe.MatchString(l) {
				heading[i] = l + " " + vcardVia
				break
			}
		}
		block = append(block, heading...)
		p.item.Filename = at.filename
	}
	if len(block) == 0 {
		return nil
	}
	block = spaceBefore(lines, at.row, block)
	_, err = insertLinesAt(at.filename, at.row, block)
	return err
}

// applyUpdate writes what one matched person gains: the fields through
// UpdateRecord, then the tags and the note as line edits of the record's own
// headline and body.
func applyUpdate(p *contactPlan) error {
	hash, err := currentHash(p.rec)
	if err != nil {
		return err
	}
	set := map[string]string{}
	for k, v := range p.set {
		set[k] = v
	}
	if p.photo {
		link, err := savePhoto(p.rec.Filename, p.c.Name, p.c.Photo)
		if err != nil {
			p.item.Warnings = append(p.item.Warnings, "the photo could not be saved: "+err.Error())
		} else {
			set["IMAGE"] = link
		}
	}
	if len(set) > 0 {
		if _, err := UpdateRecord(&common.RecordUpdate{Hash: hash, Set: set, Via: vcardVia}); err != nil {
			return err
		}
	}
	if len(p.tags) == 0 && p.notes == "" {
		return nil
	}
	if hash, err = currentHash(p.rec); err != nil {
		return err
	}
	sec := GetDb().FindByHash(hash)
	lines, from, to, ok := recordLines(p.rec.Filename, sec)
	if !ok {
		return fmt.Errorf("could not read %s", p.rec.Filename)
	}
	for _, t := range p.tags {
		lines[from], _ = addHeadlineTag(lines[from], t)
	}
	if p.notes != "" {
		lvl := sec.Headline.Lvl
		ind := indentOf(lvl)
		lines = appendNote(lines, from, to, ind, p.notes)
		lines, from, to = reread(lines, from, lvl)
		lines = prependLogbook(lines, from, to, ind, "Noted", time.Now(), vcardVia)
	}
	return writeLines(p.rec.Filename, lines)
}

// appendNote puts a note at the end of a record's own text - after what is
// written there, before any child heading - with a blank line before it.
func appendNote(lines []string, from, to int, ind, note string) []string {
	own := to
	for i := from + 1; i <= to && i < len(lines); i++ {
		if vcIsHeading(lines[i]) {
			own = i - 1
			break
		}
	}
	at := own
	for at > from && strings.TrimSpace(lines[at]) == "" {
		at--
	}
	add := []string{""}
	for _, l := range strings.Split(strings.TrimRight(note, " \t\n"), "\n") {
		if strings.TrimSpace(l) == "" {
			add = append(add, "")
		} else {
			add = append(add, ind+l)
		}
	}
	// Keep a blank line between the note and whatever comes next.
	if at+1 < len(lines) && strings.TrimSpace(lines[at+1]) != "" {
		add = append(add, "")
	}
	return splice(lines, at+1, add)
}

func vcIsHeading(l string) bool {
	stars := 0
	for stars < len(l) && l[stars] == '*' {
		stars++
	}
	return stars > 0 && stars < len(l) && l[stars] == ' '
}

// currentHash is the record's hash now. The one the plan was made with is
// usually still right; when an earlier write moved it, the record is found
// again by its ID, which no write changes.
func currentHash(rec *common.Record) (string, error) {
	if sec := GetDb().FindByHash(rec.Hash); sec != nil && sectionTitle(sec) == rec.Name {
		return rec.Hash, nil
	}
	if rec.Id != "" {
		for _, r := range allRecords(rec.Type, false) {
			if r.Id == rec.Id {
				return r.Hash, nil
			}
		}
	}
	return "", fmt.Errorf("could not find %s again after an earlier write", rec.Name)
}

// ----------------------------------------------------------------------------
// Pictures
// ----------------------------------------------------------------------------

// The folder, beside the org file, that contact photos are kept in. Relative,
// so the link in the file survives the org directory moving.
const contactPhotoDir = "images/contacts"

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func photoSlug(name string) string {
	s := strings.Trim(slugRe.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if len(s) > 60 {
		s = strings.Trim(s[:60], "-")
	}
	if s == "" {
		s = "contact"
	}
	return s
}

// savePhoto writes a contact's picture beside the org file it is linked from
// and answers with the link. The same picture already there is reused rather
// than written again as jane-2.jpg, which is what keeps a re-import from
// filling the folder.
func savePhoto(orgFile, name string, ph *vcPhoto) (string, error) {
	if ph == nil || len(ph.Data) == 0 {
		return "", fmt.Errorf("no picture")
	}
	dir := filepath.Join(filepath.Dir(orgFile), filepath.FromSlash(contactPhotoDir))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	slug := photoSlug(name)
	for n := 1; n < 1000; n++ {
		fname := slug + "." + ph.Ext
		if n > 1 {
			fname = fmt.Sprintf("%s-%d.%s", slug, n, ph.Ext)
		}
		path := filepath.Join(dir, fname)
		if have, err := os.ReadFile(path); err == nil {
			if samePhoto(have, ph.Data) {
				return "[[file:" + contactPhotoDir + "/" + fname + "]]", nil
			}
			continue
		}
		if err := os.WriteFile(path, ph.Data, 0644); err != nil {
			return "", err
		}
		return "[[file:" + contactPhotoDir + "/" + fname + "]]", nil
	}
	return "", fmt.Errorf("too many pictures called %s", slug)
}

// recordPhoto is the bytes of a record's own picture, when it is a file.
func recordPhoto(rec *common.Record) []byte {
	for _, f := range rec.Fields {
		if f.Kind != "image" {
			continue
		}
		t := linkTarget(f.Value)
		if t == "" || strings.Contains(t, "://") {
			return nil
		}
		if !filepath.IsAbs(t) {
			t = filepath.Join(filepath.Dir(rec.Filename), filepath.FromSlash(t))
		}
		b, err := os.ReadFile(t)
		if err != nil {
			return nil
		}
		return b
	}
	return nil
}

// ----------------------------------------------------------------------------
// REST
// ----------------------------------------------------------------------------

// A .vcf of a few thousand people with photos runs to tens of megabytes.
const maxContactImport = 128 << 20

/* SDOC: API
* POST /contacts/import — Import Contacts From A vCard File

	Reads a =.vcf= file - Apple Contacts, Google Contacts, Outlook; vCard 2.1,
	3.0 and 4.0 - into the contact records, matching the people already there.

	*Method:* =POST=

	*Request:*
	| Field      | Meaning                                                          |
	|------------+------------------------------------------------------------------|
	| =Text=     | The file's text. Several files are their texts one after another |
	| =Mode=     | =merge= (default), =skip= or =overwrite=                         |
	| =DryRun=   | =true= answers with the plan and writes nothing                  |
	| =Type=     | The collection; =contact= when empty                             |
	| =TargetHash=, =Filename= | Where new people go. Neither: the collection's container |

	A person is matched by the card's =UID= (kept as =VCARD_UID=), then any
	email, then the name. =merge= adds new people and fills in only what a
	matched record lacks - a second email is filed beside the first, a
	different birthday is reported as =kept= and not written. =skip= leaves
	matches alone; =overwrite= writes the card's values over them. Nothing is
	ever removed or renamed; a note is appended only when it is new. Importing
	the same file twice changes nothing the second time.

	Inline photos are saved in =images/contacts/= beside the org file and
	linked from =IMAGE=. Each record's =LOGBOOK= says the change came from the
	import.

	*Response:* =Ok=, =Msg=, =DryRun=, =Mode=, =Cards= (cards read),
	=Added=, =Updated=, =Skipped=, =Failed=, =Target= (the file new people go
	to), and =Items=: per person =Card=, =Name=, =Action= (=add=, =update=,
	=skip=, =error=), =Reason=, =Match= (=uid=, =email=, =name=), =Hash=,
	=Changes= (={Key, Value, Old, How}=, =How= being =add=, =replace= or
	=kept=), =Tags=, =Notes= and =Warnings=.
EDOC */

// PostContactImport is the endpoint.
func PostContactImport(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxContactImport))
	if err != nil {
		answerErr(w, err)
		return
	}
	var req common.ContactImportRequest
	if err := json.Unmarshal(body, &req); err != nil {
		answerErr(w, err)
		return
	}
	res, err := ImportContacts(&req)
	if err != nil {
		res.Ok = false
		res.Msg = err.Error()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}
