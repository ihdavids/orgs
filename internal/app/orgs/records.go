package orgs

/* SDOC: Records
* Records And Collections

  A *record* is one heading that stands for one thing - a person, a laptop, a
  playing card - and keeps what is known about it in its property drawer. The
  heading is the thing's name, the properties are its fields, the body under it
  is whatever anybody wanted to write about it, and the =LOGBOOK= drawer is the
  history of what has been changed.

** What makes a heading a record

   One property, and nothing else:

   #+BEGIN_EXAMPLE
   :RECORD: contact
   #+END_EXAMPLE

   The value names the *collection* the record belongs to. =contact= is just
   the collection this was built for first; =equipment=, =cards=, =wine= and
   =synths= are records in exactly the same sense and are read by exactly the
   same code. A heading with no =RECORD= property is not a record, wherever it
   sits, and a heading with one is a record even if it sits on its own in the
   middle of a project file - which is what makes a record identifiable
   without any knowledge of where it lives.

** A contact

   #+BEGIN_EXAMPLE
   * John Doe
     :PROPERTIES:
     :RECORD:       contact
     :ID:           b1d1a2f4-3c58-4f2a-9a3d-0f9a1c7e51aa
     :ADDED:        [2026-09-26 Sat 09:12]
     :EMAIL:        john.doe@example.com
     :EMAIL_WORK:   jdoe@acme.example.com
     :PHONE_MOBILE: +1-555-555-0199
     :PHONE_HOME:   +1-555-555-0100
     :ADDRESS:      123 Main St, Cityville
     :BIRTHDAY:     1990-05-15
     :IMAGE:        [[file:images/contacts/john-doe.jpg]]
     :LINKEDIN:     https://www.linkedin.com/in/johndoe
     :END:
     :LOGBOOK:
     - Updated [2026-09-26 Sat 14:02] EMAIL_WORK, PHONE_MOBILE
     :END:

     Met at the 2019 conference. Allergic to shellfish.
   #+END_EXAMPLE

** Fields, and how a field knows what it is

   A field is a property named =NAME= or =NAME_LABEL=. The label is free text
   and is how a thing has more than one of something: =PHONE_MOBILE=,
   =PHONE_HOME= and =PHONE_WORK= are three phone numbers, =EMAIL= and
   =EMAIL_WORK= are two email addresses. Nothing has to be declared first.

   What a field *is* is worked out from the part before the label, so a client
   draws a field by its kind and never needs a list of property names:

   | Name                                          | Kind    | Drawn as                    |
   |-----------------------------------------------+---------+-----------------------------|
   | EMAIL, MAIL                                   | email   | a mailto: link              |
   | PHONE, MOBILE, CELL, TEL, FAX                 | tel     | a tel: link                 |
   | ADDRESS, ADDR                                 | postal  | text, with a map link       |
   | URL, WEBSITE, HOMEPAGE, LINK, BLOG            | url     | a link                      |
   | LINKEDIN, FACEBOOK, TWITTER, GITHUB, ...      | social  | the network's own badge     |
   | BIRTHDAY, ANNIVERSARY, and anything *_DATE    | date    | a date, and on the agenda   |
   | IMAGE, PHOTO, AVATAR, PORTRAIT, PICTURE, LOGO | image   | the picture itself          |
   | anything else                                 | text    | as written                  |

   The social names are a list rather than a rule because that is what they
   are; see =socialNetworks= below to add one. Everything else is a rule, so a
   collection of guitar pedals gets =MANUAL_URL= drawn as a link and
   =BOUGHT_DATE= drawn as a date without anybody teaching it anything.

** The bookkeeping fields

   | Property | Written by | Means                                             |
   |----------+------------+---------------------------------------------------|
   | RECORD   | you or us  | which collection this belongs to - the identity   |
   | ID       | us         | a uuid, so links and other records can point here |
   | ADDED    | us         | when the record was first written down            |

   They are kept out of the field list a client is handed, because they are
   about the record rather than about the thing.

** The history

   Every change made through the record endpoints prepends a line to the
   heading's =LOGBOOK= drawer:

   #+BEGIN_EXAMPLE
   :LOGBOOK:
   - Updated [2026-09-26 Sat 14:02] EMAIL_WORK, PHONE_MOBILE
   - Renamed [2026-09-26 Sat 13:55] John Doe
   - Added [2026-09-26 Sat 09:12]
   :END:
   #+END_EXAMPLE

   Newest first, the way org writes a logbook. The clock entries org puts in
   the same drawer are left alone and ignored, and a line somebody writes by
   hand that does not match is skipped rather than being a parse error.

** Collections

   A collection is every record sharing one =RECORD= value. It does not have
   to be declared - the first heading with =:RECORD: wine:= on it makes the
   wine collection exist - but a *container* gives new records somewhere to go
   and gives the collection a name and an icon:

   #+BEGIN_EXAMPLE
   * Contacts
     :PROPERTIES:
     :COLLECTION: contact
     :ICON:       👤
     :FIELDS:     EMAIL, PHONE_MOBILE, ADDRESS, BIRTHDAY, IMAGE
     :END:
   #+END_EXAMPLE

   =FIELDS= is the order an add form should offer, not a restriction: a record
   may carry anything. Where there is no container the server offers the fields
   the records of that type already use, commonest first.

** Duplicating an entry

   Copy the heading, change the name, change the values, and delete the =ID=
   and =ADDED= lines - the server fills those in the next time it writes to the
   record, and two records with the same =ID= is the one thing that will
   confuse a link. Nothing else about a record is derived, so a record written
   by hand in a text editor is the same as one added from worg.
EDOC */

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// ----------------------------------------------------------------------------
// What a record is
// ----------------------------------------------------------------------------

// The property that makes a heading a record, and the one that makes a heading
// the container of a collection.
const (
	RecordProp     = "RECORD"
	CollectionProp = "COLLECTION"
)

// The properties that are about the record rather than about the thing. They
// are kept out of the fields a client is handed to draw.
var recordBookkeeping = map[string]bool{
	RecordProp: true, "ID": true, "ADDED": true,
	CollectionProp: true, "FIELDS": true, "ICON": true,
}

// The social networks, which are a list because that is what they are - there
// is no rule that makes "linkedin" a network and "location" not one. Add a
// name here and its field gets the network's badge; leave it out and the field
// is still drawn, as a link or as text.
var socialNetworks = map[string]string{
	"LINKEDIN": "linkedin", "FACEBOOK": "facebook", "TWITTER": "twitter",
	"X": "twitter", "INSTAGRAM": "instagram", "MASTODON": "mastodon",
	"GITHUB": "github", "GITLAB": "gitlab", "BLUESKY": "bluesky",
	"YOUTUBE": "youtube", "TIKTOK": "tiktok", "REDDIT": "reddit",
	"TELEGRAM": "telegram", "SIGNAL": "signal", "WHATSAPP": "whatsapp",
	"DISCORD": "discord", "SLACK": "slack", "MATRIX": "matrix",
	"THREADS": "threads", "SNAPCHAT": "snapchat", "PINTEREST": "pinterest",
	"TWITCH": "twitch", "SOUNDCLOUD": "soundcloud", "SPOTIFY": "spotify",
	"STRAVA": "strava", "GOODREADS": "goodreads", "KEYBASE": "keybase",
}

var (
	emailNames  = map[string]bool{"EMAIL": true, "MAIL": true, "E-MAIL": true}
	telNames    = map[string]bool{"PHONE": true, "MOBILE": true, "CELL": true, "TEL": true, "FAX": true, "TELEPHONE": true}
	postalNames = map[string]bool{"ADDRESS": true, "ADDR": true}
	urlNames    = map[string]bool{"URL": true, "WEBSITE": true, "WEB": true, "HOMEPAGE": true, "LINK": true, "BLOG": true, "SITE": true}
	dateNames   = map[string]bool{"BIRTHDAY": true, "BIRTHDATE": true, "DOB": true, "ANNIVERSARY": true, "DATE": true}
	imageNames  = map[string]bool{"IMAGE": true, "PHOTO": true, "AVATAR": true, "PORTRAIT": true, "PICTURE": true, "LOGO": true, "THUMBNAIL": true}
)

// The fields a birthday can be written in, and what each is called on the
// agenda.
var birthdayFields = map[string]bool{"BIRTHDAY": true, "BIRTHDATE": true, "DOB": true, "ANNIVERSARY": true}

