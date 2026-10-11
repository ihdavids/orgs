package orgs

import (
	"strings"
	"testing"

	"github.com/ihdavids/orgs/internal/common"
)

// A one pixel png, so a photo in a card is a real picture.
const tinyPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=="

func crlf(s string) string { return strings.ReplaceAll(s, "\n", "\r\n") }

// What Apple Contacts exports: 3.0, CRLF, a BOM, item groups with X-ABLabel,
// TYPE written several times, a folded base64 photo, a year-less birthday
// written as 1604 with X-APPLE-OMIT-YEAR, social profiles and IM.
var appleCard = "\uFEFF" + crlf(`BEGIN:VCARD
VERSION:3.0
PRODID:-//Apple Inc.//macOS 14.0//EN
N:Roe;Jane;Q.;Dr.;
FN:Jane Roe
NICKNAME:Janey
ORG:Acme Robotics;Research;Vision
TITLE:Principal Engineer
EMAIL;type=INTERNET;type=HOME;type=pref:jane@home.example
EMAIL;type=INTERNET;type=WORK:jane.roe@acme.example
item1.EMAIL;type=INTERNET:jroe@oldjob.example
item1.X-ABLabel:Old work
TEL;type=CELL;type=VOICE;type=pref:+1 (555) 555-0199
TEL;type=HOME;type=VOICE:+1 555 555 0100
TEL;type=WORK;type=FAX:+1 555 555 0111
item2.TEL:+44 20 7946 0000
item2.X-ABLabel:_$!<Main>!$_
item3.ADR;type=HOME;type=pref:;;1 Infinite Loop\nBuilding 2;Cupertino;CA;95014;United States
item3.X-ABADR:us
item4.URL;type=pref:https://janeroe.dev
item4.X-ABLabel:_$!<HomePage>!$_
item5.X-ABDATE;type=pref:2010-06-12
item5.X-ABLabel:_$!<Anniversary>!$_
item6.X-ABRELATEDNAMES;type=pref:John Roe
item6.X-ABLabel:_$!<Spouse>!$_
X-SOCIALPROFILE;type=linkedin:https://www.linkedin.com/in/janeroe
X-SOCIALPROFILE;type=twitter;x-user=janeroe:x-apple:janeroe
IMPP;X-SERVICE-TYPE=Skype;type=HOME;type=pref:skype:jane.roe
BDAY;X-APPLE-OMIT-YEAR=1604:1604-05-15
NOTE:Met at the 2019 conference\, Berlin.\nAllergic to shellfish.
PHOTO;ENCODING=b;TYPE=PNG:iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkY
 PhfDwAChwGA60e6kgAAAABJRU5ErkJggg==
UID:5E5F8A2C-1B2D-4C3E-9F00-ABCDEF012345
X-ABUID:5E5F8A2C-1B2D-4C3E-9F00-ABCDEF012345:ABPerson
END:VCARD
`)

// What Google Contacts exports: 3.0, its own item groups for custom labels,
// CATEGORIES with myContacts in it, a year-less --MMDD birthday and a photo
// url.
var googleCards = `BEGIN:VCARD
VERSION:3.0
FN:Sam Lee
N:Lee;Sam;;;
EMAIL;TYPE=INTERNET;TYPE=WORK:sam@bigco.example
EMAIL;TYPE=INTERNET;TYPE=WORK:slee@bigco.example
item1.EMAIL;TYPE=INTERNET:sam.lee@gmail.example
item1.X-ABLabel:Personal
TEL;TYPE=CELL:+1 555 555 0142
ADR;TYPE=WORK:;;1600 Amphitheatre Pkwy;Mountain View;CA;94043;USA
ORG:BigCo
TITLE:Product Manager
BDAY:--0515
URL:https://www.linkedin.com/in/samlee
URL:https://github.com/samlee
URL;TYPE=WORK:https://bigco.example/team/sam
PHOTO:https://lh3.googleusercontent.com/contacts/abc123
CATEGORIES:myContacts,Clients,Board Members
NOTE:Prefers email.
END:VCARD
BEGIN:VCARD
VERSION:3.0
FN:Ana Gómez
EMAIL;TYPE=INTERNET:ana@example.org
CATEGORIES:myContacts
END:VCARD
`

