package orgs

import "testing"

// What a field is, from its name alone. This is the rule the whole record
// format rests on - a client draws a contact and a guitar pedal with the same
// code because it never has to know what EMAIL means - and it is said a second
// time in worg/src/records.ts so an add form can show the right input before
// anything is saved. If this file changes, kindOfKey there changes with it.
func TestFieldOf(t *testing.T) {
	cases := []struct {
		key   string
		name  string
		label string
		kind  string
	}{
		// The plain ones.
		{"EMAIL", "EMAIL", "", "email"},
		{"PHONE", "PHONE", "", "tel"},
		{"ADDRESS", "ADDRESS", "", "postal"},
		{"WEBSITE", "WEBSITE", "", "url"},
		{"LINKEDIN", "LINKEDIN", "", "social"},
		{"BIRTHDAY", "BIRTHDAY", "", "date"},
		{"IMAGE", "IMAGE", "", "image"},
		{"SERIAL", "SERIAL", "", "text"},

		// A label is how a thing has more than one of something, and it is the
		// part after the *first* underscore - a label is free text.
		{"PHONE_MOBILE", "PHONE", "MOBILE", "tel"},
		{"EMAIL_WORK", "EMAIL", "WORK", "email"},
		{"EMAIL_WORK_OLD", "EMAIL", "WORK OLD", "email"},

		// The suffix rules, for the way things other than people get named.
		{"BOUGHT_DATE", "BOUGHT", "", "date"},
		{"MANUAL_URL", "MANUAL", "", "url"},
		{"BOX_IMAGE", "BOX", "", "image"},
		{"DATE_OF_PURCHASE_DATE", "DATE OF PURCHASE", "", "date"},

		// Lowercase in the file is the same field.
		{"phone_home", "PHONE", "HOME", "tel"},
	}
	for _, c := range cases {
		name, label, kind := fieldOf(c.key)
		if name != c.name || label != c.label || kind != c.kind {
			t.Errorf("fieldOf(%q) = (%q, %q, %q), want (%q, %q, %q)",
				c.key, name, label, kind, c.name, c.label, c.kind)
		}
	}
}

// MOBILE is a phone number when it stands alone and a *label* when it follows
// PHONE. Reading it as a kind suffix would quietly throw the label away and
// leave a contact with three fields all called "phone", which is why the
// suffix list is short and this is pinned separately.
func TestLabelsAreNotKinds(t *testing.T) {
	for _, key := range []string{"PHONE_MOBILE", "PHONE_WORK", "EMAIL_HOME", "ADDRESS_WORK"} {
		_, label, _ := fieldOf(key)
		if label == "" {
			t.Errorf("fieldOf(%q) lost its label", key)
		}
	}
}

// A value made actionable, or left as text. An empty answer means "print it",
// which is the right answer for a phone extension or a note in an address.
func TestFieldLink(t *testing.T) {
	cases := []struct {
		kind, name, value, want string
	}{
		{"email", "EMAIL", "jane@example.org", "mailto:jane@example.org"},
		{"email", "EMAIL", "ask her assistant", ""},
		{"tel", "PHONE", "+1-555-555-0199", "tel:+15555550199"},
		{"tel", "PHONE", "ext 12", ""},
		{"url", "WEBSITE", "janeroe.dev", "https://janeroe.dev"},
		{"url", "WEBSITE", "https://janeroe.dev", "https://janeroe.dev"},
		// A social field is written either way and both have to work.
		{"social", "LINKEDIN", "johndoe", "https://www.linkedin.com/in/johndoe"},
		{"social", "LINKEDIN", "@johndoe", "https://www.linkedin.com/in/johndoe"},
		{"social", "LINKEDIN", "https://linkedin.com/in/x", "https://linkedin.com/in/x"},
		// A network with no predictable address keeps its handle as text
		// rather than being sent somewhere guessed at.
		{"social", "SIGNAL", "jane.42", ""},
		{"text", "SERIAL", "PF-3K9L22", ""},
	}
	for _, c := range cases {
		if got := fieldLink(c.kind, c.name, c.value, ""); got != c.want {
			t.Errorf("fieldLink(%q, %q, %q) = %q, want %q", c.kind, c.name, c.value, got, c.want)
		}
	}
}