// splitFieldName takes "PHONE_MOBILE" apart into "PHONE" and "MOBILE".
//
// The split is at the *first* underscore, so "EMAIL_WORK_OLD" is an EMAIL
// labelled "WORK OLD" rather than an "EMAIL_WORK" labelled "OLD" - a label is
// free text and a name is not.
func splitFieldName(key string) (name string, label string) {
	up := strings.ToUpper(strings.TrimSpace(key))
	if i := strings.Index(up, "_"); i > 0 {
		return up[:i], strings.ReplaceAll(up[i+1:], "_", " ")
	}
	return up, ""
}

// The suffixes that say what a field is rather than labelling it. A field is
// usually named kind-first - EMAIL_WORK, PHONE_MOBILE - but the things people
// keep in a collection are as often named the other way round: PURCHASE_DATE,
// MANUAL_URL, BOX_IMAGE. Read as a label those come out as text and lose their
// link.
//
// Deliberately short. EMAIL and PHONE are not in it, because WORK_EMAIL is
// rarer than EMAIL_WORK and MOBILE as a suffix is the label on PHONE_MOBILE -
// reading that as a kind would throw the label away.
var kindSuffix = map[string]string{
	"DATE": "date", "URL": "url", "LINK": "url", "IMAGE": "image", "PHOTO": "image",
}

// fieldOf takes a property name apart into what to call it, what to label it,
// and what it is.
func fieldOf(key string) (name string, label string, kind string) {
	up := strings.ToUpper(strings.TrimSpace(key))
	if i := strings.LastIndex(up, "_"); i > 0 {
		if k, ok := kindSuffix[up[i+1:]]; ok {
			return strings.ReplaceAll(up[:i], "_", " "), "", k
		}
	}
	name, label = splitFieldName(up)
	return name, label, fieldKind(name)
}

// fieldKind is what a field is, from the part of its name before the label.
func fieldKind(name string) string {
	switch {
	case emailNames[name]:
		return "email"
	case telNames[name]:
		return "tel"
	case postalNames[name]:
		return "postal"
	case urlNames[name]:
		return "url"
	case socialNetworks[name] != "":
		return "social"
	case dateNames[name] || strings.HasSuffix(name, "DATE") || strings.HasPrefix(name, "DATE"):
		return "date"
	case imageNames[name]:
		return "image"
	}
	return "text"
}

// A bare handle rather than a url: "@johndoe", "johndoe". A social field is
// usually written one way or the other and both have to work.
var handleRe = regexp.MustCompile(`^@?[A-Za-z0-9._-]+$`)

// fieldLink makes a value actionable. An empty answer means "this is text,
// print it" - which is the right answer for a phone extension, a note in an
// address field, or a social handle nobody can build a url for.
func fieldLink(kind, name, value, fromFile string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return ""
	}
	switch kind {
	case "email":
		if strings.Contains(v, "@") {
			return "mailto:" + v
		}
	case "tel":
		// Everything a phone number is allowed to be written with, and
		// nothing else - a "PHONE: ask Jane" is text.
		digits := strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, v)
		if len(digits) >= 5 {
			return "tel:" + strings.TrimSpace(strings.Map(func(r rune) rune {
				if strings.ContainsRune("0123456789+", r) {
					return r
				}
				return -1
			}, v))
		}
	case "url":
		return webURL(v)
	case "social":
		// A handle is tried before the url heuristic, not after it. A username
		// like "jane.42" has a dot in it and no spaces, which is all webURL
		// asks of a domain - so left to itself it would send somebody to
		// https://jane.42.
		if t := linkTarget(v); t != "" {
			v = t
		}
		if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
			return v
		}
		if handleRe.MatchString(v) {
			// A network with no predictable address keeps its handle as text
			// rather than being sent somewhere guessed at.
			if base := socialBase[socialNetworks[name]]; base != "" {
				return base + strings.TrimPrefix(v, "@")
			}
			return ""
		}
		// Not a handle and not a full address: "linkedin.com/in/johndoe".
		return webURL(v)
	case "image":
		if t := linkTarget(v); t != "" {
			if strings.HasPrefix(t, "http://") || strings.HasPrefix(t, "https://") {
				return t
			}
			return mediaURL(t, fromFile)
		}
	case "postal":
		return "https://www.openstreetmap.org/search?query=" + urlQueryEscape(v)
	}
	return ""
}

// Where a bare handle lives, for the networks whose url is predictable. A
// network with no entry keeps its handle as text rather than being sent to a
// guessed address.
var socialBase = map[string]string{
	"linkedin":  "https://www.linkedin.com/in/",
	"facebook":  "https://www.facebook.com/",
	"twitter":   "https://twitter.com/",
	"instagram": "https://www.instagram.com/",
	"github":    "https://github.com/",
	"gitlab":    "https://gitlab.com/",
	"bluesky":   "https://bsky.app/profile/",
	"youtube":   "https://www.youtube.com/@",
	"tiktok":    "https://www.tiktok.com/@",
	"reddit":    "https://www.reddit.com/user/",
	"telegram":  "https://t.me/",
	"twitch":    "https://www.twitch.tv/",
	"threads":   "https://www.threads.net/@",
	"pinterest": "https://www.pinterest.com/",
	"strava":    "https://www.strava.com/athletes/",
}

func urlQueryEscape(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, " ", "+"), "&", "%26")
}

// webURL is the value as a web address, or empty when it is not one. An org
// link is unwrapped first, since that is how somebody pastes a url with a
// description on it.
func webURL(v string) string {
	if t := linkTarget(v); t != "" {
		v = t
	}
	if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
		return v
	}
	// "www.example.com" and "example.com/x" are addresses; "Jane's place" is
	// not. A dot with no spaces around it is the whole test.
	if !strings.ContainsAny(v, " \t") && strings.Contains(v, ".") && !strings.Contains(v, "@") {
		return "https://" + v
	}
	return ""
}

// ----------------------------------------------------------------------------
// Reading records out of the database
// ----------------------------------------------------------------------------

// RecordTypeOf is the collection a heading belongs to, or "" when the heading
// is not a record.
func RecordTypeOf(sec *org.Section) string {
	return strings.TrimSpace(GetProp(sec, RecordProp, "Record", "record"))
}

// CollectionTypeOf is the collection a heading is the container for, or "".
func CollectionTypeOf(sec *org.Section) string {
	return strings.TrimSpace(GetProp(sec, CollectionProp, "Collection", "collection"))
}

// sectionProps is every property of a heading, in the order they are written.
func sectionProps(sec *org.Section) ([]string, map[string]string) {
	keys := []string{}
	out := map[string]string{}
	if sec == nil || sec.Headline == nil || sec.Headline.Properties == nil {
		return keys, out
	}
	for _, p := range sec.Headline.Properties.Properties {
		k := strings.TrimSpace(p[0])
		if k == "" {
			continue
		}
		if _, seen := out[k]; !seen {
			keys = append(keys, k)
		}
		out[k] = strings.TrimSpace(p[1])
	}
	return keys, out
}

func sectionTitle(sec *org.Section) string {
	var title string
	if sec == nil || sec.Headline == nil {
		return ""
	}
	for _, n := range sec.Headline.Title {
		title += n.String()
	}
	return strings.TrimSpace(title)
}

// readRecord turns one section into a record. The body and the logbook are
// read off the file's own lines rather than out of the parsed document: a
// drawer written in column zero ends the headline's body as far as go-org is
// concerned, and the rest of the heading is hoisted to the top of the
// document, where looking for it is guesswork.
func readRecord(sec *org.Section, f *common.OrgFile, withBody bool) *common.Record {
	rtype := RecordTypeOf(sec)
	if rtype == "" || sec.Headline == nil {
		return nil
	}
	rec := &common.Record{
		Hash:    sec.Hash,
		Type:    rtype,
		Name:    sectionTitle(sec),
		LineNum: sec.Headline.Pos.Row,
		Tags:    sec.Headline.Tags,
	}
	if f != nil && f.Doc != nil {
		rec.Filename = f.Doc.Path
	}
	keys, props := sectionProps(sec)
	rec.Props = props
	rec.Id = props["ID"]
	rec.Added = trimStamp(props["ADDED"])

	for _, k := range keys {
		if recordBookkeeping[strings.ToUpper(k)] {
			continue
		}
		name, label, kind := fieldOf(k)
		fld := common.RecordField{
			Key: k, Name: name, Label: label, Kind: kind,
			Value:   props[k],
			Network: socialNetworks[name],
			Link:    fieldLink(kind, name, props[k], rec.Filename),
		}
		if kind == "image" && rec.Image == "" {
			rec.Image = fld.Link
		}
		rec.Fields = append(rec.Fields, fld)
	}

	if withBody {
		if lines, from, to, ok := recordLines(rec.Filename, sec); ok {
			rec.Notes = recordNotes(lines, from, to)
			rec.History = readLogbook(lines, from, to)
		}
	}
	if len(rec.History) > 0 {
		rec.Updated = rec.History[0].When
	}
	if rec.Updated == "" {
		rec.Updated = rec.Added
	}
	return rec
}