// What old Outlook writes: 2.1, bare parameters, quoted-printable with soft
// line breaks and Windows-1252, N with no FN, and a base64 photo folded with
// leading spaces and ended by a blank line.
var outlookCard = "BEGIN:VCARD\r\n" +
	"VERSION:2.1\r\n" +
	"N;CHARSET=Windows-1252;ENCODING=QUOTED-PRINTABLE:M=FCller;J=FCrgen\r\n" +
	"ORG;CHARSET=Windows-1252;ENCODING=QUOTED-PRINTABLE:M=FCller GmbH\r\n" +
	"TITLE:Gesch\xe4ftsf\xfchrer\r\n" +
	"TEL;WORK;VOICE:+49 30 1234567\r\n" +
	"TEL;CELL;VOICE:+49 170 1234567\r\n" +
	"EMAIL;PREF;INTERNET:juergen@mueller.example\r\n" +
	"ADR;WORK;PREF;ENCODING=QUOTED-PRINTABLE;CHARSET=Windows-1252:;;Hauptstra=DFe 5;Berlin;;10115;Deutschland\r\n" +
	"NOTE;CHARSET=Windows-1252;ENCODING=QUOTED-PRINTABLE:Erster Teil, sehr lang und noch l=E4nger=\r\n" +
	" als erlaubt=0D=0AZweite Zeile=\r\n" +
	"\r\n" +
	"BDAY:19750314\r\n" +
	"PHOTO;TYPE=PNG;ENCODING=BASE64:\r\n" +
	"    iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk\r\n" +
	"    YPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==\r\n" +
	"\r\n" +
	"X-MS-OL-DEFAULT-POSTAL-ADDRESS:2\r\n" +
	"END:VCARD\r\n"

// vCard 4.0: PREF=1, quoted comma separated TYPE, tel: and mailto: uris, a
// data: uri photo, ANNIVERSARY, a value folded with a tab.
var v4Card = `BEGIN:VCARD
VERSION:4.0
FN:Lin Wu
N:Wu;Lin;;;
EMAIL;TYPE=work:lin@work.example
EMAIL;PREF=1;TYPE=home:mailto:lin@home.example
TEL;VALUE=uri;TYPE="voice,cell":tel:+1-555-555-0177
ANNIVERSARY:20150704
PHOTO:data:image/png;base64,` + tinyPNG + `
NOTE:A note that was folded
	 with a tab.
CATEGORIES:Friends,friends,Rock-climbing
END:VCARD
`

func fields(c *vcContact) map[string]string { return c.Fields }

