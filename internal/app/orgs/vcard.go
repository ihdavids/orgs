package orgs

// Reading vCards: the .vcf files Apple Contacts, Google Contacts and Outlook
// export, and turning each card into the fields of a contact record.
//
// Nothing in here touches the database or the disk. The import itself - which
// card is which contact, what gets written - is vcardimport.go; this file is the
// part that has to be right about the format, and it is pure so that it can be
// tested against the odd things real exporters write.
//
// The three versions in the wild, and what each exporter does with them:
//
//   - 2.1, Outlook and old phones: bare parameters (TEL;WORK;VOICE:), values in
//     quoted-printable with soft line breaks and a CHARSET (often
//     Windows-1252), base64 photos folded with leading spaces and ended by a
//     blank line.
//   - 3.0, Apple and Google: TYPE= parameters, repeated or comma separated,
//     Apple's property groups (item1.EMAIL with item1.X-ABLabel naming it),
//     ENCODING=b photos, backslash escapes.
//   - 4.0, newer exporters: PREF=1, data: uris for photos, ANNIVERSARY, tel: and
//     mailto: uris as values.
//
// How a card becomes a contact:
//
//	| vCard                          | Contact field                                   |
//	|--------------------------------+-------------------------------------------------|
//	| FN (else N as "Given Family",  | the heading                                     |
//	|   else ORG, else the email)    |                                                 |
//	| NICKNAME                       | NICKNAME                                        |
//	| ORG                            | COMPANY, and DEPARTMENT for the units after it  |
//	| TITLE / ROLE                   | TITLE / ROLE                                    |
//	| EMAIL                          | the preferred (else first) one EMAIL, the rest  |
//	|                                | EMAIL_WORK, EMAIL_HOME, EMAIL_<label>           |
//	| TEL                            | PHONE_MOBILE, PHONE_HOME, PHONE_WORK, PHONE_MAIN|
//	|                                | PHONE_PAGER, FAX_HOME, FAX_WORK, PHONE_<label>  |
//	| ADR                            | ADDRESS or ADDRESS_<label>, as one line         |
//	| BDAY                           | BIRTHDAY as 1990-05-15, or --05-15 with no year |
//	| ANNIVERSARY, X-ANNIVERSARY,    | ANNIVERSARY                                     |
//	|   Apple's anniversary X-ABDATE |                                                 |
//	| other X-ABDATE                 | <LABEL>_DATE                                    |
//	| URL                            | WEBSITE / WEBSITE_<label>, or the network's     |
//	|                                | field when it is a profile (linkedin.com/in/x)  |
//	| X-SOCIALPROFILE, SOCIALPROFILE | LINKEDIN, TWITTER, GITHUB... (socialNetworks);  |
//	|                                | WEBSITE_<NETWORK> for one not in the list       |
//	| X-TWITTER, X-LINKEDIN, ...     | the network's field                             |
//	| IMPP, X-SKYPE, X-AIM, X-JABBER | the network's field (WHATSAPP, SIGNAL...) or    |
//	|                                | IM_<SERVICE>                                    |
//	| X-ABRELATEDNAMES, RELATED      | RELATED_<label>: RELATED_SPOUSE                 |
//	| NOTE                           | the record's notes                              |
//	| CATEGORIES                     | tags, made into org tags (Google's myContacts   |
//	|                                | is dropped: every Google contact is in it)      |
//	| PHOTO inline                   | a file in images/contacts/ beside the org file, |
//	|                                | linked from IMAGE                               |
//	| PHOTO url                      | IMAGE, the url itself                           |
//	| UID (else X-ABUID)             | VCARD_UID, which is what a re-import matches on |
//
// A label is the Apple X-ABLabel of the property's group when it has one (a
// custom label is the only place the name a person gave it survives), else the
// TYPE. Two of the same field are told apart the way records always tell them
// apart, by a suffix: EMAIL_WORK, EMAIL_WORK_2.

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ihdavids/orgs/internal/common"
)

// ----------------------------------------------------------------------------
// The format
// ----------------------------------------------------------------------------

// One content line of a card, read.
type vcProp struct {
	// Apple's "item1" in item1.EMAIL; empty for most lines.
	Group string
	// Uppercased: "EMAIL", "X-ABLABEL".
	Name string
	// Uppercased keys. A bare 2.1 parameter (WORK, CELL) is filed under TYPE,
	// and a bare QUOTED-PRINTABLE or BASE64 under ENCODING, because that is
	// what they mean.
	Params map[string][]string
	// The value with any transfer encoding and charset taken off, but still
	// in vCard's own escaping - structured values have to be split on their
	// semicolons before the escapes are undone, or "Smith\;Jones" splits.
	Value string
	// Decoded bytes, for a value that was base64 or a data: uri.
	Data []byte
	// What the data: uri or the parameters said the bytes are.
	MediaType string
}

// One card.
type vCard struct {
	Version string
	Props   []vcProp
	// Apple's group labels: "item1" -> "_$!<Other>!$_", read once.
	labels map[string]string
}

func (c *vCard) all(name string) []vcProp {
	out := []vcProp{}
	for _, p := range c.Props {
		if p.Name == name {
			out = append(out, p)
		}
	}
	return out
}

func (c *vCard) first(name string) (vcProp, bool) {
	for _, p := range c.Props {
		if p.Name == name {
			return p, true
		}
	}
	return vcProp{}, false
}

func (c *vCard) text(name string) string {
	if p, ok := c.first(name); ok {
		return strings.TrimSpace(p.text())
	}
	return ""
}