// An org timestamp with its brackets taken off: "[2026-09-26 Sat 09:12]" is
// stored, "2026-09-26 Sat 09:12" is what a client wants to print.
func trimStamp(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "[")
	v = strings.TrimPrefix(v, "<")
	v = strings.TrimSuffix(v, "]")
	v = strings.TrimSuffix(v, ">")
	return strings.TrimSpace(v)
}

// Every record in the database.
//
// Reading the records means walking every section of every file, and reading
// their notes and history means opening every file one of them is in. The
// contacts tab searches as you type, so that happens per keystroke without a
// cache - and it used to be one cache of the whole database gated on the reload
// counter, which meant a saved file threw away every file's records.
//
// It is now a cache per file (see fileparts.go), so a save costs the file that
// was saved. The joining below - the collections, their counts, the sort - is
// over the parts and is cheap.
func cachedRecords() ([]*common.Record, []common.RecordCollection) {
	return walkRecords()
}

// allRecords is every record of one type, or of every type when rtype is
// empty. withBody is accepted for the callers that say they do not need the
// notes, but the walk reads them anyway and caches the lot: the second caller
// along almost always does want them, and reading a file twice costs more than
// keeping what was read.
func allRecords(rtype string, withBody bool) []*common.Record {
	recs, _ := cachedRecords()
	if rtype == "" {
		return recs
	}
	out := []*common.Record{}
	for _, rec := range recs {
		if strings.EqualFold(rec.Type, rtype) {
			out = append(out, rec)
		}
	}
	return out
}

// walkRecords is the walk itself. Sections are registered lazily as queries
// touch them, so this registers as it goes for the same reason the link index
// does - a record looked up by hash straight afterwards has to be there.
// What one file contributes: its records, and the container headings it holds.
type recordPart struct {
	recs []*common.Record
	// One entry per collection type found in this file, in the order found.
	cols  []common.RecordCollection
	order []string
	// How many records of each type this file holds, for the counts.
	counts map[string]int
}

// One file's records, cached per file.
//
// Was one walk of every file gated on the reload counter, so a saved file cost
// a walk of the database - and the contacts tab searches as you type.
var recordParts = NewFileParts[recordPart]()

// walkRecords is the walk itself, now per file and joined afterwards.
//
// Sections are registered lazily as queries touch them, so this registers as it
// goes for the same reason the link index does - a record looked up by hash
// straight afterwards has to be there.
func walkRecords() ([]*common.Record, []common.RecordCollection) {
	per := recordParts.All(func(f *common.OrgFile) recordPart {
		db := GetDb()
		fname := f.Doc.Path
		part := recordPart{counts: map[string]int{}}
		byType := map[string]int{}
		for _, sec := range flattenSections(f) {
			db.RegisterSection(sec.Hash, sec, f)
			if t := CollectionTypeOf(sec); t != "" {
				if _, have := byType[t]; !have {
					c := common.RecordCollection{Type: t, Name: titleOf(t)}
					c.Hash = sec.Hash
					c.Filename = fname
					c.LineNum = sec.Headline.Pos.Row
					if n := sectionTitle(sec); n != "" {
						c.Name = n
					}
					_, props := sectionProps(sec)
					c.Icon = props["ICON"]
					c.Fields = splitList(props["FIELDS"])
					byType[t] = len(part.cols)
					part.cols = append(part.cols, c)
					part.order = append(part.order, t)
				}
			}
			if t := RecordTypeOf(sec); t != "" {
				part.counts[t]++
				if rec := readRecord(sec, f, true); rec != nil {
					part.recs = append(part.recs, rec)
				}
			}
		}
		return part
	})

	out := []*common.Record{}
	byType := map[string]*common.RecordCollection{}
	order := []string{}
	get := func(t string) *common.RecordCollection {
		if c, ok := byType[t]; ok {
			return c
		}
		c := &common.RecordCollection{Type: t, Name: titleOf(t)}
		byType[t] = c
		order = append(order, t)
		return c
	}
	for _, part := range per {
		out = append(out, part.recs...)
		for _, c := range part.cols {
			existing := get(c.Type)
			// The first container found wins, so that a second one somewhere
			// else does not quietly become the place new records land. "First"
			// is in the database's file order, which is why the parts have to
			// come back in that order rather than a map's.
			//
			// Only the container's own fields are taken: the count is
			// accumulated across every file and a wholesale copy would put it
			// back to whatever this file's container said, which is zero.
			if existing.Hash == "" {
				existing.Hash = c.Hash
				existing.Filename = c.Filename
				existing.LineNum = c.LineNum
				existing.Name = c.Name
				existing.Icon = c.Icon
				existing.Fields = c.Fields
			}
		}
		for t, n := range part.counts {
			get(t).Count += n
		}
	}

	sort.SliceStable(out, func(a, b int) bool {
		if out[a].Type != out[b].Type {
			return out[a].Type < out[b].Type
		}
		return strings.ToLower(out[a].Name) < strings.ToLower(out[b].Name)
	})

	cols := []common.RecordCollection{}
	for _, t := range order {
		cols = append(cols, *byType[t])
	}
	sort.SliceStable(cols, func(a, b int) bool {
		// Contacts first when it is there - it is the one this was built for
		// and the one most people open - and everything else by name.
		if (cols[a].Type == "contact") != (cols[b].Type == "contact") {
			return cols[a].Type == "contact"
		}
		return strings.ToLower(cols[a].Name) < strings.ToLower(cols[b].Name)
	})
	return out, cols
}

// allCollections is every collection the database knows about: the ones that
// have a container heading, and the ones that exist only because records carry
// the type.
func allCollections() []common.RecordCollection {
	_, cols := cachedRecords()
	return cols
}

func titleOf(t string) string {
	t = strings.TrimSpace(t)
	if t == "" {
		return t
	}
	return strings.ToUpper(t[:1]) + t[1:]
}

