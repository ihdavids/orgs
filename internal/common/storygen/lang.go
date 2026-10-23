package storygen

import "strings"

// ---- invented languages ----------------------------------------------------

// A language is a handful of sounds and the shapes its syllables take, drawn
// at random from a palette. Every word made from it shares those sounds, so a
// culture's people, places and gods sound related to each other and unlike
// the next culture's.
type language struct {
	onsets []string // consonants that start a syllable
	vowels []string
	codas  []string // consonants that may end one
	shapes []string // C = onset, V = vowel, K = coda
	ends   []string // favourite word endings, a language's habit
	minSyl int
	maxSyl int
}

// Each feel is a family of sounds a language can be drawn from.
var feels = []struct {
	onsets, vowels, codas, ends []string
}{
	{ // soft, liquid
		[]string{"l", "m", "n", "r", "s", "v", "th", "y", "f", "h", "ph", "w"},
		[]string{"a", "e", "i", "o", "ae", "ia", "ie", "y"},
		[]string{"l", "n", "r", "s", "th"},
		[]string{"iel", "wen", "ara", "is", "eth", "ael", "a"},
	},
	{ // hard, stony
		[]string{"k", "g", "d", "t", "b", "r", "z", "kh", "gr", "dr", "br", "th"},
		[]string{"a", "o", "u", "i", "au", "e"},
		[]string{"k", "g", "d", "rk", "n", "m", "rn", "z", "x"},
		[]string{"ak", "un", "or", "ruk", "dum", "ag", "uz"},
	},
	{ // hissing, quick
		[]string{"s", "sh", "z", "ch", "x", "t", "k", "ss", "q", "v"},
		[]string{"i", "e", "a", "ei", "y", "u"},
		[]string{"s", "sh", "x", "t", "k", "z"},
		[]string{"ix", "esh", "a", "ith", "is", "ess"},
	},
	{ // round, open
		[]string{"b", "m", "n", "p", "l", "w", "h", "k", "t", "d"},
		[]string{"a", "o", "u", "oa", "ou", "e", "i"},
		[]string{"n", "m", "l", "ng"},
		[]string{"o", "an", "ulu", "ama", "ono", "u"},
	},
	{ // clipped, northern
		[]string{"h", "s", "t", "v", "g", "b", "k", "sk", "st", "fr", "j", "r"},
		[]string{"a", "e", "i", "o", "y", "ei", "au"},
		[]string{"r", "n", "t", "ld", "rd", "f", "lf", "k"},
		[]string{"a", "ir", "und", "gar", "dis", "ulf", "e"},
	},
	{ // throaty, desert
		[]string{"q", "kh", "h", "z", "d", "r", "s", "m", "n", "j", "gh", "t"},
		[]string{"a", "aa", "i", "u", "ai", "e"},
		[]string{"r", "d", "m", "n", "q", "z", "sh"},
		[]string{"ar", "im", "ah", "id", "an", "uq"},
	},
	{ // sing-song, island
		[]string{"k", "l", "m", "n", "p", "h", "w", "t", "v", "ng"},
		[]string{"a", "e", "i", "o", "u", "ai", "ao"},
		[]string{"n", "ng"},
		[]string{"ani", "olo", "ea", "ika", "ua", "ono"},
	},
	{ // courtly, nasal
		[]string{"b", "d", "f", "l", "m", "n", "r", "s", "v", "gu", "ch", "j"},
		[]string{"a", "e", "i", "o", "au", "eu", "ie"},
		[]string{"n", "l", "r", "s", "nt", "ll"},
		[]string{"ette", "elle", "on", "ais", "and", "ier"},
	},
	{ // whispering, forest
		[]string{"f", "h", "s", "th", "w", "y", "l", "sh", "wh", "n"},
		[]string{"i", "e", "y", "ei", "ae", "u"},
		[]string{"th", "s", "l", "n", "f"},
		[]string{"wyn", "is", "eth", "ith", "ael", "ys"},
	},
	{ // iron, imperial
		[]string{"c", "v", "t", "m", "p", "r", "l", "s", "d", "qu", "g", "n"},
		[]string{"a", "e", "i", "o", "u", "ae"},
		[]string{"s", "x", "n", "m", "r", "l"},
		[]string{"us", "ia", "ax", "ius", "ora", "en"},
	},
	{ // ringing, long
		[]string{"d", "j", "n", "r", "s", "t", "v", "m", "k", "dh", "sh", "y"},
		[]string{"a", "aa", "i", "ii", "u", "e", "o"},
		[]string{"n", "m", "r", "sh", "t"},
		[]string{"ani", "esh", "aru", "indra", "a", "it"},
	},
}

var shapeSets = [][]string{
	{"CV", "CVK", "CV", "V"},
	{"CV", "CV", "CVK"},
	{"CVK", "CV", "VK"},
	{"CV", "V", "CVV"},
}

func (c *ctx) newLanguage() *language {
	// Mostly one feel, sometimes a second mixed in, so two languages from the
	// same feel still differ.
	a := feels[c.r.IntN(len(feels))]
	l := &language{
		onsets: c.pickN(a.onsets, c.between(5, len(a.onsets))),
		vowels: c.pickN(a.vowels, c.between(3, len(a.vowels))),
		codas:  c.pickN(a.codas, c.between(2, len(a.codas))),
		ends:   c.pickN(a.ends, c.between(2, 3)),
		shapes: shapeSets[c.r.IntN(len(shapeSets))],
		minSyl: 2,
		maxSyl: c.between(2, 3),
	}
	if c.chance(40) {
		b := feels[c.r.IntN(len(feels))]
		l.onsets = append(l.onsets, c.pickN(b.onsets, 2)...)
		l.vowels = append(l.vowels, c.pick(b.vowels))
	}
	return l
}