func (p vcProp) param(key string) string {
	if v := p.Params[key]; len(v) > 0 {
		return v[0]
	}
	return ""
}

// types is every TYPE the line carries, lowercased, whichever of the three ways
// it was written: TYPE=work,cell / TYPE=WORK;TYPE=CELL / 2.1's bare WORK;CELL.
func (p vcProp) types() []string {
	out := []string{}
	for _, v := range p.Params["TYPE"] {
		for _, t := range strings.Split(v, ",") {
			if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
				out = append(out, t)
			}
		}
	}
	return out
}

func (p vcProp) hasType(t string) bool {
	for _, x := range p.types() {
		if x == t {
			return true
		}
	}
	return false
}

// pref is how preferred the line is, lower first, 100 for "not said". 3.0 says
// TYPE=pref; 4.0 says PREF=1..100.
func (p vcProp) pref() int {
	if v := p.param("PREF"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
		return 1
	}
	if p.hasType("pref") {
		return 1
	}
	return 100
}

// text is the value as a person would read it: escapes undone.
func (p vcProp) text() string {
	return vcUnescape(p.Value)
}

// parts is a structured value (N, ADR, ORG) split on its unescaped
// semicolons, each part unescaped.
func (p vcProp) parts() []string {
	raw := vcSplit(p.Value, ';')
	for i := range raw {
		raw[i] = strings.TrimSpace(vcUnescape(raw[i]))
	}
	return raw
}

// list is a comma separated value (CATEGORIES, NICKNAME).
func (p vcProp) list() []string {
	out := []string{}
	for _, v := range vcSplit(p.Value, ',') {
		if v = strings.TrimSpace(vcUnescape(v)); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// vcSplit splits on sep where it is not escaped by a backslash.
func vcSplit(s string, sep byte) []string {
	out := []string{}
	var cur strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' && i+1 < len(s) {
			cur.WriteByte(c)
			cur.WriteByte(s[i+1])
			i++
			continue
		}
		if c == sep {
			out = append(out, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteByte(c)
	}
	return append(out, cur.String())
}

// vcUnescape undoes \n \N \, \; \: and \\. An unknown escape keeps its
// character, which is what 2.1 files that never escaped anything need.
func vcUnescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\\' || i+1 >= len(s) {
			b.WriteByte(c)
			continue
		}
		i++
		switch s[i] {
		case 'n', 'N':
			b.WriteByte('\n')
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// parseVCards reads every card in a .vcf. A line it cannot make sense of is
// skipped rather than failing the file: an export of five hundred contacts
// with one odd line in it should import five hundred contacts.
func parseVCards(text string) []*vCard {
	// Every byte order mark, not just the first: several files sent as one
	// text carry one each, and one left in front of BEGIN:VCARD loses a card.
	text = strings.ReplaceAll(text, "\uFEFF", "")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	cards := []*vCard{}
	var cur *vCard
	depth := 0
	for _, line := range unfoldVCard(strings.Split(text, "\n")) {
		p, ok := parseVCardLine(line)
		if !ok {
			continue
		}
		switch {
		case p.Name == "BEGIN" && strings.EqualFold(strings.TrimSpace(p.Value), "VCARD"):
			// A 2.1 AGENT can carry a whole card inside another; it is
			// somebody else's card, and is stepped over.
			depth++
			if depth == 1 {
				cur = &vCard{labels: map[string]string{}}
			}
			continue
		case p.Name == "END" && strings.EqualFold(strings.TrimSpace(p.Value), "VCARD"):
			if depth == 1 && cur != nil {
				cards = append(cards, cur)
				cur = nil
			}
			if depth > 0 {
				depth--
			}
			continue
		}
		if cur == nil || depth != 1 {
			continue
		}
		if p.Name == "VERSION" {
			cur.Version = strings.TrimSpace(p.Value)
		}
		if p.Name == "X-ABLABEL" && p.Group != "" {
			cur.labels[strings.ToLower(p.Group)] = strings.TrimSpace(p.text())
		}
		cur.Props = append(cur.Props, p)
	}
	return cards
}

var qpHeadRe = regexp.MustCompile(`(?i)[;:]\s*(ENCODING\s*=\s*)?QUOTED-PRINTABLE\b`)

// unfoldVCard joins a content line's continuations back onto it.
//
// Two kinds: the standard fold (a line break followed by a space or tab, which
// is removed with the break), and the quoted-printable soft break of 2.1, where
// a line ending in "=" continues on the next line whatever it starts with.
// The second only applies to a line whose parameters say quoted-printable,
// because a base64 value or a url can legitimately end in "=".
func unfoldVCard(phys []string) []string {
	out := []string{}
	cur := ""
	have := false
	isQP := func(l string) bool {
		i := vcValueStart(l)
		return i > 0 && qpHeadRe.MatchString(l[:i+1])
	}
	for _, l := range phys {
		if have && isQP(cur) && strings.HasSuffix(cur, "=") {
			cur = cur[:len(cur)-1] + l
			continue
		}
		if have && (strings.HasPrefix(l, " ") || strings.HasPrefix(l, "\t")) {
			cur += l[1:]
			continue
		}
		if have {
			out = append(out, cur)
		}
		if strings.TrimSpace(l) == "" {
			// A blank line ends a 2.1 base64 value, and is otherwise nothing.
			cur, have = "", false
			continue
		}
		cur, have = l, true
	}
	if have {
		out = append(out, cur)
	}
	return out
}

// vcValueStart is the index of the colon that ends a line's name and
// parameters: the first one outside double quotes. -1 when there is none.
func vcValueStart(l string) int {
	quoted := false
	for i := 0; i < len(l); i++ {
		switch l[i] {
		case '"':
			quoted = !quoted
		case ':':
			if !quoted {
				return i
			}
		}
	}
	return -1
}

var vcNameRe = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

func parseVCardLine(line string) (vcProp, bool) {
	at := vcValueStart(line)
	if at <= 0 {
		return vcProp{}, false
	}
	head, value := line[:at], line[at+1:]
	segs := splitParams(head)
	name := strings.TrimSpace(segs[0])
	p := vcProp{Params: map[string][]string{}}
	if i := strings.LastIndex(name, "."); i > 0 {
		p.Group, name = name[:i], name[i+1:]
	}
	if !vcNameRe.MatchString(name) {
		return vcProp{}, false
	}
	p.Name = strings.ToUpper(name)
	for _, s := range segs[1:] {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		k, v, has := strings.Cut(s, "=")
		if !has {
			// 2.1's bare parameter.
			up := strings.ToUpper(s)
			switch up {
			case "QUOTED-PRINTABLE", "BASE64", "8BIT", "7BIT", "B":
				p.Params["ENCODING"] = append(p.Params["ENCODING"], up)
			case "PREF":
				p.Params["TYPE"] = append(p.Params["TYPE"], "pref")
			default:
				p.Params["TYPE"] = append(p.Params["TYPE"], s)
			}
			continue
		}
		k = strings.ToUpper(strings.TrimSpace(k))
		v = strings.TrimSpace(v)
		if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
			v = v[1 : len(v)-1]
		}
		p.Params[k] = append(p.Params[k], v)
	}

	enc := strings.ToUpper(p.param("ENCODING"))
	switch enc {
	case "QUOTED-PRINTABLE":
		value = string(decodeQP(value))
		value = toUTF8(value, p.param("CHARSET"))
	case "B", "BASE64":
		p.Data = decodeB64(value)
		p.MediaType = mediaTypeOf(p)
		value = ""
	default:
		value = toUTF8(value, p.param("CHARSET"))
		if strings.HasPrefix(strings.ToLower(value), "data:") {
			if mt, data, ok := decodeDataURI(value); ok {
				p.Data, p.MediaType, value = data, mt, ""
			}
		}
	}
	p.Value = value
	return p, true
}

// splitParams splits "item1.TEL;TYPE=\"work;odd\";PREF" on semicolons that are
// not inside quotes.
func splitParams(head string) []string {
	out := []string{}
	quoted := false
	start := 0
	for i := 0; i < len(head); i++ {
		switch head[i] {
		case '"':
			quoted = !quoted
		case ';':
			if !quoted {
				out = append(out, head[start:i])
				start = i + 1
			}
		}
	}
	return append(out, head[start:])
}

func decodeQP(s string) []byte {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		// "=XX" is a byte; an "=" that is not followed by two hex digits is
		// kept, since a hand-edited file often has one.
		if c == '=' && i+2 < len(s) {
			if b, err := strconv.ParseUint(s[i+1:i+3], 16, 8); err == nil {
				out = append(out, byte(b))
				i += 2
				continue
			}
		}
		out = append(out, c)
	}
	return out
}