func splitList(v string) []string {
	out := []string{}
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// recordMatches is the search behind /records?q=. It is a substring match over
// the name, every field value and the tags, because somebody looking for a
// contact types a phone number as readily as a name.
func recordMatches(rec *common.Record, q string) bool {
	if q == "" {
		return true
	}
	needle := strings.ToLower(q)
	if strings.Contains(strings.ToLower(rec.Name), needle) {
		return true
	}
	for _, t := range rec.Tags {
		if strings.Contains(strings.ToLower(t), needle) {
			return true
		}
	}
	for _, f := range rec.Fields {
		if strings.Contains(strings.ToLower(f.Value), needle) {
			return true
		}
		if strings.Contains(strings.ToLower(f.Key), needle) {
			return true
		}
	}
	return strings.Contains(strings.ToLower(rec.Notes), needle)
}

// ----------------------------------------------------------------------------
// The lines of a record, and the drawers in them
// ----------------------------------------------------------------------------

var (
	drawerOpenRe  = regexp.MustCompile(`^\s*:([A-Za-z][A-Za-z0-9_\-]*):\s*$`)
	drawerEndRe   = regexp.MustCompile(`^\s*:END:\s*$`)
	propLineRe    = regexp.MustCompile(`^(\s*):([^:\s]+):([ \t]*)(.*)$`)
	planningRe    = regexp.MustCompile(`^\s*(SCHEDULED|DEADLINE|CLOSED):`)
	logbookLineRe = regexp.MustCompile(`^\s*-\s+(Added|Updated|Renamed|Noted)\s+\[([^\]]+)\]\s*(.*)$`)
)

// recordLines reads the file a record is in and answers with its lines and the
// half open range the record's own heading covers - from its headline row to
// the row before the next heading at its level or above.
func recordLines(filename string, sec *org.Section) (lines []string, from int, to int, ok bool) {
	if filename == "" || sec == nil || sec.Headline == nil {
		return nil, 0, 0, false
	}
	b, err := os.ReadFile(filename)
	if err != nil {
		return nil, 0, 0, false
	}
	lines = strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	from = sec.Headline.Pos.Row
	if from < 0 || from >= len(lines) {
		return nil, 0, 0, false
	}
	to = subtreeEndRow(lines, from, sec.Headline.Lvl, from)
	if to >= len(lines) {
		to = len(lines) - 1
	}
	return lines, from, to, true
}

// drawerAt finds a named drawer in the head of a record - the run of planning
// lines, drawers and blanks directly under the headline. Looking only there is
// deliberate: a :LOGBOOK: written inside a child heading or quoted in the notes
// is not this record's logbook.
func drawerAt(lines []string, from, to int, name string) (start int, end int, found bool) {
	want := ":" + strings.ToUpper(name) + ":"
	i := from + 1
	for i <= to && i < len(lines) {
		line := lines[i]
		if strings.TrimSpace(line) == "" || planningRe.MatchString(line) {
			i++
			continue
		}
		m := drawerOpenRe.FindStringSubmatch(line)
		if m == nil {
			return -1, -1, false
		}
		// Walk to this drawer's end whether or not it is the one wanted.
		j := i + 1
		for j <= to && j < len(lines) && !drawerEndRe.MatchString(lines[j]) {
			j++
		}
		if j > to || j >= len(lines) {
			return -1, -1, false
		}
		if strings.EqualFold(strings.TrimSpace(line), want) {
			return i, j, true
		}
		i = j + 1
	}
	return -1, -1, false
}

// headEnd is the row the record's drawers stop at - the first line of its
// notes. A record with no drawers at all has its notes starting right under
// the headline.
func headEnd(lines []string, from, to int) int {
	i := from + 1
	for i <= to && i < len(lines) {
		line := lines[i]
		if planningRe.MatchString(line) {
			i++
			continue
		}
		if drawerOpenRe.MatchString(line) {
			j := i + 1
			for j <= to && j < len(lines) && !drawerEndRe.MatchString(lines[j]) {
				j++
			}
			if j > to || j >= len(lines) {
				return i
			}
			i = j + 1
			continue
		}
		break
	}
	return i
}

// recordNotes is the record's body: everything after its drawers, with the
// leading indent taken off and the blank lines at either end trimmed.
func recordNotes(lines []string, from, to int) string {
	start := headEnd(lines, from, to)
	body := []string{}
	for i := start; i <= to && i < len(lines); i++ {
		body = append(body, strings.TrimRight(lines[i], " \t"))
	}
	for len(body) > 0 && strings.TrimSpace(body[0]) == "" {
		body = body[1:]
	}
	for len(body) > 0 && strings.TrimSpace(body[len(body)-1]) == "" {
		body = body[:len(body)-1]
	}
	// One common indent taken off, so a note written under a level three
	// heading does not come back with three spaces on every line.
	indent := -1
	for _, l := range body {
		if strings.TrimSpace(l) == "" {
			continue
		}
		n := len(l) - len(strings.TrimLeft(l, " \t"))
		if indent < 0 || n < indent {
			indent = n
		}
	}
	if indent > 0 {
		for i, l := range body {
			if len(l) >= indent {
				body[i] = l[indent:]
			} else {
				body[i] = strings.TrimSpace(l)
			}
		}
	}
	return strings.Join(body, "\n")
}

// readLogbook reads the record's history back. Lines that are not history -
// org's own CLOCK entries, a note somebody wrote by hand - are skipped rather
// than being an error, because the drawer is shared with the clock.
func readLogbook(lines []string, from, to int) []common.RecordChange {
	start, end, ok := drawerAt(lines, from, to, "LOGBOOK")
	if !ok {
		return nil
	}
	out := []common.RecordChange{}
	for i := start + 1; i < end; i++ {
		m := logbookLineRe.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		out = append(out, common.RecordChange{
			Kind: strings.ToLower(m[1]),
			When: strings.TrimSpace(m[2]),
			What: strings.TrimSpace(m[3]),
		})
	}
	return out
}

// ----------------------------------------------------------------------------
// Writing
// ----------------------------------------------------------------------------

func orgStamp(t time.Time) string {
	return t.Format("[2006-01-02 Mon 15:04]")
}

func indentOf(lvl int) string {
	if lvl < 1 {
		lvl = 1
	}
	return strings.Repeat(" ", lvl+1)
}

// propLine writes one property drawer line, padded so a drawer reads as a
// column rather than as a ragged list. The padding is cosmetic and nothing
// reads it back.
func propLine(indent, key, value string, width int) string {
	k := ":" + key + ":"
	if len(k) < width {
		k += strings.Repeat(" ", width-len(k))
	}
	return indent + k + " " + value
}

// alignDrawer pads every property in one drawer to a common width, so a field
// added from a client leaves the drawer looking the way a person writing one
// would have left it. Cosmetic - nothing reads the padding back - but a record
// is a text file somebody opens in an editor, and a ragged column is what
// "written by a program" looks like.
func alignDrawer(lines []string, start, end int) {
	keys := []string{}
	for i := start + 1; i < end && i < len(lines); i++ {
		if m := propLineRe.FindStringSubmatch(lines[i]); m != nil {
			keys = append(keys, m[2])
		}
	}
	if len(keys) == 0 {
		return
	}
	w := propWidth(keys)
	for i := start + 1; i < end && i < len(lines); i++ {
		if m := propLineRe.FindStringSubmatch(lines[i]); m != nil {
			lines[i] = propLine(m[1], m[2], m[4], w)
		}
	}
}

func propWidth(keys []string) int {
	w := 0
	for _, k := range keys {
		if n := len(k) + 2; n > w {
			w = n
		}
	}
	if w < 10 {
		w = 10
	}
	if w > 22 {
		w = 22
	}
	return w
}

// spaceBefore puts a blank line in front of what is about to be spliced in,
// when the line it lands after is not already one.
//
// Org does not care, but a person reading the file does: a heading written
// hard against the end of the one before it is the difference between a file
// somebody keeps and a file something appends to.
func spaceBefore(lines []string, row int, add []string) []string {
	if row >= 0 && row < len(lines) && strings.TrimSpace(lines[row]) == "" {
		return add
	}
	return append([]string{""}, add...)
}

// recordHeadingLines builds a whole record as text, ready to be spliced into a
// file. Built as lines rather than written through go-org on purpose: writing
// it through the document would rewrite the whole file to add one heading -
// every drawer re-indented and every table reflowed.
func recordHeadingLines(lvl int, name, rtype string, fields map[string]string, order []string, notes string, tags []string) []string {
	ind := indentOf(lvl)
	head := strings.Repeat("*", lvl) + " " + strings.TrimSpace(name)
	if len(tags) > 0 {
		clean := []string{}
		for _, t := range tags {
			if t = strings.TrimSpace(t); t != "" {
				clean = append(clean, t)
			}
		}
		if len(clean) > 0 {
			head += "  :" + strings.Join(clean, ":") + ":"
		}
	}
	out := []string{head}

	keys := recordFieldOrder(fields, order)
	all := append([]string{RecordProp, "ID", "ADDED"}, keys...)
	w := propWidth(all)

	out = append(out, ind+":PROPERTIES:")
	out = append(out, propLine(ind, RecordProp, rtype, w))
	out = append(out, propLine(ind, "ID", uuid.New().String(), w))
	out = append(out, propLine(ind, "ADDED", orgStamp(time.Now()), w))
	for _, k := range keys {
		out = append(out, propLine(ind, k, strings.TrimSpace(fields[k]), w))
	}
	out = append(out, ind+":END:")
	out = append(out, ind+":LOGBOOK:")
	out = append(out, ind+"- Added "+orgStamp(time.Now()))
	out = append(out, ind+":END:")

	if body := strings.TrimRight(notes, " \t\n"); body != "" {
		out = append(out, "")
		for _, l := range strings.Split(body, "\n") {
			if strings.TrimSpace(l) == "" {
				out = append(out, "")
			} else {
				out = append(out, ind+l)
			}
		}
	}
	out = append(out, "")
	return out
}

// recordFieldOrder puts the fields in the order the collection asks for, with
// anything it did not mention after them in the order they arrived. A field
// with no value is left out rather than written empty - an empty property is
// noise, and adding it later is the same call as changing it.
func recordFieldOrder(fields map[string]string, order []string) []string {
	out := []string{}
	seen := map[string]bool{}
	take := func(k string) {
		up := strings.ToUpper(strings.TrimSpace(k))
		if up == "" || seen[up] || recordBookkeeping[up] {
			return
		}
		if strings.TrimSpace(fields[k]) == "" && strings.TrimSpace(fields[up]) == "" {
			return
		}
		seen[up] = true
		out = append(out, up)
	}
	for _, k := range order {
		take(k)
	}
	rest := []string{}
	for k := range fields {
		rest = append(rest, k)
	}
	sort.Strings(rest)
	for _, k := range rest {
		take(k)
	}
	// The values are looked up by the uppercase key from here on.
	for _, k := range out {
		if _, ok := fields[k]; !ok {
			for orig, v := range fields {
				if strings.EqualFold(orig, k) {
					fields[k] = v
				}
			}
		}
	}
	return out
}

// AddRecord writes a new record and answers with where it went.
func AddRecord(req *common.RecordNew) (common.Record, error) {
	rtype := strings.TrimSpace(req.Type)
	name := strings.TrimSpace(req.Name)
	if rtype == "" {
		return common.Record{}, fmt.Errorf("a record needs a type")
	}
	if name == "" {
		return common.Record{}, fmt.Errorf("a record needs a name")
	}

	var filename string
	var row, lvl int

	parent := req.TargetHash
	if parent == "" && req.Filename == "" {
		// The collection's own container, which is the whole reason to have
		// one. Without it we need to be told where to put this.
		for _, c := range allCollections() {
			if strings.EqualFold(c.Type, rtype) && c.Hash != "" {
				parent = c.Hash
				break
			}
		}
		if parent == "" {
			return common.Record{}, fmt.Errorf("no %q collection yet - make one, or say which file to add to", rtype)
		}
	}

	var lines []string
	if parent != "" {
		sec := GetDb().FindByHash(parent)
		if sec == nil || sec.Headline == nil {
			return common.Record{}, fmt.Errorf("no heading with that hash")
		}
		f := GetDb().FileFromSection(sec)
		if f == nil || f.Doc == nil {
			return common.Record{}, fmt.Errorf("could not find the file that heading is in")
		}
		filename = f.Doc.Path
		lvl = sec.Headline.Lvl + 1
		var err error
		lines, err = ganttFileLines(filename)
		if err != nil {
			return common.Record{}, err
		}
		row = subtreeEndRow(lines, sec.Headline.Pos.Row, sec.Headline.Lvl, sec.Headline.Pos.Row)
	} else {
		f := GetDb().FindByFile(req.Filename)
		if f == nil || f.Doc == nil {
			return common.Record{}, fmt.Errorf("no file called %q", req.Filename)
		}
		filename = f.Doc.Path
		lvl = 1
		var err error
		lines, err = ganttFileLines(filename)
		if err != nil {
			return common.Record{}, err
		}
		row = len(lines) - 1
	}

	order := []string{}
	for _, c := range allCollections() {
		if strings.EqualFold(c.Type, rtype) {
			order = c.Fields
			break
		}
	}
	fields := map[string]string{}
	for k, v := range req.Fields {
		fields[strings.ToUpper(strings.TrimSpace(k))] = v
	}
	add := spaceBefore(lines, row, recordHeadingLines(lvl, name, rtype, fields, order, req.Notes, req.Tags))
	if _, err := insertLinesAt(filename, row, add); err != nil {
		return common.Record{}, err
	}
	return common.Record{
		Type: rtype, Name: name, Filename: filename, LineNum: row + 1,
	}, nil
}

// UpdateRecord changes a record in place.
//
// Every write here is a line edit rather than a document rewrite. A record
// usually shares its file with a hundred others and with whatever else is in
// there, and writing the parsed document back would reformat all of it to
// change one phone number.
func UpdateRecord(req *common.RecordUpdate) (common.Result, error) {
	sec := GetDb().FindByHash(req.Hash)
	if sec == nil || sec.Headline == nil {
		return common.Result{Ok: false}, fmt.Errorf("no heading with that hash")
	}
	f := GetDb().FileFromSection(sec)
	if f == nil || f.Doc == nil {
		return common.Result{Ok: false}, fmt.Errorf("could not find the file that heading is in")
	}
	if RecordTypeOf(sec) == "" {
		return common.Result{Ok: false}, fmt.Errorf("that heading is not a record")
	}
	filename := f.Doc.Path
	lines, from, to, lok := recordLines(filename, sec)
	if !lok {
		return common.Result{Ok: false}, fmt.Errorf("could not read %s", filename)
	}
	lvl := sec.Headline.Lvl
	ind := indentOf(lvl)
	now := time.Now()
	changed := []string{}

	// --- the name -----------------------------------------------------------
	if req.SetName && strings.TrimSpace(req.Name) != "" && strings.TrimSpace(req.Name) != sectionTitle(sec) {
		lines[from] = replaceHeadlineTitle(lines[from], strings.TrimSpace(req.Name))
		lines = prependLogbook(lines, from, to, ind, "Renamed", now, sectionTitle(sec))
		// Every edit below works on rows that may have just moved.
		lines, from, to = reread(lines, from, lvl)
	}

	// --- the fields ---------------------------------------------------------
	if len(req.Set) > 0 {
		ps, pe, has := drawerAt(lines, from, to, "PROPERTIES")
		if !has {
			// A record whose drawer was deleted by hand still has to be
			// writable; put one back directly under the headline, past any
			// planning line, which is where org keeps it.
			at := from + 1
			for at <= to && at < len(lines) && planningRe.MatchString(lines[at]) {
				at++
			}
			lines = splice(lines, at, []string{ind + ":PROPERTIES:", ind + ":END:"})
			to += 2
			ps, pe = at, at+1
		}
		keys := []string{}
		for k := range req.Set {
			keys = append(keys, strings.ToUpper(strings.TrimSpace(k)))
		}
		sort.Strings(keys)
		for _, key := range keys {
			var val string
			for k, v := range req.Set {
				if strings.EqualFold(k, key) {
					val = strings.TrimSpace(v)
				}
			}
			if recordBookkeeping[key] && key != "ID" {
				// RECORD is the record's identity and ADDED is when it
				// started; neither is somebody's to edit through here.
				continue
			}
			at := -1
			for i := ps + 1; i < pe; i++ {
				if m := propLineRe.FindStringSubmatch(lines[i]); m != nil && strings.EqualFold(m[2], key) {
					at = i
					break
				}
			}
			switch {
			case at >= 0 && val == "":
				lines = append(lines[:at], lines[at+1:]...)
				pe--
				to--
			case at >= 0:
				m := propLineRe.FindStringSubmatch(lines[at])
				lines[at] = m[1] + ":" + m[2] + ":" + m[3] + val
			case val != "":
				lines = splice(lines, pe, []string{propLine(ind, key, val, 0)})
				pe++
				to++
			default:
				continue
			}
			changed = append(changed, key)
		}
		if len(changed) > 0 {
			alignDrawer(lines, ps, pe)
			lines = prependLogbook(lines, from, to, ind, "Updated", now, strings.Join(changed, ", "))
			lines, from, to = reread(lines, from, lvl)
		}
	}

	// --- the notes ----------------------------------------------------------
	if req.SetNotes {
		start := headEnd(lines, from, to)
		body := []string{}
		if text := strings.TrimRight(req.Notes, " \t\n"); text != "" {
			body = append(body, "")
			for _, l := range strings.Split(text, "\n") {
				if strings.TrimSpace(l) == "" {
					body = append(body, "")
				} else {
					body = append(body, ind+l)
				}
			}
			body = append(body, "")
		} else {
			body = append(body, "")
		}
		tail := append([]string{}, lines[to+1:]...)
		lines = append(append(append([]string{}, lines[:start]...), body...), tail...)
		to = start + len(body) - 1
		lines = prependLogbook(lines, from, to, ind, "Noted", now, "")
		lines, from, to = reread(lines, from, lvl)
	}

	if err := writeLines(filename, lines); err != nil {
		return common.Result{Ok: false}, err
	}
	return common.Result{Ok: true}, nil
}

// reread finds the record's range again after an edit changed how many lines
// it has. Cheaper and far safer than trying to keep every index in step.
func reread(lines []string, from, lvl int) ([]string, int, int) {
	to := subtreeEndRow(lines, from, lvl, from)
	if to >= len(lines) {
		to = len(lines) - 1
	}
	return lines, from, to
}

func splice(lines []string, at int, add []string) []string {
	if at < 0 {
		at = 0
	}
	if at > len(lines) {
		at = len(lines)
	}
	out := make([]string, 0, len(lines)+len(add))
	out = append(out, lines[:at]...)
	out = append(out, add...)
	out = append(out, lines[at:]...)
	return out
}

func writeLines(filename string, lines []string) error {
	body := strings.Join(lines, "\n")
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	if err := os.WriteFile(filename, []byte(body), 0644); err != nil {
		return err
	}
	// Read the file back into the database before answering, so a client
	// that asks again straight after the write sees it. Left to the file
	// watcher, a rename followed by a read of the column view raced it and
	// drew the old title over a file that already had the new one.
	if db := odb; db != nil && db.FindByFile(filename) != nil {
		db.ReloadFile(filename)
	}
	return nil
}

// prependLogbook writes one history line, newest first the way org writes a
// logbook, making the drawer when there is not one yet.
func prependLogbook(lines []string, from, to int, ind, kind string, when time.Time, what string) []string {
	entry := ind + "- " + kind + " " + orgStamp(when)
	if what = strings.TrimSpace(what); what != "" {
		entry += " " + what
	}
	if start, _, ok := drawerAt(lines, from, to, "LOGBOOK"); ok {
		return splice(lines, start+1, []string{entry})
	}
	// After the property drawer when there is one, so the two drawers sit
	// together under the headline the way org keeps them.
	at := from + 1
	if _, pe, ok := drawerAt(lines, from, to, "PROPERTIES"); ok {
		at = pe + 1
	} else {
		for at <= to && at < len(lines) && planningRe.MatchString(lines[at]) {
			at++
		}
	}
	return splice(lines, at, []string{ind + ":LOGBOOK:", entry, ind + ":END:"})
}

// A headline: stars, an optional keyword, an optional priority, the title, and
// optional tags. Only the title is replaced - everything a heading says about
// itself other than its name is somebody else's to change.
var headlineRe = regexp.MustCompile(`^(\*+\s+)((?:[A-Z][A-Z0-9_@-]*\s+)?)((?:\[#[A-Za-z0-9]\]\s+)?)(.*?)(\s*:[^\s:]+(?::[^\s:]+)*:)?\s*$`)

func replaceHeadlineTitle(line, title string) string {
	m := headlineRe.FindStringSubmatch(line)
	if m == nil {
		return line
	}
	out := m[1] + m[2] + m[3] + title
	if m[5] != "" {
		out += "  " + strings.TrimSpace(m[5])
	}
	return out
}

// UpdateCollection changes a collection's own definition.
//
// Renaming the *type* is the interesting one: the type is what every record in
// the collection says it is, so renaming it means rewriting the RECORD
// property on all of them. Anything else would leave the records behind under
// a name nothing uses any more, which is not what "rename this collection"
// means to anybody.
//
// No history line is written on the records that move. The change happened to
// the collection, not to the thing each record stands for, and a hundred
// identical logbook lines is noise rather than history.
func UpdateCollection(req *common.RecordCollectionUpdate) (common.RecordCollectionUpdateResult, error) {
	res := common.RecordCollectionUpdateResult{}
	sec := GetDb().FindByHash(req.Hash)
	if sec == nil || sec.Headline == nil {
		return res, fmt.Errorf("no heading with that hash")
	}
	oldType := CollectionTypeOf(sec)
	if oldType == "" {
		return res, fmt.Errorf("that heading is not a collection")
	}
	f := GetDb().FileFromSection(sec)
	if f == nil || f.Doc == nil {
		return res, fmt.Errorf("could not find the file that heading is in")
	}
	newType := oldType
	if req.SetType {
		newType = strings.TrimSpace(strings.ToLower(req.Type))
		if newType == "" {
			return res, fmt.Errorf("a collection needs a type")
		}
		if !strings.EqualFold(newType, oldType) {
			for _, c := range allCollections() {
				if strings.EqualFold(c.Type, newType) && c.Hash != "" {
					return res, fmt.Errorf("there is already a %q collection in %s", newType, c.Filename)
				}
			}
		}
	}

	filename := f.Doc.Path
	lines, from, to, lok := recordLines(filename, sec)
	if !lok {
		return res, fmt.Errorf("could not read %s", filename)
	}
	ind := indentOf(sec.Headline.Lvl)

	if req.SetName && strings.TrimSpace(req.Name) != "" {
		lines[from] = replaceHeadlineTitle(lines[from], strings.TrimSpace(req.Name))
	}

	set := map[string]string{}
	if req.SetType {
		set[CollectionProp] = newType
	}
	if req.SetIcon {
		set["ICON"] = strings.TrimSpace(req.Icon)
	}
	if req.SetFields {
		clean := []string{}
		for _, k := range req.Fields {
			if k = strings.ToUpper(strings.TrimSpace(k)); k != "" {
				clean = append(clean, k)
			}
		}
		set["FIELDS"] = strings.Join(clean, ", ")
	}
	if len(set) > 0 {
		var err error
		if lines, err = setPropsIn(lines, from, to, ind, set); err != nil {
			return res, err
		}
	}
	if err := writeLines(filename, lines); err != nil {
		return res, err
	}

	// The records follow the type. Done after the container is written, so a
	// failure part way through leaves a collection that at least names itself.
	if req.SetType && !strings.EqualFold(newType, oldType) {
		moved, err := retypeRecords(oldType, newType)
		res.Moved = moved
		if err != nil {
			res.Type = newType
			return res, err
		}
	}
	res.Ok = true
	res.Type = newType
	return res, nil
}

// setPropsIn writes a set of properties into a heading's drawer, making the
// drawer when there is not one. The same rules the record update follows: an
// empty value takes the property off, and the drawer is re-aligned afterwards.
func setPropsIn(lines []string, from, to int, ind string, set map[string]string) ([]string, error) {
	ps, pe, has := drawerAt(lines, from, to, "PROPERTIES")
	if !has {
		at := from + 1
		for at <= to && at < len(lines) && planningRe.MatchString(lines[at]) {
			at++
		}
		lines = splice(lines, at, []string{ind + ":PROPERTIES:", ind + ":END:"})
		ps, pe = at, at+1
	}
	keys := []string{}
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		val := strings.TrimSpace(set[key])
		at := -1
		for i := ps + 1; i < pe; i++ {
			if m := propLineRe.FindStringSubmatch(lines[i]); m != nil && strings.EqualFold(m[2], key) {
				at = i
				break
			}
		}
		switch {
		case at >= 0 && val == "":
			lines = append(lines[:at], lines[at+1:]...)
			pe--
		case at >= 0:
			m := propLineRe.FindStringSubmatch(lines[at])
			lines[at] = m[1] + ":" + m[2] + ":" + m[3] + val
		case val != "":
			lines = splice(lines, pe, []string{propLine(ind, key, val, 0)})
			pe++
		}
	}
	alignDrawer(lines, ps, pe)
	return lines, nil
}

