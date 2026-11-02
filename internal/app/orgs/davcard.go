package orgs

// The address book a phone sees (dav.go says who may see it).
//
// A contact is a contact record (:RECORD: contact) or any heading with an
// EMAIL or PHONE property (EMAIL_WORK, PHONE_MOBILE ... count), or what
// `dav.contactQuery` finds. Its card is written from its fields by the rules
// the vCard import reads them by (vcard.go), so a card read back gives the
// same fields: a phone that saves a contact unchanged changes nothing.
//
// From the phone:
//
//   - **An edit** is compared by value, not by key (`hasValue`, the import's
//     rule: a phone number with or without its country code is the same
//     number). A value gone from the card comes off the heading, a new one is
//     added under the key the import would give it, a changed name renames
//     the heading. Fields the card cannot say (anything that is not a phone,
//     address, date, url, company...) are never touched, and neither is a
//     picture. A record is changed through UpdateRecord (line edits, a
//     history line "from CardDAV"); a plain heading property by property.
//   - **A new contact** is added to the contact collection (AddRecord), or to
//     contacts.org in the first org directory when there is none.
//   - **Deleting a contact** is refused. A person is removed in org.

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-vcard"
	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/carddav"
	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs"
	"github.com/ihdavids/orgs/internal/common"
)

type davContact struct {
	Name     string // resource file name
	File     string
	Hash     string
	Record   bool
	Uid      string
	madeUid  bool
	Title    string
	Props    map[string]string
	Keys     []string
	Emitted  []string
	Notes    string
	Tags     []string
	Text     string
	ETag     string
	Mod      time.Time
}

var davCardCache struct {
	mu       sync.Mutex
	index    uint64
	contacts []davContact
}

func davIsContact(sec *org.Section) bool {
	if RecordTypeOf(sec) == "contact" {
		return true
	}
	keys, _ := sectionProps(sec)
	for _, k := range keys {
		n, _, _ := fieldOf(k)
		if n == "EMAIL" || n == "PHONE" {
			return true
		}
	}
	return false
}

func davContacts(fresh bool) ([]davContact, error) {
	db := GetDb()
	davCardCache.mu.Lock()
	defer davCardCache.mu.Unlock()
	if !fresh && davCardCache.contacts != nil && davCardCache.index == db.ReloadIndex {
		return davCardCache.contacts, nil
	}
	idx := db.ReloadIndex
	secs := []*org.Section{}
	// The file each heading is in, by the walk that found it: the section
	// registry fills lazily (see Traps: hashes), so FileFromSection can miss.
	fileOf := map[*org.Section]*common.OrgFile{}
	if q := strings.TrimSpace(Conf().Server.Dav.ContactQuery); q != "" {
		found, err := plugs.Headings(&Db{}, q)
		if err != nil {
			return nil, err
		}
		secs = found
	} else {
		for _, name := range db.GetFiles() {
			f := db.GetFile(name)
			if f == nil || f.Doc == nil {
				continue
			}
			var walk func([]*org.Section)
			walk = func(ss []*org.Section) {
				for _, s := range ss {
					if s.Headline != nil && !s.Headline.IsArchived() && davIsContact(s) {
						secs = append(secs, s)
						fileOf[s] = f
					}
					walk(s.Children)
				}
			}
			walk(f.Doc.Outline.Children)
		}
	}
	out := []davContact{}
	seen := map[string]bool{}
	for _, sec := range secs {
		if sec.Headline == nil {
			continue
		}
		f := fileOf[sec]
		if f == nil {
			f = db.FileFromSection(sec)
		}
		if f == nil {
			continue
		}
		c := davContact{File: f.Filename, Hash: sec.Hash, Title: sectionTitle(sec), Tags: sec.Headline.Tags}
		c.Keys, c.Props = sectionProps(sec)
		if rec := readRecord(sec, f, true); rec != nil {
			c.Record, c.Notes = true, strings.TrimSpace(rec.Notes)
		}
		c.Uid = firstOf(c.Props["VCARD_UID"], c.Props["ID"])
		if c.Uid == "" {
			olp := []string{}
			for p := sec; p != nil && p.Headline != nil; p = p.Parent {
				olp = append([]string{sectionTitle(p)}, olp...)
			}
			sum := sha1.Sum([]byte(filepath.Base(f.Filename) + "\x00" + strings.Join(olp, "\x00")))
			c.Uid, c.madeUid = "orgs-"+hex.EncodeToString(sum[:8]), true
		}
		c.Name = c.Props["DAV_NAME"]
		if c.Name == "" {
			c.Name = davObjectName(c.Uid) + ".vcf"
		}
		if seen[c.Name] {
			continue
		}
		seen[c.Name] = true
		if st, err := os.Stat(f.Filename); err == nil {
			c.Mod = st.ModTime().UTC().Truncate(time.Second)
		}
		c.Text, c.Emitted = davVCard(&c)
		sum := sha1.Sum([]byte(c.Text))
		c.ETag = hex.EncodeToString(sum[:10])
		out = append(out, c)
	}
	davCardCache.contacts, davCardCache.index = out, idx
	return out, nil
}