// toUTF8 turns a value written in a legacy charset into UTF-8. Outlook writes
// Windows-1252; ISO-8859-1 is the same for every printable character. Bytes
// that are already valid UTF-8 are left alone whatever the label says, since
// a file that was converted to UTF-8 by some tool rarely has its labels fixed.
func toUTF8(s, charset string) string {
	// The label is not consulted beyond this: every legacy charset seen in
	// contact exports (Windows-1252, ISO-8859-1, ISO-8859-15 for the letters
	// people's names use) reads correctly as 1252.
	if utf8.ValidString(s) {
		return s
	}
	return common.DecodeCP1252([]byte(s))
}

func decodeB64(s string) []byte {
	clean := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
	if b, err := base64.StdEncoding.DecodeString(clean); err == nil {
		return b
	}
	if b, err := base64.RawStdEncoding.DecodeString(strings.TrimRight(clean, "=")); err == nil {
		return b
	}
	if b, err := base64.URLEncoding.DecodeString(clean); err == nil {
		return b
	}
	return nil
}

func decodeDataURI(v string) (mediaType string, data []byte, ok bool) {
	rest := v[len("data:"):]
	meta, payload, found := strings.Cut(rest, ",")
	if !found {
		return "", nil, false
	}
	mt := strings.Split(meta, ";")[0]
	if strings.HasSuffix(strings.ToLower(meta), ";base64") {
		data = decodeB64(payload)
	} else {
		u, err := url.PathUnescape(payload)
		if err != nil {
			return "", nil, false
		}
		data = []byte(u)
	}
	if data == nil {
		return "", nil, false
	}
	return strings.ToLower(mt), data, true
}

// mediaTypeOf is what the parameters say a base64 value is. 3.0 says TYPE=JPEG,
// 4.0 says MEDIATYPE=image/jpeg.
func mediaTypeOf(p vcProp) string {
	if mt := p.param("MEDIATYPE"); mt != "" {
		return strings.ToLower(mt)
	}
	for _, t := range p.types() {
		switch t {
		case "jpeg", "jpg":
			return "image/jpeg"
		case "png", "gif", "bmp", "webp", "heic", "tiff":
			return "image/" + t
		}
		if strings.HasPrefix(t, "image/") {
			return t
		}
	}
	return ""
}

// ----------------------------------------------------------------------------
// From a card to a contact
// ----------------------------------------------------------------------------

