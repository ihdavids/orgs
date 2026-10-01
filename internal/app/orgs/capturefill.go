//lint:file-ignore ST1006 allow the use of self
package orgs

import (
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

// Answering, on the way out, the parts of a capture template that answer
// themselves.
//
// A capture template's `template:` string is a form for a client to put up -
// the server has never filed it, and the type says so. That is still true of
// the parts a person has to fill in. It is not true of the parts nobody can
// answer differently: `{{uuid}}` is a number out of a hat, `{{username}}` is
// who asked, and `{{now}}` is what time it is on the machine holding the org
// files, which is the clock the rest of the database is kept by.
//
// So those are answered here, and the answer is written into the placeholder as
// its **default** rather than over the top of it:
//
//	:CUSTOM_ID: {{uuid}}        ->  :CUSTOM_ID: {{uuid|=f81d4fae-7dec-...}}
//	:CREATED:   {{now}}         ->  :CREATED:   {{now|=[2026-09-30 Wed 17:20]}}
//	:WHEN: {{now|When was it}}  ->  :WHEN: {{now|When was it|=[2026-09-30 ...]}}
//
// which is the whole point of doing it here at all:
//
//   - a client that knows nothing about any of this has a usable value in the
//     box already, so `:CUSTOM_ID: {{uuid}}` works without a line of client
//     code - and `crypto.randomUUID` is secure-context only in a browser, the
//     same wall `clienthash.ts` runs into, so for worg served over plain http
//     from another machine this is the *only* end that can answer it;
//   - a client that wants to answer it itself still sees the name and can
//     ignore the default - which is what a substitution would have taken away.
//     A literal value in the template is a value nobody can change, and the
//     timestamp on a capture is very often the one thing somebody does change.
//
// The grammar, the names and both halves of the arithmetic live in
// `internal/common/captemplate.go`, so the end that expands and the end that
// fills in do not have private understandings of the same string. This file is
// only the wiring: which answers go out, and to whom.
//
// Two things about the lifetime, neither of them obvious:
//
//   - These are answered when the template list is **asked for**, not when the
//     capture is made. worg re-asks every time its capture dialog opens and
//     `orgs cap` asks once per run, so a uuid is fresh per capture; a client
//     that holds one list open all day and captures from it twice would offer
//     the same uuid twice.
//   - `/ext/capture/templates` - the endpoint a user *edits* their own
//     templates through - is deliberately not expanded. It hands back the
//     template as written, or saving one back would bake this afternoon into it
//     for ever.

// FillCaptureTemplateAutos expands every template on its way to a client. Each
// is expanded on its own, so two of them asking for {{uuid}} get two uuids, and
// the list shares one reading of the clock, so a list fetched a tick before
// midnight does not come back half on each day.
func FillCaptureTemplateAutos(temps []common.CaptureTemplate, username string) []common.CaptureTemplate {
	now := time.Now()
	for i := range temps {
		if temps[i].Template == "" {
			continue
		}
		temps[i].Template = common.CapExpandAutos(temps[i].Template, now, username)
	}
	return temps
}
