package orgs

// The endpoints a reader needs for encrypted headings, as opposed to the ones
// crypt.go has for managing them.
//
// crypt.go answers "decrypt this" and "encrypt that". Reading a note that has
// a secret in it asks slightly different questions, and the differences are
// all about not handing out more than was asked for:
//
//   - **Is anything held right now?** A lock indicator asks every few seconds,
//     so `/crypt/state` runs no gpg and reads no file - `/crypt/config` does
//     both.
//   - **Which headings in this file are secret?** `/crypt/file`, metadata only,
//     so a page can put a locked placeholder where each block is.
//   - **Show me this one.** `/crypt/reveal` differs from `/crypt/decrypt` in
//     three ways: it never writes; it holds the passphrase only when the
//     request says `keep`, and only after gpg has accepted it; and it refuses
//     to call gpg at all in symmetric mode with no passphrase, rather than
//     letting gpg go looking for one - which, on a desktop, is a pinentry
//     window popping up on the server's screen.
//   - **Encrypt this one**, from a heading that may not be tagged yet:
//     `/crypt/seal` tags it as well, because an armoured body under a heading
//     without the tag is invisible to the sweep, the listing and Emacs's
//     org-decrypt-entries.
//   - **Save my edit** without the body ever reaching the disk in the clear:
//     `/crypt/edit`, which is `/crypt/update` plus two checks - the passphrase
//     must open what is there now (so a typo cannot re-encrypt a note under a
//     passphrase nobody knows), and what is there must be what the edit started
//     from (so a change made elsewhere is refused rather than overwritten).
//
// A heading is named by hash or by file and headline row. Never by a row
// inside the body: a reveal hands over plaintext and must land on exactly the
// heading somebody clicked.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// registerCryptRoutes adds the endpoints for revealing encrypted headings. Called from RestApi.
func registerCryptRoutes(api *mux.Router) {
	api.HandleFunc("/crypt/state", RequestCryptState).Methods("GET")
	api.HandleFunc("/crypt/file", RequestCryptFile).Methods("GET")
	api.HandleFunc("/crypt/reveal", PostCryptReveal).Methods("POST")
	api.HandleFunc("/crypt/seal", PostCryptSeal).Methods("POST")
	api.HandleFunc("/crypt/edit", PostCryptEdit).Methods("POST")
}

func cryptStateNow() common.CryptState {
	st := common.CryptState{Ok: true, Tag: cryptTag(), UnlockSeconds: cryptSettings().UnlockSeconds}
	switch {
	case gpgBin() == "":
		st.Mode = "off"
		st.Msg = "no gpg on this machine"
	case strings.TrimSpace(cryptSettings().Key) != "":
		st.Mode = "key"
	default:
		st.Mode = "symmetric"
	}
	if _, left := unlocked(); left > 0 {
		st.Unlocked = true
		st.UnlockIn = left
	}
	if st.UnlockSeconds < 0 {
		st.UnlockSeconds = 0
	}
	return st
}

/* SDOC: API
* GET /crypt/state — Is a Passphrase Held Right Now

	The cheap half of =/crypt/config=: no gpg is run and no file is read, so a
	lock indicator can ask every few seconds.

	*Method:* =GET=

	*Response:* A =CryptState= JSON object.
	| Field           | Description                                                    |
	|-----------------+----------------------------------------------------------------|
	| =Mode=          | =off=, =symmetric= or =key=                                    |
	| =Tag=           | The tag that marks a heading for encryption                    |
	| =Unlocked=      | Whether a passphrase is held, and =UnlockIn= for how long      |
	| =UnlockSeconds= | =crypt.unlockSeconds=: zero means nothing is ever held         |
EDOC */
func RequestCryptState(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	cryptJson(w, cryptStateNow())
}

/* SDOC: API
* GET /crypt/file — The Secret Headings in One File

	Every heading in one file carrying the crypt tag, in the order they are
	written, and whether each is ciphertext right now. Metadata only, as
	=/crypt/headings= is: what is secret, never what the secret is.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter  | Type   | Required | Description                     |
	|------------+--------+----------+---------------------------------|
	| =filename= | string | yes      | The file, as =/files= names it  |

	*Response:* A =CryptHeadings= JSON object.
EDOC */
func RequestCryptFile(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	fname, err := resolveRequestedFile(r.URL.Query().Get("filename"))
	if err != nil {
		cryptJson(w, common.CryptHeadings{Msg: err.Error(), Headings: []common.CryptHeading{}})
		return
	}
	res := common.CryptHeadings{Ok: true, Headings: cryptHeadingsOf(fname)}
	for _, h := range res.Headings {
		res.Tagged++
		if !h.Encrypted {
			res.Plain++
		}
	}
	cryptJson(w, res)
}