func expect(t *testing.T, got map[string]string, want map[string]string) {
	t.Helper()
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestVCardApple(t *testing.T) {
	cs := contactsFromVCard(appleCard)
	if len(cs) != 1 {
		t.Fatalf("got %d cards, want 1", len(cs))
	}
	c := cs[0]
	if c.Name != "Jane Roe" {
		t.Errorf("name %q", c.Name)
	}
	expect(t, fields(c), map[string]string{
		"NICKNAME":       "Janey",
		"COMPANY":        "Acme Robotics",
		"DEPARTMENT":     "Research, Vision",
		"TITLE":          "Principal Engineer",
		"EMAIL":          "jane@home.example",
		"EMAIL_WORK":     "jane.roe@acme.example",
		"EMAIL_OLD_WORK": "jroe@oldjob.example",
		"PHONE_MOBILE":   "+1 (555) 555-0199",
		"PHONE_HOME":     "+1 555 555 0100",
		"FAX_WORK":       "+1 555 555 0111",
		"PHONE_MAIN":     "+44 20 7946 0000",
		"ADDRESS_HOME":   "1 Infinite Loop, Building 2, Cupertino, CA 95014, United States",
		"WEBSITE":        "https://janeroe.dev",
		"ANNIVERSARY":    "2010-06-12",
		"RELATED_SPOUSE": "John Roe",
		"LINKEDIN":       "https://www.linkedin.com/in/janeroe",
		"TWITTER":        "janeroe",
		"IM_SKYPE":       "jane.roe",
		"BIRTHDAY":       "--05-15",
		"VCARD_UID":      "5E5F8A2C-1B2D-4C3E-9F00-ABCDEF012345",
	})
	if c.Notes != "Met at the 2019 conference, Berlin.\nAllergic to shellfish." {
		t.Errorf("notes %q", c.Notes)
	}
	if c.Photo == nil || c.Photo.Ext != "png" || len(c.Photo.Data) < 60 {
		t.Errorf("photo %+v", c.Photo)
	}
	// The fields the record code reads as dates must parse as dates.
	if _, _, y, ok := parseBirthday(c.Fields["BIRTHDAY"]); !ok || y != 0 {
		t.Errorf("birthday %q does not read back as a year-less date", c.Fields["BIRTHDAY"])
	}
	for _, k := range c.Keys {
		if strings.ContainsAny(c.Fields[k], "\n\r") {
			t.Errorf("%s has a line break in it", k)
		}
	}
}

func TestVCardGoogle(t *testing.T) {
	cs := contactsFromVCard(googleCards)
	if len(cs) != 2 {
		t.Fatalf("got %d cards, want 2", len(cs))
	}
	c := cs[0]
	expect(t, fields(c), map[string]string{
		"EMAIL":          "sam@bigco.example",
		"EMAIL_WORK":     "slee@bigco.example",
		"EMAIL_PERSONAL": "sam.lee@gmail.example",
		"PHONE_MOBILE":   "+1 555 555 0142",
		"ADDRESS_WORK":   "1600 Amphitheatre Pkwy, Mountain View, CA 94043, USA",
		"COMPANY":        "BigCo",
		"BIRTHDAY":       "--05-15",
		"LINKEDIN":       "https://www.linkedin.com/in/samlee",
		"GITHUB":         "https://github.com/samlee",
		"WEBSITE_WORK":   "https://bigco.example/team/sam",
		"IMAGE":          "https://lh3.googleusercontent.com/contacts/abc123",
	})
	if strings.Join(c.Tags, ",") != "Clients,Board_Members" {
		t.Errorf("tags %v", c.Tags)
	}
	if cs[1].Name != "Ana Gómez" || len(cs[1].Tags) != 0 {
		t.Errorf("second card %q %v", cs[1].Name, cs[1].Tags)
	}
}

func TestVCardOutlook21(t *testing.T) {
	cs := contactsFromVCard(outlookCard)
	if len(cs) != 1 {
		t.Fatalf("got %d cards, want 1", len(cs))
	}
	c := cs[0]
	// No FN: the name is made from N, in Windows-1252.
	if c.Name != "Jürgen Müller" {
		t.Errorf("name %q", c.Name)
	}
	expect(t, fields(c), map[string]string{
		"COMPANY":      "Müller GmbH",
		"TITLE":        "Geschäftsführer",
		"PHONE_WORK":   "+49 30 1234567",
		"PHONE_MOBILE": "+49 170 1234567",
		"EMAIL":        "juergen@mueller.example",
		"ADDRESS_WORK": "Hauptstraße 5, Berlin, 10115, Deutschland",
		"BIRTHDAY":     "1975-03-14",
	})
	if c.Notes != "Erster Teil, sehr lang und noch länger als erlaubt\nZweite Zeile" {
		t.Errorf("notes %q", c.Notes)
	}
	if c.Photo == nil || c.Photo.Ext != "png" {
		t.Errorf("photo %+v", c.Photo)
	}
}

func TestVCard40(t *testing.T) {
	c := contactsFromVCard(v4Card)[0]
	expect(t, fields(c), map[string]string{
		// PREF=1 makes the home address the main one, wherever it was written.
		"EMAIL":        "lin@home.example",
		"EMAIL_WORK":   "lin@work.example",
		"PHONE_MOBILE": "+1-555-555-0177",
		"ANNIVERSARY":  "2015-07-04",
	})
	if c.Notes != "A note that was folded with a tab." {
		t.Errorf("notes %q", c.Notes)
	}
	if c.Photo == nil || c.Photo.Ext != "png" {
		t.Errorf("photo %+v", c.Photo)
	}
	if strings.Join(c.Tags, ",") != "Friends,Rock_climbing" {
		t.Errorf("tags %v", c.Tags)
	}
}

func TestVCardLines(t *testing.T) {
	// Escapes inside a structured value split on the right semicolons.
	p, ok := parseVCardLine(`N:Smith\;Jones;Ann;;;`)
	if !ok || p.parts()[0] != "Smith;Jones" || p.parts()[1] != "Ann" {
		t.Errorf("N parts %q", p.parts())
	}
	// Groups, and the three ways of writing a type.
	p, _ = parseVCardLine(`item7.TEL;TYPE=work,cell;TYPE=pref:1`)
	if p.Group != "item7" || p.Name != "TEL" || strings.Join(p.types(), ",") != "work,cell,pref" || p.pref() != 1 {
		t.Errorf("parsed %+v", p)
	}
	p, _ = parseVCardLine(`TEL;HOME;FAX:1`)
	if strings.Join(p.types(), ",") != "home,fax" {
		t.Errorf("bare types %v", p.types())
	}
	// A colon inside a quoted parameter is not the end of the name.
	p, _ = parseVCardLine(`X-THING;LABEL="a:b":value`)
	if p.Value != "value" || p.param("LABEL") != "a:b" {
		t.Errorf("quoted param %+v", p)
	}
	// A base64 value ending in "=" is not a soft line break.
	got := unfoldVCard([]string{"PHOTO;ENCODING=b:AAA=", "UID:x"})
	if len(got) != 2 {
		t.Errorf("unfolded into %q", got)
	}
	// Two cards with an AGENT card inside the first.
	cs := parseVCards("BEGIN:VCARD\nFN:A\nAGENT:BEGIN:VCARD\nFN:Inner\nEND:VCARD\nEND:VCARD\nBEGIN:VCARD\nFN:B\nEND:VCARD\n")
	if len(cs) != 2 || cs[0].text("FN") != "A" || cs[1].text("FN") != "B" {
		t.Errorf("cards %d", len(cs))
	}
}

func TestVCardDuplicateLabels(t *testing.T) {
	c := contactsFromVCard(`BEGIN:VCARD
VERSION:3.0
FN:Two Works
EMAIL;TYPE=HOME;TYPE=PREF:a@home.example
EMAIL;TYPE=WORK:b@work.example
EMAIL;TYPE=WORK:c@work.example
EMAIL;TYPE=WORK:B@Work.example
END:VCARD`)[0]
	expect(t, fields(c), map[string]string{
		"EMAIL": "a@home.example", "EMAIL_WORK": "b@work.example", "EMAIL_WORK_2": "c@work.example",
	})
	if _, dup := c.Fields["EMAIL_WORK_3"]; dup {
		t.Errorf("the same address twice was written twice")
	}
}

// A record as the server reads one, for the planning tests.
func testRecord(name string, kv ...string) *common.Record {
	r := &common.Record{Hash: "h-" + name, Id: "id-" + name, Type: "contact", Name: name, Filename: "/x/contacts.org"}
	for i := 0; i+1 < len(kv); i += 2 {
		n, l, k := fieldOf(kv[i])
		r.Fields = append(r.Fields, common.RecordField{Key: kv[i], Name: n, Label: l, Kind: k, Value: kv[i+1]})
	}
	return r
}

func changeKeys(item common.ContactImportItem, how string) string {
	out := []string{}
	for _, c := range item.Changes {
		if c.How == how {
			out = append(out, c.Key)
		}
	}
	return strings.Join(out, ",")
}

func TestPlanImportMatching(t *testing.T) {
	cs := contactsFromVCard(googleCards + appleCard)
	existing := []*common.Record{
		// Sam is matched by an email, under another field name.
		testRecord("Samuel Lee", "EMAIL_WORK", "SAM@bigco.example", "BIRTHDAY", "1980-06-01", "PHONE_MOBILE", "555-555-0142"),
		// Jane by name.
		testRecord("jane roe", "PHONE_MOBILE", "+15555550199"),
	}
	plans := planImport(cs, existing, "merge", nil)
	if len(plans) != 3 {
		t.Fatalf("got %d plans", len(plans))
	}
	sam, ana, jane := plans[0].item, plans[1].item, plans[2].item

	if sam.Action != "update" || sam.Match != "email" {
		t.Errorf("sam %s %s", sam.Action, sam.Match)
	}
	// The address he has is not added again; the birthday differs and merge
	// keeps the record's; the second work address goes beside the first.
	if strings.Contains(changeKeys(sam, "add"), "PHONE_MOBILE") {
		t.Errorf("the same phone number was added again: %s", changeKeys(sam, "add"))
	}
	if changeKeys(sam, "kept") != "BIRTHDAY" {
		t.Errorf("kept %q", changeKeys(sam, "kept"))
	}
	if plans[0].set["EMAIL"] != "" || plans[0].set["EMAIL_WORK_2"] != "slee@bigco.example" {
		t.Errorf("sam's emails %v", plans[0].set)
	}
	if strings.Join(sam.Tags, ",") != "Clients,Board_Members" || !sam.Notes {
		t.Errorf("sam tags %v notes %v", sam.Tags, sam.Notes)
	}

	if ana.Action != "add" || ana.Match != "" {
		t.Errorf("ana %s", ana.Action)
	}
	if jane.Action != "update" || jane.Match != "name" || !plans[2].photo {
		t.Errorf("jane %s %s photo %v", jane.Action, jane.Match, plans[2].photo)
	}
	if _, again := plans[2].set["PHONE_MOBILE"]; again {
		t.Errorf("jane's mobile written again with different punctuation")
	}

	// skip leaves both matches alone.
	plans = planImport(cs, existing, "skip", nil)
	if plans[0].item.Action != "skip" || plans[2].item.Action != "skip" || plans[1].item.Action != "add" {
		t.Errorf("skip: %s %s %s", plans[0].item.Action, plans[1].item.Action, plans[2].item.Action)
	}

	// A year-less birthday on the same day is the same birthday.
	if !sameValue("date", "--05-15", "1980-05-15") || sameValue("date", "--05-15", "1980-06-15") {
		t.Errorf("year-less dates")
	}

	// overwrite writes the different birthday, and does not replace an
	// address the card still has.
	plans = planImport(cs, existing, "overwrite", nil)
	if plans[0].set["BIRTHDAY"] != "--05-15" || changeKeys(plans[0].item, "replace") != "BIRTHDAY" || plans[0].set["EMAIL_WORK_2"] != "slee@bigco.example" {
		t.Errorf("overwrite: %v", plans[0].item.Changes)
	}
}

// Importing what the address book already says changes nothing: this is what
// makes running the same import twice safe.
func TestPlanImportIsIdempotent(t *testing.T) {
	c := contactsFromVCard(appleCard)[0]
	kv := []string{}
	for _, k := range c.Keys {
		kv = append(kv, k, c.Fields[k])
	}
	kv = append(kv, "IMAGE", "[[file:images/contacts/jane-roe.png]]")
	rec := testRecord("Jane Roe", kv...)
	rec.Notes = "Some words of my own.\n\nMet at the 2019 conference, Berlin.\n  Allergic to shellfish."
	rec.Tags = nil
	for _, mode := range []string{"merge", "overwrite"} {
		photo := func(*common.Record) []byte { return c.Photo.Data }
		p := planImport(contactsFromVCard(appleCard), []*common.Record{rec}, mode, photo)[0]
		if p.item.Action != "skip" || p.item.Match != "uid" {
			t.Errorf("%s: %s (%s) %+v", mode, p.item.Action, p.item.Reason, p.item.Changes)
		}
	}
}

// One person twice in a file is one person.
func TestPlanImportFoldsDuplicates(t *testing.T) {
	text := `BEGIN:VCARD
FN:Dee Dupe
EMAIL:dee@example.org
END:VCARD
BEGIN:VCARD
FN:Dee D.
EMAIL:DEE@example.org
TEL;TYPE=CELL:555 0101 222
NOTE:second card
END:VCARD`
	plans := planImport(contactsFromVCard(text), nil, "merge", nil)
	if len(plans) != 1 {
		t.Fatalf("got %d plans", len(plans))
	}
	p := plans[0]
	if p.item.Cards != 2 || p.set["PHONE_MOBILE"] != "555 0101 222" || p.notes != "second card" || p.item.Name != "Dee Dupe" {
		t.Errorf("folded %+v %v", p.item, p.set)
	}
}

func TestOrgTagAndLabels(t *testing.T) {
	for in, want := range map[string]string{
		"Board Members": "Board_Members", "rock-climbing": "rock_climbing", "  ": "", "Ü-bahn": "Ü_bahn",
	} {
		if got := orgTag(in); got != want {
			t.Errorf("orgTag(%q) = %q, want %q", in, got, want)
		}
	}
	if got := labelKey("Old work"); got != "OLD_WORK" {
		t.Errorf("labelKey %q", got)
	}
}