// retypeRecords rewrites the RECORD property of every record of one type.
//
// Grouped by file and applied from the bottom of each file up, so that one
// file is read once, written once, and every row still means what it meant
// when it was worked out - editing a record part way down a file moves nothing
// above it.
func retypeRecords(oldType, newType string) (int, error) {
	byFile := map[string][]*org.Section{}
	db := GetDb()
	for _, rec := range allRecords(oldType, false) {
		sec, ok := db.ByHash[rec.Hash]
		if !ok || sec == nil || sec.Headline == nil || rec.Filename == "" {
			continue
		}
		byFile[rec.Filename] = append(byFile[rec.Filename], sec)
	}

	moved := 0
	for filename, secs := range byFile {
		lines, err := ganttFileLines(filename)
		if err != nil {
			return moved, err
		}
		sort.SliceStable(secs, func(a, b int) bool {
			return secs[a].Headline.Pos.Row > secs[b].Headline.Pos.Row
		})
		touched := false
		for _, sec := range secs {
			from := sec.Headline.Pos.Row
			if from < 0 || from >= len(lines) {
				continue
			}
			to := subtreeEndRow(lines, from, sec.Headline.Lvl, from)
			if to >= len(lines) {
				to = len(lines) - 1
			}
			ps, pe, ok := drawerAt(lines, from, to, "PROPERTIES")
			if !ok {
				continue
			}
			for i := ps + 1; i < pe; i++ {
				m := propLineRe.FindStringSubmatch(lines[i])
				if m != nil && strings.EqualFold(m[2], RecordProp) {
					lines[i] = m[1] + ":" + m[2] + ":" + m[3] + newType
					touched = true
					moved++
					break
				}
			}
		}
		if touched {
			if err := writeLines(filename, lines); err != nil {
				return moved, err
			}
		}
	}
	return moved, nil
}