func firstOf(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// The vCard TYPE a field label is written as, the inverse of typeLabels.
var davTypeOf = map[string]string{
	"MOBILE": "cell", "HOME": "home", "WORK": "work", "OTHER": "other", "MAIN": "main",
	"PAGER": "pager", "CAR": "car", "VIDEO": "video", "SCHOOL": "school", "TEXTPHONE": "textphone",
}

func vcEsc(s string) string {
	r := strings.NewReplacer(`\`, `\\`, "\n", `\n`, ",", `\,`, ";", `\;`)
	return r.Replace(s)
}

// davVCard writes the card, and says which fields it carried: those, and
// only those, are what a card coming back without them takes off.
func davVCard(c *davContact) (string, []string) {
	b := &strings.Builder{}
	line := func(s string) { b.WriteString(s + "\r\n") }
	group := 0
	// typed writes PROP with the label as a TYPE when vCard has one for it,
	// else in Apple's grouped X-ABLabel, which is how a custom label survives.
	typed := func(prop, label, extra, value string) {
		types := []string{}
		if extra != "" {
			types = append(types, extra)
		}
		custom := ""
		for _, part := range strings.Split(label, "_") {
			if part == "" {
				continue
			}
			if t, ok := davTypeOf[part]; ok {
				types = append(types, t)
			} else {
				custom = label
				types = nil
				if extra != "" {
					types = []string{extra}
				}
				break
			}
		}
		head := prop
		if custom != "" {
			group++
			head = fmt.Sprintf("item%d.%s", group, prop)
		}
		if len(types) > 0 {
			head += ";TYPE=" + strings.Join(types, ",")
		}
		line(head + ":" + value)
		if custom != "" {
			line(fmt.Sprintf("item%d.X-ABLabel:%s", group, vcEsc(strings.ReplaceAll(strings.ToLower(custom), "_", " "))))
		}
	}
	line("BEGIN:VCARD")
	line("VERSION:3.0")
	line("PRODID:-//orgs//CardDAV//EN")
	line("UID:" + vcEsc(c.Uid))
	line("FN:" + vcEsc(c.Title))
	words := strings.Fields(c.Title)
	if len(words) > 1 {
		line("N:" + vcEsc(words[len(words)-1]) + ";" + vcEsc(strings.Join(words[:len(words)-1], " ")) + ";;;")
	} else {
		line("N:" + vcEsc(c.Title) + ";;;;")
	}
	emitted := []string{}
	keys := append([]string{}, c.Keys...)
	// The plain EMAIL first and preferred: the import reads the first email
	// as EMAIL.
	sort.SliceStable(keys, func(a, b int) bool { return keys[a] == "EMAIL" && keys[b] != "EMAIL" })
	company := ""
	for _, k := range keys {
		v := strings.TrimSpace(c.Props[k])
		if v == "" {
			continue
		}
		name, label, kind := fieldOf(k)
		used := true
		switch {
		case name == "EMAIL":
			v = strings.TrimPrefix(v, "mailto:")
			if k == "EMAIL" {
				line("EMAIL;TYPE=INTERNET,pref:" + vcEsc(v))
			} else {
				typed("EMAIL", label, "INTERNET", vcEsc(v))
			}
		case name == "PHONE":
			typed("TEL", label, "", vcEsc(strings.TrimPrefix(v, "tel:")))
		case name == "FAX":
			typed("TEL", label, "fax", vcEsc(strings.TrimPrefix(v, "tel:")))
		case name == "ADDRESS":
			typed("ADR", label, "", ";;"+vcEsc(v)+";;;;")
		case name == "BIRTHDAY" && label == "":
			line("BDAY:" + v)
		case name == "ANNIVERSARY" && label == "":
			line("X-ANNIVERSARY:" + v)
		case name == "COMPANY" && label == "":
			company = v
		case name == "DEPARTMENT" && label == "":
		case (name == "TITLE" || name == "ROLE" || name == "NICKNAME") && label == "":
			line(name + ":" + vcEsc(v))
		case name == "WEBSITE":
			typed("URL", label, "", v)
		case kind == "social" && label == "":
			line("X-SOCIALPROFILE;TYPE=" + strings.ToLower(name) + ":" + v)
		case name == "IMAGE" && label == "":
			if p := davPhoto(c.File, v); p != "" {
				line(p)
			}
			used = false // a picture is never taken off from the phone
		default:
			used = false
		}
		if used {
			emitted = append(emitted, k)
		}
	}
	if company != "" || c.Props["DEPARTMENT"] != "" {
		line("ORG:" + vcEsc(company) + ";" + vcEsc(c.Props["DEPARTMENT"]))
		for _, k := range []string{"COMPANY", "DEPARTMENT"} {
			if c.Props[k] != "" {
				emitted = append(emitted, k)
			}
		}
	}
	if c.Record && c.Notes != "" {
		line("NOTE:" + vcEsc(c.Notes))
	}
	if len(c.Tags) > 0 {
		esc := []string{}
		for _, t := range c.Tags {
			esc = append(esc, vcEsc(t))
		}
		line("CATEGORIES:" + strings.Join(esc, ","))
	}
	line("END:VCARD")
	return b.String(), emitted
}

// davPhoto is a PHOTO line for an IMAGE field: a url as a url, a picture
// beside the org file inline (up to 256k; phones do not fetch).
func davPhoto(orgFile, v string) string {
	v = strings.TrimSpace(v)
	if m := txLinkRe.FindStringSubmatch(v); m != nil {
		v = m[1]
	}
	v = strings.TrimPrefix(v, "file:")
	if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
		return "PHOTO;VALUE=uri:" + v
	}
	p := v
	if !filepath.IsAbs(p) {
		p = filepath.Join(filepath.Dir(orgFile), p)
	}
	if _, ok := insideOrgDirs(p); !ok {
		return ""
	}
	st, err := os.Stat(p)
	if err != nil || st.Size() > 256<<10 {
		return ""
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	t := strings.ToUpper(strings.TrimPrefix(filepath.Ext(p), "."))
	if t == "JPG" {
		t = "JPEG"
	}
	if mt := mime.TypeByExtension(filepath.Ext(p)); !strings.HasPrefix(mt, "image/") {
		return ""
	}
	return "PHOTO;ENCODING=b;TYPE=" + t + ":" + base64.StdEncoding.EncodeToString(b)
}

// ---------------------------------------------------------------------------
// The backend
// ---------------------------------------------------------------------------

type cardBackend struct{}

func (b *cardBackend) CurrentUserPrincipal(ctx context.Context) (string, error) {
	return "/dav/card/" + url.PathEscape(davUser(ctx)) + "/", nil
}

func (b *cardBackend) AddressBookHomeSetPath(ctx context.Context) (string, error) {
	return "/dav/card/" + url.PathEscape(davUser(ctx)) + "/contacts/", nil
}

func (b *cardBackend) bookPath(ctx context.Context) string {
	return "/dav/card/" + url.PathEscape(davUser(ctx)) + "/contacts/org/"
}

func (b *cardBackend) book(ctx context.Context) carddav.AddressBook {
	return carddav.AddressBook{Path: b.bookPath(ctx), Name: "Org contacts", Description: "Contacts from orgs",
		MaxResourceSize: 1 << 20}
}

func (b *cardBackend) ListAddressBooks(ctx context.Context) ([]carddav.AddressBook, error) {
	return []carddav.AddressBook{b.book(ctx)}, nil
}

func (b *cardBackend) GetAddressBook(ctx context.Context, p string) (*carddav.AddressBook, error) {
	if strings.TrimSuffix(p, "/")+"/" != b.bookPath(ctx) {
		return nil, webdav.NewHTTPError(http.StatusNotFound, fmt.Errorf("no such address book"))
	}
	ab := b.book(ctx)
	return &ab, nil
}

func (b *cardBackend) CreateAddressBook(ctx context.Context, ab *carddav.AddressBook) error {
	return webdav.NewHTTPError(http.StatusForbidden, fmt.Errorf("orgs has one address book"))
}

func (b *cardBackend) DeleteAddressBook(ctx context.Context, p string) error {
	return webdav.NewHTTPError(http.StatusForbidden, fmt.Errorf("the address book is your org files"))
}

func (b *cardBackend) object(ctx context.Context, c davContact) (carddav.AddressObject, error) {
	card, err := vcard.NewDecoder(strings.NewReader(c.Text)).Decode()
	if err != nil {
		return carddav.AddressObject{}, err
	}
	return carddav.AddressObject{Path: b.bookPath(ctx) + url.PathEscape(c.Name), ModTime: c.Mod,
		ContentLength: int64(len(c.Text)), ETag: c.ETag, Card: card}, nil
}

func (b *cardBackend) find(ctx context.Context, p string, fresh bool) (*davContact, error) {
	dir, name := path.Split(p)
	if dir != b.bookPath(ctx) {
		return nil, webdav.NewHTTPError(http.StatusNotFound, fmt.Errorf("no such address book"))
	}
	if n, err := url.PathUnescape(name); err == nil {
		name = n
	}
	cs, err := davContacts(fresh)
	if err != nil {
		return nil, err
	}
	for i := range cs {
		if cs[i].Name == name {
			return &cs[i], nil
		}
	}
	return nil, nil
}

func (b *cardBackend) GetAddressObject(ctx context.Context, p string, req *carddav.AddressDataRequest) (*carddav.AddressObject, error) {
	c, err := b.find(ctx, p, false)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, webdav.NewHTTPError(http.StatusNotFound, fmt.Errorf("no such contact"))
	}
	o, err := b.object(ctx, *c)
	return &o, err
}

func (b *cardBackend) ListAddressObjects(ctx context.Context, p string, req *carddav.AddressDataRequest) ([]carddav.AddressObject, error) {
	if _, err := b.GetAddressBook(ctx, p); err != nil {
		return nil, err
	}
	cs, err := davContacts(false)
	if err != nil {
		return nil, err
	}
	out := make([]carddav.AddressObject, 0, len(cs))
	for _, c := range cs {
		if o, err := b.object(ctx, c); err == nil {
			out = append(out, o)
		}
	}
	return out, nil
}

func (b *cardBackend) QueryAddressObjects(ctx context.Context, p string, q *carddav.AddressBookQuery) ([]carddav.AddressObject, error) {
	all, err := b.ListAddressObjects(ctx, p, &q.DataRequest)
	if err != nil {
		return nil, err
	}
	// An empty filter matches every card (RFC 6352 10.5); the library's
	// any-of over no filters matches none.
	if len(q.PropFilters) == 0 {
		if q.Limit > 0 && q.Limit < len(all) {
			all = all[:q.Limit]
		}
		return all, nil
	}
	return carddav.Filter(q, all)
}

func (b *cardBackend) DeleteAddressObject(ctx context.Context, p string) error {
	return webdav.NewHTTPError(http.StatusForbidden, fmt.Errorf("contacts are removed in org, not from the phone"))
}

// davCardChanges is what a card coming back changes: the fields to set (""
// takes one off), and the name.
func davCardChanges(c *davContact, in *vcContact) (map[string]string, string) {
	set := map[string]string{}
	props := map[string]string{}
	for k, v := range c.Props {
		props[k] = v
	}
	for _, k := range c.Emitted {
		if !hasValue(in.Fields, k, props[k]) {
			set[k] = ""
			delete(props, k)
		}
	}
	for _, k := range in.Keys {
		v := in.Fields[k]
		if k == vcUidField || k == "IMAGE" || strings.TrimSpace(v) == "" {
			continue
		}
		if hasValue(props, k, v) {
			continue
		}
		key := freeKey(props, k)
		props[key], set[key] = v, v
	}
	name := ""
	if in.Name != "" && in.Name != c.Title {
		name = in.Name
	}
	return set, name
}

func (b *cardBackend) PutAddressObject(ctx context.Context, p string, card vcard.Card, opts *carddav.PutAddressObjectOptions) (*carddav.AddressObject, error) {
	have, err := b.find(ctx, p, true)
	if err != nil {
		return nil, err
	}
	etag := ""
	var asEvent *davEvent
	if have != nil {
		etag = have.ETag
		asEvent = &davEvent{}
	}
	if err := davPrecondition(asEvent, etag, opts.IfMatch, opts.IfNoneMatch); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := vcard.NewEncoder(&buf).Encode(card); err != nil {
		return nil, webdav.NewHTTPError(http.StatusBadRequest, err)
	}
	got := contactsFromVCard(buf.String())
	if len(got) == 0 {
		return nil, webdav.NewHTTPError(http.StatusBadRequest, fmt.Errorf("no card in that"))
	}
	in := got[0]
	_, name := path.Split(p)
	if n, err := url.PathUnescape(name); err == nil {
		name = n
	}
	if have == nil {
		if err := davNewContact(name, in); err != nil {
			return nil, err
		}
	} else if err := davUpdateContact(have, in); err != nil {
		return nil, err
	}
	c, err := b.find(ctx, p, true)
	if err != nil || c == nil {
		return &carddav.AddressObject{Path: p, ModTime: time.Now()}, nil
	}
	o, err := b.object(ctx, *c)
	return &o, err
}

func davUpdateContact(have *davContact, in *vcContact) error {
	set, rename := davCardChanges(have, in)
	if have.madeUid && rename != "" {
		// The uid was made from the title: keep it.
		set[vcUidField] = have.Uid
	}
	notes, setNotes := "", false
	if have.Record && strings.TrimSpace(in.Notes) != have.Notes {
		notes, setNotes = in.Notes, true
	}
	if len(set) == 0 && rename == "" && !setNotes {
		return nil
	}
	if have.Record {
		_, err := UpdateRecord(&common.RecordUpdate{Hash: have.Hash, Set: set, Name: rename, SetName: rename != "",
			Notes: notes, SetNotes: setNotes, Via: "from CardDAV"})
		return err
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		// Looked up again each time: every write reloads the file.
		sec := GetDb().FindByHash(have.Hash)
		f := GetDb().FileFromSection(sec)
		if sec == nil || f == nil {
			return fmt.Errorf("the contact moved; sync again")
		}
		if err := setHeadingProperty(f, sec, k, set[k]); err != nil {
			return err
		}
	}
	if rename != "" {
		sec := GetDb().FindByHash(have.Hash)
		f := GetDb().FileFromSection(sec)
		if sec == nil || f == nil || sec.Headline == nil {
			return fmt.Errorf("the contact moved; sync again")
		}
		lines, err := readFileLines(f.Filename)
		if err != nil {
			return err
		}
		row := sec.Headline.Pos.Row
		if row >= len(lines) || txLevel(lines[row]) == 0 {
			return fmt.Errorf("the contact moved; sync again")
		}
		lines[row] = replaceHeadlineTitle(lines[row], rename)
		return writeLines(f.Filename, trimTrailing(lines))
	}
	return nil
}

func davNewContact(name string, in *vcContact) error {
	fields := map[string]string{"DAV_NAME": name}
	for k, v := range in.Fields {
		fields[k] = v
	}
	if in.Name == "" {
		return webdav.NewHTTPError(http.StatusBadRequest, fmt.Errorf("a contact with no name"))
	}
	req := &common.RecordNew{Type: "contact", Name: in.Name, Fields: fields, Notes: in.Notes, Tags: in.Tags}
	if _, err := AddRecord(req); err == nil {
		return nil
	}
	// No contact collection: contacts.org in the first org directory.
	if len(Conf().Server.OrgDirs) == 0 {
		return fmt.Errorf("no org directory to file a contact in")
	}
	root, _ := filepath.Abs(Conf().Server.OrgDirs[0])
	file := filepath.Join(root, "contacts.org")
	if _, err := os.Stat(file); err != nil {
		if err := os.WriteFile(file, []byte("#+TITLE: Contacts\n"), 0644); err != nil {
			return err
		}
	}
	if GetDb().FindByFile(file) == nil {
		GetDb().LoadFile(file)
	}
	req.Filename = file
	_, err := AddRecord(req)
	return err
}

var _ carddav.Backend = (*cardBackend)(nil)