// The three ways a yearly date may be written, all of them exact. A year of
// zero means the record did not give one, which is what makes the age optional
// rather than wrong.
func TestParseBirthday(t *testing.T) {
	cases := []struct {
		in               string
		m, d, y          int
		ok               bool
	}{
		{"1990-05-15", 5, 15, 1990, true},
		{"[1990-05-15 Tue]", 5, 15, 1990, true},
		{"<1990-05-15>", 5, 15, 1990, true},
		{"--05-15", 5, 15, 0, true},
		{"-05-15", 5, 15, 0, true},
		{"5/15/1990", 5, 15, 1990, true},
		{"", 0, 0, 0, false},
		{"sometime in May", 0, 0, 0, false},
		{"1990-13-40", 0, 0, 0, false},
	}
	for _, c := range cases {
		m, d, y, ok := parseBirthday(c.in)
		if ok != c.ok || (ok && (m != c.m || d != c.d || y != c.y)) {
			t.Errorf("parseBirthday(%q) = (%d, %d, %d, %v), want (%d, %d, %d, %v)",
				c.in, m, d, y, ok, c.m, c.d, c.y, c.ok)
		}
	}
}

// Renaming rewrites the title and nothing else. A heading says several things
// about itself and only one of them is its name.
func TestReplaceHeadlineTitle(t *testing.T) {
	cases := []struct{ in, title, want string }{
		{"* John Doe", "Jane Roe", "* Jane Roe"},
		{"** John Doe", "Jane Roe", "** Jane Roe"},
		{"** John Doe  :work:friend:", "Jane Roe", "** Jane Roe  :work:friend:"},
		{"* TODO John Doe", "Jane Roe", "* TODO Jane Roe"},
		{"* TODO [#A] John Doe  :work:", "Jane Roe", "* TODO [#A] Jane Roe  :work:"},
		// Not a heading: left exactly as it was.
		{"  not a heading", "Jane Roe", "  not a heading"},
	}
	for _, c := range cases {
		if got := replaceHeadlineTitle(c.in, c.title); got != c.want {
			t.Errorf("replaceHeadlineTitle(%q, %q) = %q, want %q", c.in, c.title, got, c.want)
		}
	}
}

// The drawers of a record are found by walking the lines under its headline,
// because go-org ends a headline's body at a drawer in column zero and hoists
// the rest of the heading to the top of the document. Walking has to skip a
// planning line, has to step over a drawer that is not the one wanted, and has
// to stop at the first line of the notes - a :LOGBOOK: quoted in the body is
// not this record's logbook.
func TestDrawerAt(t *testing.T) {
	lines := []string{
		"** John Doe",
		"   SCHEDULED: <2026-09-26 Sat>",
		"   :PROPERTIES:",
		"   :RECORD: contact",
		"   :END:",
		"   :LOGBOOK:",
		"   - Added [2026-09-01 Tue 09:12]",
		"   :END:",
		"",
		"   Some notes, which mention :LOGBOOK: in passing.",
		"   :LOGBOOK:",
		"   :END:",
	}
	to := len(lines) - 1

	if s, e, ok := drawerAt(lines, 0, to, "PROPERTIES"); !ok || s != 2 || e != 4 {
		t.Errorf("properties drawer = (%d, %d, %v), want (2, 4, true)", s, e, ok)
	}
	if s, e, ok := drawerAt(lines, 0, to, "LOGBOOK"); !ok || s != 5 || e != 7 {
		t.Errorf("logbook drawer = (%d, %d, %v), want (5, 7, true) - the one in the notes is not it", s, e, ok)
	}
	if _, _, ok := drawerAt(lines, 0, to, "CLOCK"); ok {
		t.Errorf("found a CLOCK drawer that is not there")
	}
	if got := headEnd(lines, 0, to); got != 8 {
		t.Errorf("headEnd = %d, want 8 (the blank line before the notes)", got)
	}
}