// A picture that came in the card.
type vcPhoto struct {
	Data []byte
	// "jpg", "png" - what the file is saved as.
	Ext string
}

// One card read into the shape of a contact record.
type vcContact struct {
	Name   string
	Fields map[string]string
	// The order the fields came out of the card, which is the order they are
	// written in when the collection does not ask for its own.
	Keys  []string
	Notes string
	Tags  []string
	// The card's UID, also in Fields as VCARD_UID.
	Uid      string
	Photo    *vcPhoto
	PhotoURL string
	// Something about the card a person should hear: a photo that was not a
	// picture, a card with no name.
	Warnings []string
	// How many cards were folded into this one (a file that holds a person
	// twice).
	Cards int
}

const vcUidField = "VCARD_UID"

// What kind of thing a field holds for the purposes of "do we have this
// already" - a profile url and the same url in WEBSITE are the same thing.
func compareKind(key string) string {
	_, _, kind := fieldOf(key)
	if kind == "social" {
		return "url"
	}
	return kind
}

// normValue is a value reduced to what makes it the same as another: case in
// an email, punctuation in a phone number, the scheme and trailing slash of a
// url.
func normValue(kind, v string) string {
	v = strings.TrimSpace(v)
	if t := linkTarget(v); t != "" && strings.HasPrefix(v, "[[") {
		v = t
	}
	switch kind {
	case "email":
		return strings.ToLower(strings.TrimPrefix(strings.ToLower(v), "mailto:"))
	case "tel":
		d := strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, v)
		return d
	case "url":
		s := strings.ToLower(v)
		for _, p := range []string{"https://", "http://", "www."} {
			s = strings.TrimPrefix(s, p)
		}
		s = strings.TrimSuffix(s, "/")
		s = strings.TrimPrefix(s, "@")
		return s
	case "date":
		if m, d, y, ok := parseBirthday(v); ok {
			return fmt.Sprintf("%04d-%02d-%02d", y, m, d)
		}
	case "postal":
		return strings.Map(func(r rune) rune {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				return unicode.ToLower(r)
			}
			return -1
		}, v)
	}
	return strings.ToLower(strings.Join(strings.Fields(v), " "))
}

// sameValue says whether two values of one kind are the same thing. Phone
// numbers match when one is the other with a country code in front, since
// one exporter writes +1 555 0100 and a person typed 555 0100.
func sameValue(kind, a, b string) bool {
	na, nb := normValue(kind, a), normValue(kind, b)
	if na == "" || nb == "" {
		return false
	}
	if na == nb {
		return true
	}
	if kind == "date" {
		// A date with no year does not contradict one with: --05-15 is what
		// Google exports for a birthday whose year it was not told.
		ma, da, ya, oka := parseBirthday(a)
		mb, db, yb, okb := parseBirthday(b)
		return oka && okb && ma == mb && da == db && (ya == 0 || yb == 0)
	}
	if kind == "tel" {
		short, long := na, nb
		if len(short) > len(long) {
			short, long = long, short
		}
		return len(short) >= 7 && strings.HasSuffix(long, short)
	}
	if kind == "url" {
		// A profile written as a url and the same profile as a handle.
		la, lb := lastSegment(na), lastSegment(nb)
		return la != "" && la == lb && (!strings.Contains(na, "/") || !strings.Contains(nb, "/"))
	}
	return false
}

func lastSegment(s string) string {
	if i := strings.LastIndex(s, "/"); i >= 0 {
		return s[i+1:]
	}
	return s
}

// hasValue says whether fields already hold this value under a field of the
// same kind, whatever it is called there.
func hasValue(fields map[string]string, key, value string) bool {
	kind := compareKind(key)
	name, _, _ := fieldOf(key)
	for k, v := range fields {
		if compareKind(k) != kind {
			continue
		}
		// A bare handle only says who somebody is on one network: janeroe
		// on twitter is not the linkedin.com/in/janeroe already there.
		if kind == "url" && (!strings.Contains(v, "/") || !strings.Contains(value, "/")) {
			if n, _, _ := fieldOf(k); n != name {
				continue
			}
		}
		if sameValue(kind, v, value) {
			return true
		}
	}
	return false
}

// add puts one value in under base or base_label, the next free suffix when
// that is taken, and not at all when the card has said it already.
func (c *vcContact) add(base, label, value string) {
	value = vcOneLine(value)
	if value == "" {
		return
	}
	key := base
	if label != "" {
		key += "_" + label
	}
	if hasValue(c.Fields, key, value) {
		return
	}
	key = freeKey(c.Fields, key)
	c.Fields[key] = value
	c.Keys = append(c.Keys, key)
}

// set puts a value in under exactly this key, unless it is there already.
func (c *vcContact) set(key, value string) {
	value = vcOneLine(value)
	if value == "" {
		return
	}
	if _, have := c.Fields[key]; have {
		return
	}
	c.Fields[key] = value
	c.Keys = append(c.Keys, key)
}

func freeKey(fields map[string]string, key string) string {
	if _, taken := fields[key]; !taken {
		return key
	}
	for n := 2; ; n++ {
		k := fmt.Sprintf("%s_%d", key, n)
		if _, taken := fields[k]; !taken {
			return k
		}
	}
}

// A property value is one line; a value that came with line breaks in it (a
// title, a street) has them made into spaces.
func vcOneLine(v string) string {
	return strings.Join(strings.Fields(v), " ")
}

// labelKey turns a label into the part of a property name after the
// underscore: "Old work" -> OLD_WORK.
func labelKey(s string) string {
	var b strings.Builder
	under := false
	for _, r := range strings.TrimSpace(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if under && b.Len() > 0 {
				b.WriteByte('_')
			}
			under = false
			b.WriteRune(unicode.ToUpper(r))
			continue
		}
		under = true
	}
	return b.String()
}