// AddCollection writes a container heading for a collection.
func AddCollection(req *common.RecordCollectionNew) (common.RecordCollection, error) {
	rtype := strings.TrimSpace(req.Type)
	if rtype == "" {
		return common.RecordCollection{}, fmt.Errorf("a collection needs a type")
	}
	for _, c := range allCollections() {
		if strings.EqualFold(c.Type, rtype) && c.Hash != "" {
			return common.RecordCollection{}, fmt.Errorf("there is already a %q collection in %s", rtype, c.Filename)
		}
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = titleOf(rtype)
	}

	var filename string
	var row, lvl int
	var lines []string
	if req.ParentHash != "" {
		sec := GetDb().FindByHash(req.ParentHash)
		if sec == nil || sec.Headline == nil {
			return common.RecordCollection{}, fmt.Errorf("no heading with that hash")
		}
		f := GetDb().FileFromSection(sec)
		if f == nil || f.Doc == nil {
			return common.RecordCollection{}, fmt.Errorf("could not find the file that heading is in")
		}
		filename = f.Doc.Path
		lvl = sec.Headline.Lvl + 1
		var err error
		lines, err = ganttFileLines(filename)
		if err != nil {
			return common.RecordCollection{}, err
		}
		row = subtreeEndRow(lines, sec.Headline.Pos.Row, sec.Headline.Lvl, sec.Headline.Pos.Row)
	} else {
		f := GetDb().FindByFile(req.Filename)
		if f == nil || f.Doc == nil {
			return common.RecordCollection{}, fmt.Errorf("no file called %q", req.Filename)
		}
		filename = f.Doc.Path
		lvl = 1
		var err error
		lines, err = ganttFileLines(filename)
		if err != nil {
			return common.RecordCollection{}, err
		}
		row = len(lines) - 1
	}

	ind := indentOf(lvl)
	w := propWidth([]string{CollectionProp, "ICON", "FIELDS"})
	add := []string{
		strings.Repeat("*", lvl) + " " + name,
		ind + ":PROPERTIES:",
		propLine(ind, CollectionProp, rtype, w),
	}
	if icon := strings.TrimSpace(req.Icon); icon != "" {
		add = append(add, propLine(ind, "ICON", icon, w))
	}
	if len(req.Fields) > 0 {
		add = append(add, propLine(ind, "FIELDS", strings.Join(req.Fields, ", "), w))
	}
	add = append(add, ind+":END:", "")
	add = spaceBefore(lines, row, add)

	if _, err := insertLinesAt(filename, row, add); err != nil {
		return common.RecordCollection{}, err
	}
	return common.RecordCollection{
		Type: rtype, Name: name, Icon: req.Icon, Fields: req.Fields,
		Filename: filename, LineNum: row + 1,
	}, nil
}