// The heading a reveal, seal or edit names.
func cryptOpTarget(req common.CryptOp) (*org.Section, *common.OrgFile, error) {
	if strings.TrimSpace(req.Hash) != "" {
		return cryptTarget(strings.ReplaceAll(strings.TrimSpace(req.Hash), " ", "+"))
	}
	if strings.TrimSpace(req.Filename) == "" {
		return nil, nil, fmt.Errorf("name a heading by hash, or by filename and the line its headline is on")
	}
	fname, err := resolveRequestedFile(req.Filename)
	if err != nil {
		return nil, nil, err
	}
	f := GetDb().FindByFile(fname)
	if f == nil || f.Doc == nil {
		return nil, nil, fmt.Errorf("file not found in the database: %s", fname)
	}
	sec := findHeadingByLine(f, req.Line)
	// Exactly the headline's row. A row in the body would resolve to the
	// heading above it, which is a guess - and a reveal must not guess.
	if sec == nil || headlineRow(sec) != req.Line {
		return nil, nil, fmt.Errorf("no heading starts on line %d of %s", req.Line+1, fname)
	}
	GetDb().RegisterSection(sec.Hash, sec, f)
	return sec, f, nil
}

func readCryptOp(r *http.Request) (common.CryptOp, error) {
	var req common.CryptOp
	// The same cap crypt.go puts on a request; the shape differs.
	raw, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		return req, err
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		return req, err
	}
	return req, nil
}

// Is this heading encrypted to a passphrase rather than to a key? Only then
// is "no passphrase" known to be hopeless before gpg is asked.
func cryptSymmetric(sec *org.Section) bool {
	return cryptKeyFor(sec) == ""
}

// gpg's words for "that is not the passphrase", which a client wants to tell
// apart from every other failure: one means "try again", the rest mean "stop".
func isWrongPassphrase(err error) bool {
	if err == nil {
		return false
	}
	m := strings.ToLower(err.Error())
	return strings.Contains(m, "bad session key") || strings.Contains(m, "bad passphrase") ||
		strings.Contains(m, "no secret key")
}