// Apple's built in labels, written _$!<Name>!$_ so that its own app can
// translate them. A custom label is written as typed.
var appleLabelRe = regexp.MustCompile(`^_\$!<(.*)>!\$_$`)

var appleLabels = map[string]string{
	"homepage": "", "mobile": "MOBILE", "iphone": "MOBILE", "main": "MAIN",
	"homefax": "FAX_HOME", "workfax": "FAX_WORK", "otherfax": "FAX_OTHER",
	"pager": "PAGER", "other": "OTHER", "home": "HOME", "work": "WORK",
	"school": "SCHOOL", "anniversary": "ANNIVERSARY",
}

// groupLabel is the X-ABLabel of a property's group, as a label key. ok is
// false when there is no label; a label that maps to nothing (HomePage on a
// url) answers ("", true).
func (c *vCard) groupLabel(p vcProp) (string, bool) {
	if p.Group == "" {
		return "", false
	}
	l, have := c.labels[strings.ToLower(p.Group)]
	if !have || l == "" {
		return "", false
	}
	if m := appleLabelRe.FindStringSubmatch(l); m != nil {
		if v, known := appleLabels[strings.ToLower(m[1])]; known {
			return v, true
		}
		return labelKey(m[1]), true
	}
	return labelKey(l), true
}

// The TYPE values that say nothing a person would label a field with.
var noiseTypes = map[string]bool{
	"pref": true, "internet": true, "x400": true, "voice": true, "msg": true,
	"text": true, "postal": true, "parcel": true, "dom": true, "intl": true,
	"uri": true, "x-internet": true, "bbs": true, "modem": true, "isdn": true,
}

var typeLabels = map[string]string{
	"cell": "MOBILE", "mobile": "MOBILE", "iphone": "MOBILE",
	"home": "HOME", "work": "WORK", "other": "OTHER", "main": "MAIN",
	"pager": "PAGER", "car": "CAR", "video": "VIDEO", "school": "SCHOOL",
	"textphone": "TEXTPHONE",
}

// typeLabel is a label from the TYPEs, place before medium: WORK, MOBILE,
// WORK_MOBILE. Fax is the caller's business (it changes the field, not the
// label).
func typeLabel(p vcProp) string {
	place, medium := "", ""
	for _, t := range p.types() {
		if noiseTypes[t] || t == "fax" {
			continue
		}
		l, known := typeLabels[t]
		if !known {
			if strings.HasPrefix(t, "x-") {
				l = labelKey(t[2:])
			} else if strings.Contains(t, "/") {
				continue
			} else {
				l = labelKey(t)
			}
		}
		switch l {
		case "MOBILE", "PAGER", "VIDEO", "CAR", "TEXTPHONE":
			if medium == "" {
				medium = l
			}
		default:
			if place == "" {
				place = l
			}
		}
	}
	switch {
	case place != "" && medium != "":
		return place + "_" + medium
	case medium != "":
		return medium
	}
	return place
}

// labelOf is the label for one property: Apple's own label when the group
// has one, the TYPEs otherwise.
func (c *vCard) labelOf(p vcProp) string {
	if l, ok := c.groupLabel(p); ok {
		return l
	}
	return typeLabel(p)
}

// byPref orders properties preferred first, keeping file order otherwise.
func byPref(ps []vcProp) []vcProp {
	out := append([]vcProp{}, ps...)
	sort.SliceStable(out, func(a, b int) bool { return out[a].pref() < out[b].pref() })
	return out
}

// Hosts that make a url somebody's profile on a network rather than a website.
var socialHosts = map[string]string{
	"linkedin.com": "LINKEDIN", "twitter.com": "TWITTER", "x.com": "TWITTER",
	"github.com": "GITHUB", "gitlab.com": "GITLAB", "facebook.com": "FACEBOOK",
	"fb.com": "FACEBOOK", "instagram.com": "INSTAGRAM", "bsky.app": "BLUESKY",
	"youtube.com": "YOUTUBE", "tiktok.com": "TIKTOK", "reddit.com": "REDDIT",
	"t.me": "TELEGRAM", "threads.net": "THREADS", "pinterest.com": "PINTEREST",
	"twitch.tv": "TWITCH", "soundcloud.com": "SOUNDCLOUD", "spotify.com": "SPOTIFY",
	"strava.com": "STRAVA", "goodreads.com": "GOODREADS", "keybase.io": "KEYBASE",
	"snapchat.com": "SNAPCHAT", "wa.me": "WHATSAPP", "signal.me": "SIGNAL",
	"discord.com": "DISCORD", "matrix.to": "MATRIX",
}

// parseLooseURL reads "https://x.com/a" and "x.com/a" alike.
func parseLooseURL(v string) *url.URL {
	v = strings.TrimSpace(v)
	if !strings.Contains(v, "://") {
		v = "https://" + v
	}
	u, err := url.Parse(v)
	if err != nil || u.Host == "" {
		return nil
	}
	return u
}

func profilePath(v string) bool {
	u := parseLooseURL(v)
	return u != nil && strings.Trim(u.Path, "/") != ""
}