// ----------------------------------------------------------------------------
// Birthdays
// ----------------------------------------------------------------------------

var (
	isoDateRe    = regexp.MustCompile(`(\d{4})-(\d{2})-(\d{2})`)
	noYearDateRe = regexp.MustCompile(`^-{1,2}(\d{2})-(\d{2})$`)
	slashDateRe  = regexp.MustCompile(`^(\d{1,2})/(\d{1,2})/(\d{4})$`)
)

// parseBirthday reads the day a yearly thing started. Three ways of writing
// one are taken: the ISO date in the example, an org timestamp (which contains
// one), and vCard's year-less "--05-15" for a birthday whose year nobody
// knows. A year of zero means "not given", which is what makes the age
// optional rather than wrong.
func parseBirthday(v string) (month, day, year int, ok bool) {
	s := strings.TrimSpace(v)
	if s == "" {
		return 0, 0, 0, false
	}
	if m := noYearDateRe.FindStringSubmatch(s); m != nil {
		mo, _ := strconv.Atoi(m[1])
		d, _ := strconv.Atoi(m[2])
		return mo, d, 0, validMonthDay(mo, d)
	}
	if m := isoDateRe.FindStringSubmatch(s); m != nil {
		y, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		d, _ := strconv.Atoi(m[3])
		return mo, d, y, validMonthDay(mo, d)
	}
	if m := slashDateRe.FindStringSubmatch(s); m != nil {
		mo, _ := strconv.Atoi(m[1])
		d, _ := strconv.Atoi(m[2])
		y, _ := strconv.Atoi(m[3])
		return mo, d, y, validMonthDay(mo, d)
	}
	return 0, 0, 0, false
}

func validMonthDay(m, d int) bool {
	return m >= 1 && m <= 12 && d >= 1 && d <= 31
}

// Birthdays finds every birthday that falls between two dates.
//
// A birthday is a yearly thing and a record stores the day it started, so the
// occurrence is worked out here rather than being left to a client to do
// calendar arithmetic on - which is also what makes the same answer serve the
// agenda, the CLI and anything else that asks.
func Birthdays(from, to time.Time) []common.BirthdayEvent {
	out := []common.BirthdayEvent{}
	if to.Before(from) {
		from, to = to, from
	}
	for _, rec := range allRecords("", false) {
		for _, f := range rec.Fields {
			if !birthdayFields[f.Name] {
				continue
			}
			mo, d, y, ok := parseBirthday(f.Value)
			if !ok {
				continue
			}
			for year := from.Year(); year <= to.Year(); year++ {
				when := time.Date(year, time.Month(mo), d, 0, 0, 0, 0, from.Location())
				// A 29 February in a year that has none rolls into March;
				// pull it back to the 28th, which is where it is kept.
				if when.Month() != time.Month(mo) {
					when = time.Date(year, time.Month(mo), 1, 0, 0, 0, 0, from.Location()).
						AddDate(0, 1, -1)
				}
				if when.Before(from) || when.After(to) {
					continue
				}
				age := 0
				if y > 0 && year >= y {
					age = year - y
				}
				out = append(out, common.BirthdayEvent{
					Hash: rec.Hash, Name: rec.Name, Type: rec.Type,
					Filename: rec.Filename, LineNum: rec.LineNum,
					Date:     when.Format("2006-01-02"),
					Birthday: f.Value, Age: age, Field: f.Name, Image: rec.Image,
				})
			}
		}
	}
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].Date != out[b].Date {
			return out[a].Date < out[b].Date
		}
		return strings.ToLower(out[a].Name) < strings.ToLower(out[b].Name)
	})
	return out
}

// ----------------------------------------------------------------------------
// The fields a collection actually uses
// ----------------------------------------------------------------------------

// RecordFieldsInUse answers "what does a record of this type usually carry?",
// which is what an add form offers before anybody has typed anything. The
// collection's own FIELDS come first and in its order; everything else follows
// by how many records use it.
func RecordFieldsInUse(rtype string) []common.RecordFieldUse {
	counts := map[string]int{}
	order := []string{}
	for _, rec := range allRecords(rtype, false) {
		for _, f := range rec.Fields {
			if _, seen := counts[f.Key]; !seen {
				order = append(order, f.Key)
			}
			counts[f.Key]++
		}
	}
	declared := []string{}
	for _, c := range allCollections() {
		if strings.EqualFold(c.Type, rtype) {
			declared = c.Fields
			break
		}
	}
	rank := map[string]int{}
	for i, k := range declared {
		rank[strings.ToUpper(k)] = i
		if _, seen := counts[strings.ToUpper(k)]; !seen {
			counts[strings.ToUpper(k)] = 0
			order = append(order, strings.ToUpper(k))
		}
	}
	out := []common.RecordFieldUse{}
	for _, k := range order {
		name, label, kind := fieldOf(k)
		out = append(out, common.RecordFieldUse{
			Key: k, Name: name, Label: label, Kind: kind, Count: counts[k],
		})
	}
	sort.SliceStable(out, func(a, b int) bool {
		ra, oka := rank[out[a].Key]
		rb, okb := rank[out[b].Key]
		if oka != okb {
			return oka
		}
		if oka && okb {
			return ra < rb
		}
		if out[a].Count != out[b].Count {
			return out[a].Count > out[b].Count
		}
		return out[a].Key < out[b].Key
	})
	return out
}

