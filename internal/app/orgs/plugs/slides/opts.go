package slides

// Building the javascript options object a framework is initialised with.
//
// The rule here is **say only what was asked for**. Every one of these
// libraries has sensible defaults and changes them between versions; writing
// out all forty of them pins this exporter to the version it was written
// against and silently overrides whatever a theme wanted. So an option appears
// in the object only when the org file said something about it.
//
// It is built here rather than in the template because a javascript object with
// template conditionals inside it is a syntax error waiting for the first deck
// that sets two of them - a missing comma nobody sees until a presentation does
// not start.

import (
	"fmt"
	"strconv"
	"strings"
)

// JSOpts is a javascript object literal under construction.
type JSOpts struct {
	c     Conf
	pairs []string
}

func NewJSOpts(c Conf) *JSOpts { return &JSOpts{c: c} }

// Raw writes a key with the javascript given, for the handful that are neither
// a string nor a number.
func (o *JSOpts) Raw(key, js string) *JSOpts {
	o.pairs = append(o.pairs, fmt.Sprintf("%s: %s", key, js))
	return o
}

// Bool writes a boolean option if the document said anything about it.
func (o *JSOpts) Bool(name, key string) *JSOpts {
	if v := o.c.DocStr(name, "\x00"); v != "\x00" {
		return o.Raw(key, strconv.FormatBool(Truthy(v)))
	}
	return o
}

// Str writes a string option if the document said anything about it.
func (o *JSOpts) Str(name, key string) *JSOpts {
	if v := o.c.DocStr(name, ""); v != "" {
		return o.Raw(key, quote(v))
	}
	return o
}

// Num writes a numeric option if the document said a number. A value that is
// not a number is left out rather than passed through: `width: banana` stops
// the whole presentation, and the mistake is worth less than the deck.
func (o *JSOpts) Num(name, key string) *JSOpts {
	v := o.c.DocStr(name, "")
	if v == "" {
		return o
	}
	if _, err := strconv.ParseFloat(v, 64); err == nil {
		return o.Raw(key, v)
	}
	// A percentage or a css length is a legal width for some of these.
	if strings.HasSuffix(v, "%") || strings.HasSuffix(v, "px") {
		return o.Raw(key, quote(v))
	}
	return o
}

// NumOrBool is for the options that take either, like reveal's autoSlide: a
// number of milliseconds, or false.
func (o *JSOpts) NumOrBool(name, key string) *JSOpts {
	v := o.c.DocStr(name, "")
	if v == "" {
		return o
	}
	if _, err := strconv.ParseFloat(v, 64); err == nil {
		return o.Raw(key, v)
	}
	return o.Raw(key, strconv.FormatBool(Truthy(v)))
}

// Has reports whether anything was written at all, so a caller can leave the
// whole initialiser off.
func (o *JSOpts) Has() bool { return len(o.pairs) > 0 }

func (o *JSOpts) String() string {
	if len(o.pairs) == 0 {
		return "{}"
	}
	return "{\n  " + strings.Join(o.pairs, ",\n  ") + "\n}"
}

// Join adds the pairs of another set, for an option group built elsewhere.
func (o *JSOpts) Join(other *JSOpts) *JSOpts {
	o.pairs = append(o.pairs, other.pairs...)
	return o
}

func quote(s string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`, "\n", `\n`).Replace(s) + "'"
}