// networkOfURL is the social network a url is a profile on, or "".
func networkOfURL(v string) string {
	u := parseLooseURL(v)
	if u == nil {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	for {
		if n, ok := socialHosts[host]; ok {
			return n
		}
		i := strings.Index(host, ".")
		if i < 0 {
			return ""
		}
		host = host[i+1:]
	}
}

// IM services, by the name an exporter gives them or the scheme of their uri.
var imServices = map[string]string{
	"skype": "SKYPE", "xmpp": "JABBER", "jabber": "JABBER", "aim": "AIM",
	"icq": "ICQ", "msn": "MSN", "msnim": "MSN", "ymsgr": "YAHOO", "yahoo": "YAHOO",
	"gtalk": "GOOGLE_TALK", "googletalk": "GOOGLE_TALK", "google talk": "GOOGLE_TALK",
	"qq": "QQ", "sip": "SIP", "gadugadu": "GADUGADU", "facebook": "FACEBOOK",
	"facebook messenger": "FACEBOOK", "telegram": "TELEGRAM", "signal": "SIGNAL",
	"whatsapp": "WHATSAPP", "discord": "DISCORD", "slack": "SLACK", "matrix": "MATRIX",
	"wechat": "WECHAT", "line": "LINE", "kik": "KIK", "facetime": "FACETIME",
}

// imKey is the field an IM handle goes in: the network's own when records know
// it (WHATSAPP), IM_<SERVICE> otherwise.
func imKey(service string) string {
	s := strings.ToLower(strings.TrimSpace(service))
	k, ok := imServices[s]
	if !ok {
		k = labelKey(service)
	}
	if k == "" {
		return "IM"
	}
	if socialNetworks[k] != "" {
		return k
	}
	return "IM_" + k
}

// imHandle takes the scheme off an IM uri: skype:jdoe?call -> jdoe,
// aim:goim?screenname=jdoe -> jdoe.
func imHandle(v string) (scheme, handle string) {
	v = strings.TrimSpace(v)
	i := strings.Index(v, ":")
	if i <= 0 || strings.HasPrefix(v[i:], "://") {
		return "", v
	}
	scheme, rest := strings.ToLower(v[:i]), v[i+1:]
	if path, query, has := strings.Cut(rest, "?"); has {
		if q, err := url.ParseQuery(query); err == nil && q.Get("screenname") != "" {
			return scheme, q.Get("screenname")
		}
		rest = path
	}
	if u, err := url.PathUnescape(rest); err == nil {
		rest = u
	}
	return scheme, strings.TrimPrefix(rest, "//")
}

// The legacy one-property-per-service IM lines Google and older exporters
// write.
var legacyIM = map[string]string{
	"X-SKYPE": "skype", "X-SKYPE-USERNAME": "skype", "X-AIM": "aim",
	"X-JABBER": "jabber", "X-ICQ": "icq", "X-MSN": "msn", "X-YAHOO": "yahoo",
	"X-GOOGLE-TALK": "gtalk", "X-GTALK": "gtalk", "X-QQ": "qq",
	"X-GADUGADU": "gadugadu",
}

var omitYearRe = regexp.MustCompile(`^(\d{4})-?(\d{2})-?(\d{2})`)

// vcDate reads BDAY, ANNIVERSARY and X-ABDATE into the form the birthday code
// reads: 1990-05-15, or --05-15 when the year is not known. Apple marks a
// year-less birthday by writing year 1604 with X-APPLE-OMIT-YEAR=1604. A value
// that is not a date (4.0 allows VALUE=text: "circa 1950") is kept as written.
func vcDate(p vcProp) string {
	v := strings.TrimSpace(p.text())
	if v == "" {
		return ""
	}
	if i := strings.IndexAny(v, "Tt"); i > 0 {
		v = v[:i]
	}
	if strings.HasPrefix(v, "--") {
		d := strings.ReplaceAll(v[2:], "-", "")
		if len(d) == 4 {
			return "--" + d[:2] + "-" + d[2:]
		}
		return v
	}
	m := omitYearRe.FindStringSubmatch(v)
	if m == nil {
		return v
	}
	if omit := p.param("X-APPLE-OMIT-YEAR"); omit != "" && omit == m[1] {
		return "--" + m[2] + "-" + m[3]
	}
	return m[1] + "-" + m[2] + "-" + m[3]
}

// vcAddress is one ADR as a line a person would write on an envelope:
// "1 Infinite Loop, Cupertino, CA 95014, United States".
func vcAddress(p vcProp) string {
	parts := p.parts()
	for len(parts) < 7 {
		parts = append(parts, "")
	}
	pobox, ext, street, city, region, code, country := parts[0], parts[1], parts[2], parts[3], parts[4], parts[5], parts[6]
	out := []string{}
	addLines := func(s string) {
		for _, l := range strings.Split(s, "\n") {
			if l = strings.TrimSpace(strings.Trim(strings.TrimSpace(l), ",")); l != "" {
				out = append(out, l)
			}
		}
	}
	addLines(street)
	addLines(ext)
	if pobox != "" {
		if strings.ContainsAny(pobox, "0123456789") && !strings.Contains(strings.ToLower(pobox), "box") {
			pobox = "PO Box " + pobox
		}
		addLines(pobox)
	}
	addLines(city)
	addLines(strings.TrimSpace(region + " " + code))
	addLines(country)
	return strings.Join(out, ", ")
}

// orgTag makes a category into an org tag: letters, digits, _ @ # %, with
// anything else between words made an underscore.
func orgTag(s string) string {
	var b strings.Builder
	under := false
	for _, r := range strings.TrimSpace(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("_@#%", r) {
			if under && b.Len() > 0 {
				b.WriteByte('_')
			}
			under = false
			b.WriteRune(r)
			continue
		}
		under = true
	}
	return b.String()
}

// Groups every contact of one exporter is in, which say nothing about anyone.
var noiseCategories = map[string]bool{"mycontacts": true, "my contacts": true, "* mycontacts": true}

// photoOf turns a PHOTO (or Outlook's X-MS-CARDPICTURE) into a picture to
// save or a url to link.
func (c *vcContact) photoOf(p vcProp) {
	if len(p.Data) > 0 {
		ext := ""
		switch p.MediaType {
		case "image/jpeg", "image/jpg":
			ext = "jpg"
		case "image/png", "image/gif", "image/webp", "image/bmp", "image/heic", "image/tiff":
			ext = strings.TrimPrefix(p.MediaType, "image/")
		}
		sniff := http.DetectContentType(p.Data)
		if ext == "" || !strings.HasPrefix(sniff, "image/") && ext != "heic" {
			// What the bytes are beats what the line said they are.
			switch sniff {
			case "image/jpeg":
				ext = "jpg"
			case "image/png", "image/gif", "image/webp", "image/bmp":
				ext = strings.TrimPrefix(sniff, "image/")
			default:
				if ext == "" {
					c.Warnings = append(c.Warnings, "the photo is not a picture this can read, so it was left out")
					return
				}
			}
		}
		c.Photo = &vcPhoto{Data: p.Data, Ext: ext}
		return
	}
	v := strings.TrimSpace(p.Value)
	if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
		c.PhotoURL = v
	}
}