func plainSum(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// Fill in what every answer about a heading carries.
func cryptOpResult(sec *org.Section, f *common.OrgFile) common.CryptOpResult {
	res := common.CryptOpResult{}
	if sec != nil {
		res.Hash = sec.Hash
		res.Headline = sectionTitle(sec)
		res.Line = headlineRow(sec)
	}
	if f != nil && f.Doc != nil {
		res.Filename = f.Doc.Path
	}
	return res
}

func cryptOpFinish(w http.ResponseWriter, res common.CryptOpResult) {
	if _, left := unlocked(); left > 0 {
		res.Unlocked = true
		res.UnlockIn = left
	}
	cryptJson(w, res)
}

// The passphrase a request will use, or none - and in symmetric mode, none
// means stopping here rather than letting gpg go and look for one.
func cryptOpPass(req common.CryptOp, sec *org.Section) (string, bool) {
	pass := passFor(req.Passphrase)
	if pass == "" && cryptSymmetric(sec) {
		return "", false
	}
	return pass, true
}

/* SDOC: API
* POST /crypt/reveal — Show One Encrypted Heading

	Hands back the plaintext of one encrypted heading, and optionally its
	rendering. Writes nothing, ever.

	*Method:* =POST=

	*Request Body:* A =CryptOp=.
	| Field        | Type   | Required | Description                                             |
	|--------------+--------+----------+---------------------------------------------------------|
	| =hash=       | string | or file  | The heading                                             |
	| =filename=   | string | or hash  | The file, with =line=, the headline's zero based row    |
	| =passphrase= | string | no       | Unless one is held. Never stored unless =keep=          |
	| =keep=       | bool   | no       | Hold it for =crypt.unlockSeconds=, once it has worked   |
	| =html=       | bool   | no       | Also render the plaintext with the html exporter        |

	*Response:* A =CryptOpResult= carrying =Text=, =Base= (its sha256, for an
	edit) and, when asked, =Html=. =NeedPassphrase= means nothing was tried;
	=WrongPassphrase= means gpg refused the one given.

	Unlike =/crypt/decrypt=, a passphrase is held only when =keep= asks for it
	and only after it has opened the heading - a mistyped one is never held -
	and in symmetric mode with no passphrase gpg is not called at all.
EDOC */
func PostCryptReveal(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	req, err := readCryptOp(r)
	if err != nil {
		cryptJson(w, common.CryptOpResult{Msg: err.Error()})
		return
	}

	cryptLock.Lock()
	defer cryptLock.Unlock()

	sec, f, err := cryptOpTarget(req)
	if err != nil {
		cryptJson(w, common.CryptOpResult{Msg: err.Error()})
		return
	}
	res := cryptOpResult(sec, f)
	res.Encrypted = true
	pass, ok := cryptOpPass(req, sec)
	if !ok {
		res.NeedPassphrase = true
		res.Msg = "locked: a passphrase is needed to read this"
		cryptOpFinish(w, res)
		return
	}
	text, err := readHeading(sec, f, pass)
	if err != nil {
		if strings.Contains(err.Error(), "not encrypted") {
			res.Encrypted = false
		}
		if isWrongPassphrase(err) {
			res.WrongPassphrase = true
			// A held passphrase that does not open this one - it was
			// encrypted under another - is a question for the person, not
			// a dead end.
			res.NeedPassphrase = req.Passphrase == ""
			res.Msg = "that passphrase does not open this heading"
		} else {
			res.Msg = err.Error()
		}
		cryptOpFinish(w, res)
		return
	}
	if req.Keep {
		holdAfter(req.Passphrase)
	}
	res.Ok = true
	res.Msg = "revealed"
	res.Text = text
	res.Base = plainSum(text)
	if req.Html {
		res.Html = drillRender(text, f.Doc.Path)
	}
	cryptOpFinish(w, res)
}

/* SDOC: API
* POST /crypt/seal — Encrypt One Heading, Tagging It If Need Be

	=/crypt/encrypt= for a heading somebody has just decided is secret: it
	encrypts the body and puts the crypt tag on the headline if it is not
	there. Without the tag an armoured body is invisible to =/crypt/headings=,
	to the sweep and to Emacs's =org-decrypt-entries=.

	*Method:* =POST=

	*Request Body:* A =CryptOp=: =hash= or =filename= + =line=, =passphrase=
	(symmetric mode, unless one is held) and =keep=.

	*Response:* A =CryptOpResult=. =Hash= is the heading's hash afterwards,
	since tagging the headline can change it.

	Everything crypt.go promises holds: the ciphertext is decrypted and
	compared before a byte is written, and the file is replaced atomically.
	The tag is written after the encryption, so a failed encryption leaves the
	file exactly as it was.
EDOC */
func PostCryptSeal(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	req, err := readCryptOp(r)
	if err != nil {
		cryptJson(w, common.CryptOpResult{Msg: err.Error()})
		return
	}

	cryptLock.Lock()
	defer cryptLock.Unlock()

	sec, f, err := cryptOpTarget(req)
	if err != nil {
		cryptJson(w, common.CryptOpResult{Msg: err.Error()})
		return
	}
	res := cryptOpResult(sec, f)
	pass, ok := cryptOpPass(req, sec)
	if !ok {
		res.NeedPassphrase = true
		res.Msg = "a passphrase is needed to encrypt this"
		cryptOpFinish(w, res)
		return
	}
	row := headlineRow(sec)
	filename := f.Doc.Path
	changed, err := encryptHeading(sec, f, pass)
	if err != nil {
		res.Msg = err.Error()
		cryptOpFinish(w, res)
		return
	}
	if changed && req.Keep {
		holdAfter(req.Passphrase)
	}

	// The tag. Encrypting changed only rows below the headline's drawers, so
	// the headline is still on the row it was.
	tagged := false
	if !isCrypt(sec) {
		lines, rerr := readOrgLines(filename)
		if rerr == nil && row >= 0 && row < len(lines) {
			if line, did := addHeadlineTag(lines[row], cryptTag()); did {
				lines[row] = line
				if werr := writeFileAtomic(filename, lines); werr != nil {
					res.Msg = "encrypted, but the tag could not be written: " + werr.Error()
					cryptOpFinish(w, res)
					return
				}
				GetDb().ReloadFile(filename)
				tagged = true
			}
		}
	}

	// Found again: the file has been rewritten, and the hash may have moved.
	if nf := GetDb().FindByFile(filename); nf != nil {
		if ns := findHeadingByLine(nf, row); ns != nil && headlineRow(ns) == row {
			GetDb().RegisterSection(ns.Hash, ns, nf)
			res.Hash = ns.Hash
			res.Headline = sectionTitle(ns)
		}
	}
	res.Ok = true
	switch {
	case changed:
		res.Encrypted = true
		res.Msg = "encrypted"
	case lineIsArmoredUnder(filename, row):
		res.Encrypted = true
		res.Msg = "already encrypted"
	default:
		res.Msg = "nothing is written under it yet, so there is nothing to encrypt"
	}
	if tagged {
		res.Msg += ", and tagged :" + cryptTag() + ":"
	}
	cryptOpFinish(w, res)
}

// Whether the heading on a row has ciphertext for a body, read off the file.
func lineIsArmoredUnder(filename string, row int) bool {
	f := GetDb().FindByFile(filename)
	if f == nil {
		return false
	}
	sec := findHeadingByLine(f, row)
	if sec == nil {
		return false
	}
	lines, from, to, ok := recordLines(filename, sec)
	if !ok {
		return false
	}
	start, end := cryptRange(lines, from, to)
	if start > end || start >= len(lines) {
		return false
	}
	return isArmored(lines[start : end+1])
}

/* SDOC: API
* POST /crypt/edit — Change an Encrypted Heading You Have Read

	Replaces an encrypted heading's body with new text, encrypted, without the
	text ever being written to disk in the clear. =/crypt/update= with two
	checks before anything is written:

	1. the passphrase must open what is there now, so a typo cannot quietly
	   re-encrypt the note under a passphrase nobody knows;
	2. what is there must be what the edit started from (=base=, the =Base= a
	   reveal answered with), so a change made somewhere else since is refused
	   rather than overwritten.

	*Method:* =POST=

	*Request Body:* A =CryptOp=: the heading, =text=, =base=, =passphrase=
	(unless one is held) and =keep=.

	*Response:* A =CryptOpResult=, with the new =Base=.
EDOC */
func PostCryptEdit(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	req, err := readCryptOp(r)
	if err != nil {
		cryptJson(w, common.CryptOpResult{Msg: err.Error()})
		return
	}

	cryptLock.Lock()
	defer cryptLock.Unlock()

	sec, f, err := cryptOpTarget(req)
	if err != nil {
		cryptJson(w, common.CryptOpResult{Msg: err.Error()})
		return
	}
	res := cryptOpResult(sec, f)
	res.Encrypted = true
	text := strings.TrimRight(req.Text, "\n")
	if strings.TrimSpace(text) == "" {
		// An empty secret is an armoured block saying there is one where there
		// is not. Deleting the body is a different act, and not this one.
		res.Msg = "the new text is empty; nothing was written"
		cryptOpFinish(w, res)
		return
	}
	if strings.TrimSpace(req.Base) == "" {
		res.Msg = "an edit names what it started from (base); reveal the heading first"
		cryptOpFinish(w, res)
		return
	}
	pass, ok := cryptOpPass(req, sec)
	if !ok {
		res.NeedPassphrase = true
		res.Msg = "a passphrase is needed to save this"
		cryptOpFinish(w, res)
		return
	}
	now, err := readHeading(sec, f, pass)
	if err != nil {
		if isWrongPassphrase(err) {
			res.WrongPassphrase = true
			res.NeedPassphrase = req.Passphrase == ""
			res.Msg = "that passphrase does not open this heading, so nothing was written"
		} else {
			res.Msg = err.Error()
		}
		cryptOpFinish(w, res)
		return
	}
	if plainSum(now) != strings.TrimSpace(req.Base) {
		res.Msg = "this heading has changed since it was revealed; reveal it again before editing"
		cryptOpFinish(w, res)
		return
	}
	armor, err := encryptBody(text, cryptKeyFor(sec), pass)
	if err != nil {
		res.Msg = err.Error()
		cryptOpFinish(w, res)
		return
	}
	if err := spliceArmor(sec, f, armor); err != nil {
		res.Msg = err.Error()
		cryptOpFinish(w, res)
		return
	}
	if req.Keep {
		holdAfter(req.Passphrase)
	}
	res.Ok = true
	res.Msg = "saved, encrypted"
	// What gpg will hand back next time: the text as written, which is what
	// a following edit has to name as its base.
	res.Base = plainSum(text)
	cryptOpFinish(w, res)
}