// ----------------------------------------------------------------------------
// REST
// ----------------------------------------------------------------------------

func recordJson(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func recordErr(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: err.Error()})
}

/* SDOC: API
* GET /records — Find Records

	Answers with every record, or with the records of one collection.

	| Parameter | Meaning                                                      |
	|-----------+--------------------------------------------------------------|
	| type      | Only this collection: =contact=, =equipment=. All when empty |
	| q         | Only records matching this text anywhere in them             |
	| body      | =1= to include the notes and the history                     |

	A record's fields come back parsed - name, label, kind and an actionable
	link - so a client draws a contact and a guitar pedal with the same code.

EDOC */
func RequestRecords(w http.ResponseWriter, r *http.Request) {
	rtype := r.URL.Query().Get("type")
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	withBody := r.URL.Query().Get("body") == "1" || q != ""
	recs := allRecords(rtype, withBody)
	out := []common.Record{}
	for _, rec := range recs {
		if recordMatches(rec, q) {
			out = append(out, *rec)
		}
	}
	recordJson(w, out)
}

/* SDOC: API
* GET /record/{hash} — One Record

	The record that heading is, with its notes and its update history. Answers
	with an error rather than an empty record when the heading is not one.

EDOC */
func RequestRecord(w http.ResponseWriter, r *http.Request) {
	hash, err := GetHash(mux.Vars(r), "hash")
	if err != nil {
		recordErr(w, err)
		return
	}
	// FindByHash walks every file into the registry on a miss, which is what
	// a heading nothing has walked yet needs.
	sec := GetDb().FindByHash(hash)
	if sec == nil {
		recordErr(w, fmt.Errorf("no heading with that hash"))
		return
	}
	rec := readRecord(sec, GetDb().FileFromSection(sec), true)
	if rec == nil {
		recordErr(w, fmt.Errorf("that heading is not a record"))
		return
	}
	recordJson(w, rec)
}

/* SDOC: API
* GET /records/collections — What Collections Exist

	Every collection the database knows about: the ones with a container
	heading and the ones that exist only because records carry the type, with
	how many records each holds and where new ones would be filed.

EDOC */
func RequestCollections(w http.ResponseWriter, r *http.Request) {
	recordJson(w, allCollections())
}

/* SDOC: API
* GET /records/fields — The Fields A Collection Uses

	=?type=contact= answers with the field names records of that type carry,
	the collection's own FIELDS first and the rest by how many records use
	them. This is what an add form offers before anything has been typed.

EDOC */
func RequestRecordFields(w http.ResponseWriter, r *http.Request) {
	recordJson(w, RecordFieldsInUse(r.URL.Query().Get("type")))
}

/* SDOC: API
* POST /record — Add A Record

	#+BEGIN_EXAMPLE
	{ "Type": "contact", "Name": "Jane Roe",
	  "Fields": { "EMAIL": "jane@example.com", "PHONE_MOBILE": "+1-555-0100" },
	  "Notes": "Met at the fair." }
	#+END_EXAMPLE

	Files it under the collection's container heading. =TargetHash= files it
	under a heading of your choosing instead, and =Filename= appends it to the
	end of a file - which is how the first record of a collection with no
	container yet gets written.

EDOC */
func PostRecord(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req common.RecordNew
	if err := json.Unmarshal(body, &req); err != nil {
		recordErr(w, err)
		return
	}
	rec, err := AddRecord(&req)
	if err != nil {
		recordErr(w, err)
		return
	}
	recordJson(w, rec)
}

/* SDOC: API
* POST /record/update — Change A Record

	#+BEGIN_EXAMPLE
	{ "Hash": "...", "Set": { "PHONE_MOBILE": "+1-555-0101", "FAX": "" } }
	#+END_EXAMPLE

	Every field named in =Set= is written and a field set to an empty string is
	taken off, the same rule the property endpoint follows. =SetName= with
	=Name= renames the heading and =SetNotes= with =Notes= replaces the body.

	Every change prepends a line to the record's LOGBOOK, so what was touched
	and when is on the record itself rather than in a log somewhere else.

EDOC */
func PostRecordUpdate(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req common.RecordUpdate
	if err := json.Unmarshal(body, &req); err != nil {
		recordErr(w, err)
		return
	}
	res, err := UpdateRecord(&req)
	if err != nil {
		recordErr(w, err)
		return
	}
	recordJson(w, res)
}

/* SDOC: API
* POST /records/collection — Start A Collection

	#+BEGIN_EXAMPLE
	{ "Type": "synth", "Name": "Synthesisers", "Icon": "🎹",
	  "Fields": ["MAKER", "MODEL", "YEAR", "SERIAL", "IMAGE"],
	  "Filename": "things.org" }
	#+END_EXAMPLE

	Writes the container heading new records of that type are filed under.
	Refuses when that collection already has one, because a second container
	would quietly become the place things land.

EDOC */
func PostCollection(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req common.RecordCollectionNew
	if err := json.Unmarshal(body, &req); err != nil {
		recordErr(w, err)
		return
	}
	c, err := AddCollection(&req)
	if err != nil {
		recordErr(w, err)
		return
	}
	recordJson(w, c)
}

/* SDOC: API
* POST /records/collection/update — Change A Collection

	#+BEGIN_EXAMPLE
	{ "Hash": "...", "SetName": true, "Name": "Synthesisers",
	  "SetIcon": true, "Icon": "🎹",
	  "SetFields": true, "Fields": ["MAKER", "MODEL", "YEAR"] }
	#+END_EXAMPLE

	Each part is only written when its flag is set, because clearing the icon
	and leaving it alone are different things and "" says both.

	=SetType= with a new =Type= renames the collection, which means rewriting
	the =RECORD= property of every record that carried the old name - otherwise
	the records would be left behind under a name nothing uses. The answer says
	how many moved. It refuses when another collection already answers to the
	new name.
EDOC */
func PostCollectionUpdate(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req common.RecordCollectionUpdate
	if err := json.Unmarshal(body, &req); err != nil {
		recordErr(w, err)
		return
	}
	res, err := UpdateCollection(&req)
	if err != nil {
		if res.Moved > 0 {
			// Part of it happened; say what, rather than only that it failed.
			res.Msg = err.Error()
			recordJson(w, res)
			return
		}
		recordErr(w, err)
		return
	}
	if res.Moved > 0 {
		res.Msg = fmt.Sprintf("renamed, and %d record(s) came with it", res.Moved)
	}
	recordJson(w, res)
}

/* SDOC: API
* GET /records/birthdays — Birthdays In A Window

	=?from=2026-09-01&to=2026-09-30= answers with every birthday and
	anniversary falling in those dates, worked out from the day each record
	says the thing started. Both are optional: the default window is the next
	year from today.

	The occurrence is worked out here rather than in the client, so the agenda,
	the CLI and anything else asking get the same answer - including the 29th
	of February, which is kept on the 28th in a year that has no 29th.

EDOC */
func RequestBirthdays(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	to := from.AddDate(1, 0, 0)
	if v := r.URL.Query().Get("from"); v != "" {
		if t, err := time.ParseInLocation("2006-01-02", v, now.Location()); err == nil {
			from = t
		}
	}
	if v := r.URL.Query().Get("to"); v != "" {
		if t, err := time.ParseInLocation("2006-01-02", v, now.Location()); err == nil {
			to = t
		}
	}
	recordJson(w, Birthdays(from, to))
}

// --- the contact conveniences ----------------------------------------------
//
// Contacts are records of type "contact" and nothing else, so these are the
// record endpoints with the type filled in. They exist because "the contacts
// api" is a thing people look for, and because a client that only ever deals
// with people should not have to remember to say so on every call.

/* SDOC: API
* GET /contacts — Find Contacts

	=/records?type=contact=, with =q= searching names, numbers, addresses and
	notes alike.

EDOC */
func RequestContacts(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	out := []common.Record{}
	for _, rec := range allRecords("contact", true) {
		if recordMatches(rec, q) {
			out = append(out, *rec)
		}
	}
	recordJson(w, out)
}

/* SDOC: API
* POST /contact — Add A Contact

	=/record= with the type filled in as =contact=.

EDOC */
func PostContact(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req common.RecordNew
	if err := json.Unmarshal(body, &req); err != nil {
		recordErr(w, err)
		return
	}
	req.Type = "contact"
	rec, err := AddRecord(&req)
	if err != nil {
		recordErr(w, err)
		return
	}
	recordJson(w, rec)
}