// contactOf reads one card into a contact.
func contactOf(card *vCard) *vcContact {
	c := &vcContact{Fields: map[string]string{}, Cards: 1}

	// --- the name ------------------------------------------------------------
	c.Name = vcOneLine(card.text("FN"))
	if c.Name == "" {
		if n, ok := card.first("N"); ok {
			parts := n.parts()
			for len(parts) < 5 {
				parts = append(parts, "")
			}
			c.Name = vcOneLine(strings.Join([]string{parts[1], parts[2], parts[0]}, " "))
		}
	}

	if v := card.text("UID"); v != "" {
		c.Uid = strings.TrimPrefix(v, "urn:uuid:")
	} else if v := card.text("X-ABUID"); v != "" {
		c.Uid = v
	}

	for _, p := range card.all("NICKNAME") {
		for _, n := range p.list() {
			c.add("NICKNAME", "", n)
		}
	}
	if o, ok := card.first("ORG"); ok {
		parts := o.parts()
		if len(parts) > 0 {
			c.set("COMPANY", parts[0])
		}
		dept := []string{}
		for _, d := range parts[1:] {
			if d != "" {
				dept = append(dept, d)
			}
		}
		if len(dept) > 0 {
			c.set("DEPARTMENT", strings.Join(dept, ", "))
		}
	}
	if c.Name == "" {
		c.Name = c.Fields["COMPANY"]
	}
	c.set("TITLE", card.text("TITLE"))
	c.set("ROLE", card.text("ROLE"))

	// --- email ---------------------------------------------------------------
	for i, p := range byPref(card.all("EMAIL")) {
		v := strings.TrimSpace(p.text())
		if strings.HasPrefix(strings.ToLower(v), "mailto:") {
			v = v[len("mailto:"):]
		}
		if i == 0 {
			c.add("EMAIL", "", v)
			continue
		}
		c.add("EMAIL", card.labelOf(p), v)
	}
	if c.Name == "" {
		c.Name = c.Fields["EMAIL"]
	}

	// --- phones --------------------------------------------------------------
	for _, p := range card.all("TEL") {
		v := strings.TrimSpace(p.text())
		if strings.HasPrefix(strings.ToLower(v), "tel:") {
			v = v[len("tel:"):]
		}
		label := card.labelOf(p)
		switch {
		case strings.HasPrefix(label, "FAX"):
			c.add("FAX", strings.TrimPrefix(strings.TrimPrefix(label, "FAX"), "_"), v)
		case p.hasType("fax"):
			c.add("FAX", label, v)
		default:
			c.add("PHONE", label, v)
		}
	}

	// --- addresses -----------------------------------------------------------
	for _, p := range byPref(card.all("ADR")) {
		c.add("ADDRESS", card.labelOf(p), vcAddress(p))
	}

	// --- dates ---------------------------------------------------------------
	if p, ok := card.first("BDAY"); ok {
		c.set("BIRTHDAY", vcDate(p))
	}
	for _, name := range []string{"ANNIVERSARY", "X-ANNIVERSARY", "X-MS-ANNIVERSARY", "X-EVOLUTION-ANNIVERSARY"} {
		if p, ok := card.first(name); ok {
			c.set("ANNIVERSARY", vcDate(p))
		}
	}
	for _, p := range card.all("X-ABDATE") {
		l := card.labelOf(p)
		switch l {
		case "ANNIVERSARY":
			c.set("ANNIVERSARY", vcDate(p))
		case "":
			c.add("OTHER_DATE", "", vcDate(p))
		default:
			c.add(l+"_DATE", "", vcDate(p))
		}
	}

	// --- the web -------------------------------------------------------------
	social := func(network, value string) {
		network = strings.ToUpper(strings.TrimSpace(network))
		if socialNetworks[network] != "" {
			c.add(network, "", value)
			return
		}
		if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
			c.add("WEBSITE", labelKey(network), value)
			return
		}
		if k := labelKey(network); k != "" {
			c.add(k, "", value)
		}
	}
	for _, name := range []string{"X-SOCIALPROFILE", "SOCIALPROFILE"} {
		for _, p := range card.all(name) {
			v := strings.TrimSpace(p.text())
			network := strings.ToUpper(firstNonNoise(p.types()))
			if network == "" {
				network = p.param("X-SERVICE-TYPE")
			}
			if network == "" {
				network = networkOfURL(v)
			}
			if !strings.HasPrefix(v, "http://") && !strings.HasPrefix(v, "https://") {
				// Apple writes x-apple:handle when it has no url.
				if u := p.param("X-USER"); u != "" {
					v = u
				} else if _, h := imHandle(v); h != "" {
					v = h
				}
			}
			social(network, v)
		}
	}
	for _, p := range card.all("URL") {
		v := strings.TrimSpace(p.text())
		// A url on a network's host with a path is somebody's profile there;
		// the network's front page is just a website.
		if n := networkOfURL(v); n != "" && profilePath(v) {
			c.add(n, "", v)
			continue
		}
		c.add("WEBSITE", card.labelOf(p), v)
	}
	for _, p := range card.Props {
		if !strings.HasPrefix(p.Name, "X-") {
			continue
		}
		if n := strings.TrimPrefix(p.Name, "X-"); socialNetworks[n] != "" {
			social(n, strings.TrimSpace(p.text()))
		}
		if svc, ok := legacyIM[p.Name]; ok {
			c.add(imKey(svc), "", strings.TrimSpace(p.text()))
		}
	}
	for _, p := range card.all("IMPP") {
		scheme, handle := imHandle(p.text())
		svc := p.param("X-SERVICE-TYPE")
		if svc == "" {
			svc = scheme
		}
		c.add(imKey(svc), "", handle)
	}

	// --- people --------------------------------------------------------------
	for _, name := range []string{"X-ABRELATEDNAMES", "RELATED"} {
		for _, p := range card.all(name) {
			c.add("RELATED", card.labelOf(p), strings.TrimSpace(p.text()))
		}
	}

	// --- the rest ------------------------------------------------------------
	notes := []string{}
	for _, p := range card.all("NOTE") {
		// Quoted-printable carries Outlook's line breaks as =0D=0A, which
		// would leave a carriage return on every line of the org file.
		n := strings.ReplaceAll(strings.ReplaceAll(p.text(), "\r\n", "\n"), "\r", "\n")
		if n = strings.TrimSpace(n); n != "" {
			notes = append(notes, n)
		}
	}
	c.Notes = strings.Join(notes, "\n\n")

	seen := map[string]bool{}
	for _, p := range card.all("CATEGORIES") {
		for _, cat := range p.list() {
			if noiseCategories[strings.ToLower(cat)] {
				continue
			}
			t := orgTag(cat)
			if t == "" || seen[strings.ToLower(t)] {
				continue
			}
			seen[strings.ToLower(t)] = true
			c.Tags = append(c.Tags, t)
		}
	}

	for _, name := range []string{"PHOTO", "X-MS-CARDPICTURE", "LOGO"} {
		if c.Photo != nil || c.PhotoURL != "" {
			break
		}
		if p, ok := card.first(name); ok {
			c.photoOf(p)
		}
	}
	if c.PhotoURL != "" {
		c.set("IMAGE", c.PhotoURL)
	}
	if c.Uid != "" {
		c.set(vcUidField, c.Uid)
	}
	return c
}