func (c *ctx) syllable(l *language, shape string) string {
	var b strings.Builder
	for _, ch := range shape {
		switch ch {
		case 'C':
			b.WriteString(c.pick(l.onsets))
		case 'V':
			b.WriteString(c.pick(l.vowels))
		case 'K':
			b.WriteString(c.pick(l.codas))
		}
	}
	return b.String()
}

// word is a new word of the language: lower case, tidied of runs no one
// could say. A syllable never follows itself (no "aongaongaon"), a syllable
// of a vowel alone only starts a word, and a word has three letters or more
// and a consonant, or it is made again.
func (c *ctx) word(l *language) string {
	for try := 0; ; try++ {
		n := c.between(l.minSyl, l.maxSyl)
		var b strings.Builder
		prev := ""
		for i := 0; i < n; i++ {
			shape := c.pick(l.shapes)
			for i > 0 && !strings.ContainsRune(shape, 'C') {
				shape = c.pick(l.shapes)
			}
			syl := c.syllable(l, shape)
			for k := 0; k < 4 && syl == prev; k++ {
				syl = c.syllable(l, shape)
			}
			prev = syl
			b.WriteString(syl)
		}
		w := b.String()
		if c.chance(35) {
			w = trimEnd(w) + c.pick(l.ends)
		}
		w = tidy(w)
		if (len(w) >= 3 && hasConsonant(w)) || try >= 8 {
			return w
		}
	}
}

func hasConsonant(w string) bool {
	for _, r := range w {
		if !strings.ContainsRune("aeiouy", r) {
			return true
		}
	}
	return false
}

// name is a word as a name.
func (c *ctx) name(l *language) string {
	w := c.word(l)
	if len(w) > 3 && w[0] == w[1] {
		w = w[1:]
	}
	return capital(w)
}

// trimEnd takes off a trailing vowel so an ending can be put on.
func trimEnd(w string) string {
	if len(w) > 3 && strings.ContainsRune("aeiouy", rune(w[len(w)-1])) {
		return w[:len(w)-1]
	}
	return w
}

// tidy collapses a letter written three times running, three vowels
// running, and three consonants running down to two, and keeps a word to a
// sayable length.
func tidy(w string) string {
	isV := func(r byte) bool { return strings.IndexByte("aeiouy", r) >= 0 }
	out := []byte{}
	for i := 0; i < len(w); i++ {
		ch := w[i]
		n := len(out)
		if n >= 2 && out[n-1] == ch && out[n-2] == ch {
			continue
		}
		if n >= 2 && isV(ch) && isV(out[n-1]) && isV(out[n-2]) {
			continue
		}
		if n >= 2 && !isV(ch) && !isV(out[n-1]) && !isV(out[n-2]) {
			continue
		}
		out = append(out, ch)
	}
	if len(out) > 11 {
		out = out[:11]
		for len(out) > 4 && !isV(out[len(out)-1]) && !isV(out[len(out)-2]) {
			out = out[:len(out)-1]
		}
	}
	return string(out)
}

// ---- names of people -------------------------------------------------------

// personName is a full name in the result's genre. gender is "feminine",
// "masculine", "neutral" or anything else for any of them. Fantasy names come
// from an invented language (l, or a fresh one); a third of science fiction
// names do too, the rest are real names from the tables.
func (c *ctx) personName(l *language, gender string) string {
	if c.genre == "fantasy" || (c.genre == "sf" && c.chance(33)) {
		if l == nil {
			l = c.newLanguage()
		}
		n := c.name(l)
		switch {
		case c.chance(55):
			return n + " " + c.name(l)
		case c.chance(30):
			return n + " " + c.pick(tab("epithets").all())
		}
		return n
	}
	var given []string
	switch gender {
	case "feminine", "masculine", "neutral":
		given = tab("given_" + gender).all()
	default:
		given = append(append(tab("given_feminine").all(), tab("given_masculine").all()...), tab("given_neutral").all()...)
	}
	n := c.pick(given) + " " + c.pick(tab("surnames").all())
	if c.genre == "sf" && c.chance(25) {
		n += " (\"" + c.pick(tab("callsigns").all()) + "\")"
	}
	return n
}

// firstName is the name a person goes by: the first word.
func firstName(full string) string {
	f := strings.Fields(full)
	if len(f) == 0 {
		return full
	}
	return f[0]
}

// ---- names of places -------------------------------------------------------

// englishPlace is an English-style compound: Ash + ford.
func (c *ctx) englishPlace() string {
	return c.pick(tab("place_fronts").all()) + c.pick(tab("place_backs").all())
}

// A place name: invented-language for fantasy (often), an English compound
// otherwise, and the science-fiction habits of naming after people and
// hopes.
func (c *ctx) placeName(l *language) string {
	kinds := tab("sf_place_kinds").all()
	switch c.genre {
	case "fantasy":
		if l != nil && c.chance(65) {
			return c.name(l)
		}
		return c.englishPlace()
	case "sf":
		switch c.r.IntN(5) {
		case 0:
			return "New " + c.englishPlace()
		case 1:
			return c.pick(tab("sf_namesakes").all()) + " " + c.pick(kinds)
		case 2:
			return c.pick(tab("sf_hopes").all()) + " " + c.pick(kinds)
		case 3:
			if l == nil {
				l = c.newLanguage()
			}
			return c.name(l) + " " + c.pick(kinds)
		}
		return c.pick(tab("sf_hopes").all())
	}
	switch c.r.IntN(5) {
	case 0, 1:
		return c.englishPlace() + " " + c.pick(tab("modern_place_suffixes").all())
	case 2:
		return "Port " + c.pick(tab("surnames").all())
	}
	return c.englishPlace()
}