func firstNonNoise(ts []string) string {
	for _, t := range ts {
		if !noiseTypes[t] {
			return t
		}
	}
	return ""
}

// contactsFromVCard reads a whole .vcf into contacts, in file order.
func contactsFromVCard(text string) []*vcContact {
	out := []*vcContact{}
	for _, card := range parseVCards(text) {
		out = append(out, contactOf(card))
	}
	return out
}

// absorb folds another reading of the same person into this one: what it
// has that this lacks is added, nothing this has is changed.
func (c *vcContact) absorb(o *vcContact) {
	for _, k := range o.Keys {
		v := o.Fields[k]
		if hasValue(c.Fields, k, v) {
			continue
		}
		if _, taken := c.Fields[k]; taken {
			kind := compareKind(k)
			if kind == "text" || kind == "date" || kind == "image" || k == vcUidField {
				continue
			}
		}
		key := freeKey(c.Fields, k)
		c.Fields[key] = v
		c.Keys = append(c.Keys, key)
	}
	if o.Notes != "" && !notesContain(c.Notes, o.Notes) {
		if c.Notes != "" {
			c.Notes += "\n\n"
		}
		c.Notes += o.Notes
	}
	for _, t := range o.Tags {
		if !vcHasTag(c.Tags, t) {
			c.Tags = append(c.Tags, t)
		}
	}
	if c.Photo == nil && o.Photo != nil {
		c.Photo = o.Photo
	}
	if c.Uid == "" {
		c.Uid = o.Uid
	}
	c.Warnings = append(c.Warnings, o.Warnings...)
	c.Cards += o.Cards
}

func vcHasTag(tags []string, t string) bool {
	for _, x := range tags {
		if strings.EqualFold(x, t) {
			return true
		}
	}
	return false
}

// notesContain says whether a note is already written in some notes, ignoring
// how the lines were wrapped and indented.
func notesContain(have, note string) bool {
	squash := func(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }
	n := squash(note)
	return n == "" || strings.Contains(squash(have), n)
}

// photoSize is a photo's size the way a person reads one: 12 KB.
func photoSize(p *vcPhoto) string {
	n := len(p.Data)
	if n < 1024 {
		return fmt.Sprintf("%d bytes", n)
	}
	return fmt.Sprintf("%d KB", (n+512)/1024)
}

// samePhoto compares two pictures by their bytes.
func samePhoto(a, b []byte) bool { return len(a) > 0 && bytes.Equal(a, b) }
