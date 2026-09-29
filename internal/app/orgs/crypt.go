package orgs

// org-crypt: a heading tagged `:crypt:` whose body is ciphertext on disk.
//
// The format is Emacs org-crypt's, exactly: the heading keeps its line, its
// planning line and its property drawer in the clear, and everything from there
// to the end of the subtree is replaced by an ASCII armoured gpg message. A
// heading encrypted here opens in Emacs and one encrypted in Emacs opens here.
//
// Three decisions carry this file, and each of them is about what the feature
// is for rather than about how to call gpg.
//
//  1. **The server holds no passphrase.** One arrives with the request that
//     needs it, is handed to gpg down a pipe, and is forgotten. That is what
//     makes "the server is reachable from more than this machine" survivable:
//     reaching it gains an attacker ciphertext. `crypt.unlockSeconds` will hold
//     one in memory if asked, and says in its own name what it costs.
//
//  2. **The property drawer stays in the clear**, as org-crypt leaves it. A
//     heading's `:ID:` is how links, attachments and every other index reach
//     it; encrypting that would break the database to hide a uuid. What is
//     secret is what was written, and what was written is the body.
//
//  3. **Nothing is written until the round trip has been checked.** Every
//     encrypt decrypts its own output and compares it with what it was given
//     before a single byte goes to disk. Encryption that half worked is not a
//     failed save, it is a destroyed note, and there is no undo for a body
//     nobody can read.
//
// Two smaller things that are not obvious and are load bearing:
//
//   - **The armour goes at column zero.** gpg refuses an indented one outright -
//     "no valid OpenPGP data found" - so the indent the rest of this server
//     writes bodies at would make every encrypted heading undecryptable by gpg,
//     by Emacs, and by this. Reading strips a common indent anyway, so a file
//     somebody indented by hand still opens.
//   - **The whole subtree is encrypted**, child headings included, which is what
//     org-crypt does. Encrypting only the heading's own body would leave its
//     children sitting in the clear underneath it.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
)

// cryptLock serialises the read/modify/write cycle.
var cryptLock sync.Mutex

const armorBegin = "-----BEGIN PGP MESSAGE-----"
const armorEnd = "-----END PGP MESSAGE-----"

// How long gpg is given. A body is kilobytes, so this is not about the work -
// it is about a gpg that has stopped to ask something at a prompt nobody can
// see, which would otherwise hold the request open forever.
const gpgTimeout = 30 * time.Second

// ---------------------------------------------------------------------------
// Settings
// ---------------------------------------------------------------------------

func cryptSettings() common.CryptSettings {
	if Conf().Server == nil {
		return common.CryptSettings{}
	}
	return Conf().Server.Crypt
}

func cryptTag() string {
	t := strings.TrimSpace(cryptSettings().Tag)
	if t == "" {
		return "crypt"
	}
	return strings.ToLower(t)
}

// The gpg to use, and whether there is one. Everything here is off without it,
// and says so rather than failing one request at a time.
func gpgBin() string {
	if b := strings.TrimSpace(cryptSettings().Bin); b != "" {
		if _, err := os.Stat(b); err == nil {
			return b
		}
		if p, err := exec.LookPath(b); err == nil {
			return p
		}
		return ""
	}
	for _, name := range []string{"gpg", "gpg2"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// The unlock
// ---------------------------------------------------------------------------

// A passphrase held in memory, if the configuration asked for one to be.
//
// Off by default and deliberately awkward to turn on: it is the one thing in
// here that trades the feature's main property away, so it is a number of
// seconds rather than a switch.
var (
	unlockPass string
	unlockTill time.Time
	unlockMu   sync.Mutex
)

func unlockFor(pass string) int {
	secs := cryptSettings().UnlockSeconds
	if secs <= 0 || pass == "" {
		return 0
	}
	unlockMu.Lock()
	defer unlockMu.Unlock()
	unlockPass = pass
	unlockTill = time.Now().Add(time.Duration(secs) * time.Second)
	return secs
}

func unlocked() (string, int) {
	unlockMu.Lock()
	defer unlockMu.Unlock()
	if unlockPass == "" || time.Now().After(unlockTill) {
		unlockPass = ""
		return "", 0
	}
	return unlockPass, int(time.Until(unlockTill).Seconds())
}

func relock() {
	unlockMu.Lock()
	defer unlockMu.Unlock()
	unlockPass = ""
	unlockTill = time.Time{}
}

// The passphrase to use: the one sent with the request, or the one held if the
// configuration allows holding one. A request that carries one refreshes it.
func passFor(sent string) string {
	if sent != "" {
		unlockFor(sent)
		return sent
	}
	held, _ := unlocked()
	return held
}

// ---------------------------------------------------------------------------
// Calling gpg
// ---------------------------------------------------------------------------

// Run gpg over some bytes.
//
// The passphrase goes down a pipe on fd 3 rather than on the command line,
// because a command line is world readable on every machine this might run on -
// `ps` would show it to anybody with a shell. `--batch` and
// `--pinentry-mode loopback` are what stop gpg trying to open a prompt on a
// terminal that is not there.
func runGpg(args []string, in []byte, pass string) ([]byte, error) {
	bin := gpgBin()
	if bin == "" {
		return nil, fmt.Errorf("no gpg on this machine, so nothing here can encrypt")
	}
	full := []string{"--batch", "--yes", "--quiet", "--no-tty"}
	if pass != "" {
		full = append(full, "--pinentry-mode", "loopback", "--passphrase-fd", "3")
	}
	full = append(full, args...)

	cmd := exec.Command(bin, full...)
	cmd.Stdin = bytes.NewReader(in)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb

	if pass != "" {
		r, w, err := os.Pipe()
		if err != nil {
			return nil, err
		}
		// ExtraFiles[0] is the child's fd 3.
		cmd.ExtraFiles = []*os.File{r}
		go func() {
			defer w.Close()
			io.WriteString(w, pass)
		}()
		defer r.Close()
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			// gpg's own words, which say "bad passphrase" where that is what
			// happened - and never the passphrase itself.
			msg := strings.TrimSpace(errb.String())
			if msg == "" {
				msg = err.Error()
			}
			// gpg prefixes its own lines with "gpg:", so prefixing again reads
			// as "gpg: gpg: ...".
			msg = strings.TrimPrefix(firstLines(msg, 3), "gpg: ")
			return nil, fmt.Errorf("gpg: %s", msg)
		}
	case <-time.After(gpgTimeout):
		_ = cmd.Process.Kill()
		<-done
		return nil, fmt.Errorf("gpg did not answer in %s - it may be waiting at a prompt nothing can see", gpgTimeout)
	}
	return out.Bytes(), nil
}

func firstLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "; ")
}

// Encrypt, and check the result decrypts back to what we were given before
// anybody is told it worked.
//
// The check is not belt and braces. A wrong key, a gpg that wrote a warning
// where the ciphertext should be, a passphrase with a stray newline - each of
// those produces output that looks like an answer, and writing it would replace
// somebody's note with something nobody can ever read again. There is no undo
// for that.
func encryptBody(plain string, key string, pass string) (string, error) {
	var args []string
	if key != "" {
		args = []string{"--armor", "--encrypt", "--recipient", key, "--trust-model", "always"}
	} else {
		if pass == "" {
			return "", fmt.Errorf("symmetric encryption needs a passphrase")
		}
		args = []string{"--armor", "--symmetric"}
	}
	out, err := runGpg(args, []byte(plain), pass)
	if err != nil {
		return "", err
	}
	armor := strings.TrimRight(string(out), "\n")
	if !strings.HasPrefix(armor, armorBegin) {
		return "", fmt.Errorf("gpg did not produce an armoured message")
	}
	// In key mode the server cannot check its own work: it has no private key,
	// which is exactly the property that mode exists for. The armour itself is
	// what can be checked, and is.
	if key == "" {
		back, derr := decryptBody(armor, pass)
		if derr != nil {
			return "", fmt.Errorf("the encryption could not be read back, so nothing was written: %s", derr)
		}
		if back != plain {
			return "", fmt.Errorf("the encryption did not read back the same, so nothing was written")
		}
	}
	return armor, nil
}

func decryptBody(armor string, pass string) (string, error) {
	out, err := runGpg([]string{"--decrypt"}, []byte(armor+"\n"), pass)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// ---------------------------------------------------------------------------
// Finding the secret part of a heading
// ---------------------------------------------------------------------------

// Does this heading carry the tag? Its own tag, not one inherited from a
// parent: encrypting every heading under a `:crypt:` project because the
// project said so is not what anybody meant by tagging the project.
func isCrypt(sec *org.Section) bool {
	return HeadlineAloneHasTag(cryptTag(), sec)
}

// The key this heading is encrypted to: its own :CRYPTKEY: property, then the
// server's. org-crypt reads the property the same way.
func cryptKeyFor(sec *org.Section) string {
	if v := strings.TrimSpace(GetProp(sec, "CRYPTKEY", "CryptKey", "cryptkey")); v != "" {
		return v
	}
	return strings.TrimSpace(cryptSettings().Key)
}

// The rows that are the heading's secret: everything after its planning line
// and its drawers, to the end of its subtree.
//
// The whole subtree, child headings included, because that is what org-crypt
// encrypts - and because encrypting only the heading's own body would leave
// everything written under it sitting in the clear.
func cryptRange(lines []string, from, to int) (start int, end int) {
	start = headEnd(lines, from, to)
	return start, to
}

// Is what is written there already ciphertext?
func isArmored(body []string) bool {
	for _, l := range body {
		t := strings.TrimSpace(l)
		if t == "" {
			continue
		}
		return strings.HasPrefix(t, armorBegin)
	}
	return false
}

// The armoured message as gpg will take it: the block only, with any indent
// taken off.
//
// The indent matters more than it looks. gpg refuses an indented armour
// outright - "no valid OpenPGP data found" - so a file whose block somebody has
// re-indented, by hand or by a formatter, is one nothing can decrypt until the
// indent comes off. Taking it off here costs nothing and is the difference
// between a note being readable and being lost.
func armorOf(body []string) string {
	out := []string{}
	in := false
	for _, l := range body {
		t := strings.TrimSpace(l)
		if !in {
			if strings.HasPrefix(t, armorBegin) {
				in = true
				out = append(out, t)
			}
			continue
		}
		out = append(out, t)
		if strings.HasPrefix(t, armorEnd) {
			break
		}
	}
	return strings.Join(out, "\n")
}

// A body as lines, with the trailing blank ones taken off - gpg's output ends
// with a newline and the splice adds its own.
func bodyLines(text string) []string {
	return strings.Split(strings.TrimRight(text, "\n"), "\n")
}

// Write a file without ever letting plaintext reach the disk under its own
// name.
//
// Through a temporary file in the same directory and a rename, which is atomic
// on every filesystem this runs on: a reader either gets the whole old file or
// the whole new one, and a crash between the two leaves the old one. The same
// directory matters - a rename across filesystems is a copy, and a copy is not
// atomic.
func writeFileAtomic(filename string, lines []string) error {
	dir := filepath.Dir(filename)
	tmp, err := os.CreateTemp(dir, ".orgs-crypt-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0644); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(strings.Join(lines, "\n") + "\n"); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, filename)
}

// ---------------------------------------------------------------------------
// The work
// ---------------------------------------------------------------------------

// Encrypt one heading in place. Answers with whether anything changed.
//
// Called with cryptLock held.
func encryptHeading(sec *org.Section, f *common.OrgFile, pass string) (changed bool, err error) {
	if f == nil || f.Doc == nil {
		return false, fmt.Errorf("that heading has no file")
	}
	filename := f.Doc.Path
	lines, from, to, ok := recordLines(filename, sec)
	if !ok {
		return false, fmt.Errorf("could not read %s", filename)
	}
	start, end := cryptRange(lines, from, to)
	body := []string{}
	if start <= end && start < len(lines) {
		if end >= len(lines) {
			end = len(lines) - 1
		}
		body = lines[start : end+1]
	}
	if isArmored(body) {
		return false, nil
	}
	plain := strings.TrimRight(strings.Join(body, "\n"), "\n")
	if strings.TrimSpace(plain) == "" {
		// Nothing written there. Encrypting nothing produces a block that says
		// there is a secret where there is not one.
		return false, nil
	}
	// Encrypted exactly as it sits in the file, indent and all, which is what
	// org-crypt does - it encrypts the region.
	//
	// The temptation is to take the indent off first, as a source block does
	// before it runs. It is wrong here for a reason worth writing down: the
	// region is a whole subtree, so it contains child heading lines at column
	// zero, and the common indent of a body containing one is zero - meaning a
	// heading with children would not be dedented while one without would.
	// Decrypting would then re-indent one and not the other, and re-indenting a
	// child heading stops it being a heading. Verbatim in, verbatim out, and
	// the round trip is exact for both.
	armor, err := encryptBody(plain, cryptKeyFor(sec), pass)
	if err != nil {
		return false, err
	}
	out := append([]string{}, lines[:start]...)
	// Column zero, and a blank line after, which is what org-crypt writes and
	// what gpg will read back.
	out = append(out, strings.Split(armor, "\n")...)
	out = append(out, "")
	if end+1 < len(lines) {
		out = append(out, lines[end+1:]...)
	}
	if err := writeFileAtomic(filename, out); err != nil {
		return false, err
	}
	GetDb().ReloadFile(filename)
	return true, nil
}

// Read one heading's secret back. Writes nothing.
func readHeading(sec *org.Section, f *common.OrgFile, pass string) (string, error) {
	if f == nil || f.Doc == nil {
		return "", fmt.Errorf("that heading has no file")
	}
	lines, from, to, ok := recordLines(f.Doc.Path, sec)
	if !ok {
		return "", fmt.Errorf("could not read %s", f.Doc.Path)
	}
	start, end := cryptRange(lines, from, to)
	if start > end || start >= len(lines) {
		return "", fmt.Errorf("that heading has nothing written under it")
	}
	if end >= len(lines) {
		end = len(lines) - 1
	}
	body := lines[start : end+1]
	if !isArmored(body) {
		return "", fmt.Errorf("that heading is not encrypted")
	}
	return decryptBody(armorOf(body), pass)
}

// Put a heading's body back in the clear, on disk. The explicit, asked-for
// version of what Emacs does when you open an encrypted entry to edit it.
//
// Called with cryptLock held.
func decryptHeadingInPlace(sec *org.Section, f *common.OrgFile, text string) error {
	filename := f.Doc.Path
	lines, from, to, ok := recordLines(filename, sec)
	if !ok {
		return fmt.Errorf("could not read %s", filename)
	}
	start, end := cryptRange(lines, from, to)
	if end >= len(lines) {
		end = len(lines) - 1
	}
	out := append([]string{}, lines[:start]...)
	// Written back exactly as it was encrypted - see encryptHeading. Re-indenting
	// here is what would turn a child heading into a line of text.
	out = append(out, bodyLines(text)...)
	out = append(out, "")
	if end+1 < len(lines) {
		out = append(out, lines[end+1:]...)
	}
	if err := writeFileAtomic(filename, out); err != nil {
		return err
	}
	GetDb().ReloadFile(filename)
	return nil
}

// Every heading carrying the tag.
func cryptHeadings() []common.CryptHeading {
	out := []common.CryptHeading{}
	db := GetDb()
	for _, fname := range db.GetFiles() {
		f := db.FindByFile(fname)
		if f == nil || f.Doc == nil {
			continue
		}
		var lines []string
		for _, sec := range flattenSections(f) {
			if sec == nil || sec.Headline == nil || !isCrypt(sec) {
				continue
			}
			db.RegisterSection(sec.Hash, sec, f)
			if lines == nil {
				b, err := os.ReadFile(fname)
				if err != nil {
					break
				}
				lines = strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
			}
			from := sec.Headline.Pos.Row
			if from < 0 || from >= len(lines) {
				continue
			}
			to := subtreeEndRow(lines, from, sec.Headline.Lvl, from)
			if to >= len(lines) {
				to = len(lines) - 1
			}
			start, end := cryptRange(lines, from, to)
			body := []string{}
			if start <= end && start < len(lines) {
				body = lines[start : end+1]
			}
			out = append(out, common.CryptHeading{
				Hash:      sec.Hash,
				Headline:  sectionTitle(sec),
				Filename:  fname,
				Olp:       outlinePath(sec),
				Line:      from,
				Encrypted: isArmored(body),
				Key:       strings.TrimSpace(GetProp(sec, "CRYPTKEY", "CryptKey", "cryptkey")),
				Lines:     end - start + 1,
			})
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// The endpoints
// ---------------------------------------------------------------------------

func cryptJson(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// The heading a request names, and the file it is in.
func cryptTarget(hash string) (*org.Section, *common.OrgFile, error) {
	sec := GetDb().FindByHash(hash)
	if sec == nil || sec.Headline == nil {
		return nil, nil, fmt.Errorf("no heading with that hash")
	}
	f := GetDb().FileFromSection(sec)
	if f == nil || f.Doc == nil {
		return nil, nil, fmt.Errorf("that heading has no file")
	}
	return sec, f, nil
}

type cryptRequest struct {
	Hash string `json:"hash"`
	// Sent with the request that needs it and never stored, unless
	// `crypt.unlockSeconds` says otherwise.
	Passphrase string `json:"passphrase"`
	// For a decrypt: write the plaintext back to the file rather than only
	// handing it over. Off unless asked for.
	Write bool `json:"write"`
	// For an update: the new body, in the clear, to be encrypted before
	// anything is written.
	Text string `json:"text"`
}

func readCryptRequest(r *http.Request) (cryptRequest, error) {
	var req cryptRequest
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		return req, err
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return req, err
	}
	return req, nil
}

/* SDOC: API
* GET /crypt/config — Whether This Server Can Encrypt, and How

	Answers with the mode, the gpg it found, and how many headings carry the tag
	against how many of those are ciphertext right now.

	*Method:* =GET=

	*Response:* A =CryptConfig= JSON object.
	| Field       | Description                                                           |
	|-------------+-----------------------------------------------------------------------|
	| =Mode=      | =off= with no gpg, =symmetric= with no key, =key= with one            |
	| =Bin=       | The gpg it will use                                                   |
	| =Version=   | What that gpg says about itself                                       |
	| =Key=       | The key it encrypts to, in key mode                                   |
	| =Tag=       | The tag that marks a heading for encryption                           |
	| =Unlocked=  | Whether a passphrase is being held, and =UnlockIn= for how much longer |
	| =Tagged=    | Headings carrying the tag                                             |
	| =Encrypted= | How many of those are ciphertext                                      |

	=Tagged= minus =Encrypted= is the number worth putting in front of somebody:
	headings that are meant to be secret and are sitting in the clear.
EDOC */
func RequestCryptConfig(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	res := common.CryptConfig{Tag: cryptTag()}
	bin := gpgBin()
	res.Bin = bin
	if bin == "" {
		res.Ok = true
		res.Mode = "off"
		res.Msg = "no gpg on this machine"
	} else {
		res.Ok = true
		if out, err := runGpg([]string{"--version"}, nil, ""); err == nil {
			res.Version = firstLines(strings.TrimSpace(string(out)), 1)
		}
		if k := strings.TrimSpace(cryptSettings().Key); k != "" {
			res.Mode = "key"
			res.Key = k
		} else {
			res.Mode = "symmetric"
		}
	}
	if _, left := unlocked(); left > 0 {
		res.Unlocked = true
		res.UnlockIn = left
	}
	for _, h := range cryptHeadings() {
		res.Tagged++
		if h.Encrypted {
			res.Encrypted++
		}
	}
	cryptJson(w, res)
}

/* SDOC: API
* GET /crypt/headings — What Is Marked Secret

	Every heading carrying the crypt tag, and whether each is ciphertext right
	now.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter | Type   | Required | Description                                  |
	|-----------+--------+----------+----------------------------------------------|
	| =plain=   | string | no       | =t= for only the ones sitting in the clear  |

	*Response:* A =CryptHeadings= JSON object holding =Headings=, =Tagged= and
	=Plain=.

	Never a body. A listing says what is secret, not what the secret is - an
	endpoint that handed over plaintext because it was convenient would undo the
	feature for every client that ever called it.
EDOC */
func RequestCryptHeadings(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	res := common.CryptHeadings{Ok: true, Headings: []common.CryptHeading{}}
	onlyPlain := r.URL.Query().Get("plain") == "t"
	for _, h := range cryptHeadings() {
		res.Tagged++
		if !h.Encrypted {
			res.Plain++
		}
		if onlyPlain && h.Encrypted {
			continue
		}
		res.Headings = append(res.Headings, h)
	}
	cryptJson(w, res)
}

/* SDOC: API
* POST /crypt/encrypt — Encrypt One Heading

	Replaces the heading's body - everything after its planning line and its
	drawers, to the end of its subtree - with an ASCII armoured gpg message, in
	the file.

	*Method:* =POST=

	*Request Body:*
	| Field         | Type   | Required | Description                                       |
	|---------------+--------+----------+---------------------------------------------------|
	| =hash=        | string | yes      | The heading                                       |
	| =passphrase=  | string | in symmetric mode | Never stored. In key mode none is needed |

	*Response:* A =CryptResult=. =Text= is always empty here: there is no reason
	for an encrypt to echo what it has just hidden.

	The heading keeps its own line, its planning line and its property drawer in
	the clear, exactly as org-crypt leaves them - a heading's =:ID:= is how links
	and attachments reach it, and encrypting that would break the database to
	hide a uuid.

	Nothing is written until the ciphertext has been decrypted back and compared
	with what it was made from. Encryption that half worked is not a failed save,
	it is a destroyed note.
EDOC */
func PostCryptEncrypt(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	req, err := readCryptRequest(r)
	if err != nil {
		cryptJson(w, common.CryptResult{Msg: err.Error()})
		return
	}

	cryptLock.Lock()
	defer cryptLock.Unlock()

	sec, f, err := cryptTarget(req.Hash)
	if err != nil {
		cryptJson(w, common.CryptResult{Msg: err.Error()})
		return
	}
	res := common.CryptResult{Hash: req.Hash, Filename: f.Doc.Path}
	changed, err := encryptHeading(sec, f, passFor(req.Passphrase))
	if err != nil {
		res.Msg = err.Error()
		cryptJson(w, res)
		return
	}
	res.Ok = true
	res.Encrypted = true
	if changed {
		res.Msg = "encrypted"
	} else {
		res.Msg = "already encrypted"
	}
	cryptJson(w, res)
}

/* SDOC: API
* POST /crypt/decrypt — Read One Heading Back

	Hands back the plaintext of an encrypted heading. Writes nothing unless
	asked to.

	*Method:* =POST=

	*Request Body:*
	| Field        | Type   | Required | Description                                          |
	|--------------+--------+----------+------------------------------------------------------|
	| =hash=       | string | yes      | The heading                                          |
	| =passphrase= | string | yes      | Unless the server is holding one                     |
	| =write=      | bool   | no       | Also put the body back in the clear, in the file     |

	*Response:* A =CryptResult= carrying =Text=.

	=write= is the one operation here that puts plaintext on a disk, so it is
	asked for rather than assumed - and it is what Emacs does when you open an
	encrypted entry to edit it. To change an encrypted heading without its body
	ever being written in the clear, use =/crypt/update=.
EDOC */
func PostCryptDecrypt(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	req, err := readCryptRequest(r)
	if err != nil {
		cryptJson(w, common.CryptResult{Msg: err.Error()})
		return
	}

	cryptLock.Lock()
	defer cryptLock.Unlock()

	sec, f, err := cryptTarget(req.Hash)
	if err != nil {
		cryptJson(w, common.CryptResult{Msg: err.Error()})
		return
	}
	res := common.CryptResult{Hash: req.Hash, Filename: f.Doc.Path, Encrypted: true}
	text, err := readHeading(sec, f, passFor(req.Passphrase))
	if err != nil {
		res.Msg = err.Error()
		cryptJson(w, res)
		return
	}
	res.Ok = true
	res.Text = text
	res.Msg = "decrypted"
	if req.Write {
		if err := decryptHeadingInPlace(sec, f, text); err != nil {
			res.Msg = "read back, but the file could not be written: " + err.Error()
			cryptJson(w, res)
			return
		}
		res.Encrypted = false
		res.Msg = "decrypted, and written back in the clear"
	}
	cryptJson(w, res)
}

/* SDOC: API
* POST /crypt/update — Change an Encrypted Heading

	Replaces the heading's body with new text, encrypted. The plaintext is never
	written to disk: it arrives, is encrypted, and the ciphertext is what lands.

	*Method:* =POST=

	*Request Body:*
	| Field        | Type   | Required | Description                        |
	|--------------+--------+----------+------------------------------------|
	| =hash=       | string | yes      | The heading                        |
	| =text=       | string | yes      | The new body, in the clear         |
	| =passphrase= | string | in symmetric mode | Never stored               |

	*Response:* A =CryptResult=.

	This is how an encrypted heading is edited. The alternative - decrypt to the
	file, edit, encrypt again - puts the note in the clear on a disk for as long
	as the editing takes, which on a server is as long as somebody leaves a tab
	open.
EDOC */
func PostCryptUpdate(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	req, err := readCryptRequest(r)
	if err != nil {
		cryptJson(w, common.CryptResult{Msg: err.Error()})
		return
	}

	cryptLock.Lock()
	defer cryptLock.Unlock()

	sec, f, err := cryptTarget(req.Hash)
	if err != nil {
		cryptJson(w, common.CryptResult{Msg: err.Error()})
		return
	}
	res := common.CryptResult{Hash: req.Hash, Filename: f.Doc.Path}
	pass := passFor(req.Passphrase)
	// Verbatim, like every other body here: a client that decrypted, edited and
	// sent it back gets an exact round trip, and one composing fresh text is
	// responsible for its own indentation. Guessing would double the indent on
	// the first of those, which is the common case.
	armor, err := encryptBody(strings.TrimRight(req.Text, "\n"), cryptKeyFor(sec), pass)
	if err != nil {
		res.Msg = err.Error()
		cryptJson(w, res)
		return
	}

	filename := f.Doc.Path
	lines, from, to, ok := recordLines(filename, sec)
	if !ok {
		res.Msg = "could not read " + filename
		cryptJson(w, res)
		return
	}
	start, end := cryptRange(lines, from, to)
	if end >= len(lines) {
		end = len(lines) - 1
	}
	out := append([]string{}, lines[:start]...)
	out = append(out, strings.Split(armor, "\n")...)
	out = append(out, "")
	if end+1 < len(lines) {
		out = append(out, lines[end+1:]...)
	}
	if err := writeFileAtomic(filename, out); err != nil {
		res.Msg = err.Error()
		cryptJson(w, res)
		return
	}
	GetDb().ReloadFile(filename)
	res.Ok = true
	res.Encrypted = true
	res.Msg = "written"
	cryptJson(w, res)
}

/* SDOC: API
* POST /crypt/sweep — Encrypt Everything That Should Be

	Encrypts every heading carrying the crypt tag that is currently sitting in
	the clear. This is org-crypt's =org-encrypt-entries=, and it is what makes
	the feature safe to live with: tag a heading, sweep, and what is on disk is
	ciphertext.

	*Method:* =POST=

	*Request Body:*
	| Field        | Type   | Required | Description                |
	|--------------+--------+----------+----------------------------|
	| =passphrase= | string | in symmetric mode | Never stored      |

	*Response:* A =CryptSweepResult= saying how many were encrypted, how many
	were already, how many could not be - with a line each saying why - and which
	files were written.

	A heading that fails stops nothing: the rest are still done, and the ones
	that were not are named. Stopping at the first failure would leave a sweep
	half finished with no way to tell which half.
EDOC */
func PostCryptSweep(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	req, err := readCryptRequest(r)
	if err != nil {
		cryptJson(w, common.CryptSweepResult{Msg: err.Error()})
		return
	}

	cryptLock.Lock()
	defer cryptLock.Unlock()

	res := common.CryptSweepResult{Ok: true, Problems: []string{}, Files: []string{}}
	pass := passFor(req.Passphrase)
	written := map[string]bool{}

	// Re-listed after every heading that is encrypted, rather than walked once.
	//
	// Encrypting a subtree *removes* the headings inside it from the parse -
	// they are inside the armoured block now - and a heading's hash is
	// accumulated from the chain of headlines walked before it, so every
	// heading below the one just encrypted has a different hash than it did a
	// moment ago. A list taken up front gets the first one right and then
	// addresses headings that no longer exist, which is exactly the trap
	// `/move` documents for a batch of refiles.
	//
	// Relisting is O(headings) per encrypted heading and a sweep is rare, so
	// the simple correct thing is the right thing here. The loop ends when a
	// pass encrypts nothing, which it must: every pass either encrypts one or
	// records a failure for it, and neither can happen twice for the same
	// heading.
	tried := map[string]bool{}
	for {
		did := false
		for _, h := range cryptHeadings() {
			if h.Encrypted {
				continue
			}
			// Keyed by where it is rather than by its hash, which is the thing
			// that keeps moving.
			key := h.Filename + "\x00" + strings.Join(h.Olp, "\x00")
			if tried[key] {
				continue
			}
			tried[key] = true
			sec, f, err := cryptTarget(h.Hash)
			if err != nil {
				res.Failed++
				res.Problems = append(res.Problems, h.Headline+": "+err.Error())
				did = true
				break
			}
			changed, err := encryptHeading(sec, f, pass)
			if err != nil {
				res.Failed++
				res.Problems = append(res.Problems, h.Headline+": "+err.Error())
			} else if changed {
				res.Encrypted++
				written[f.Doc.Path] = true
			} else {
				res.Already++
			}
			did = true
			// Back to the top: the file has just been rewritten and every hash
			// below this heading has moved.
			break
		}
		if !did {
			break
		}
	}
	// Anything tagged that was already ciphertext when the sweep started.
	for _, h := range cryptHeadings() {
		if h.Encrypted {
			key := h.Filename + "\x00" + strings.Join(h.Olp, "\x00")
			if !tried[key] {
				res.Already++
			}
		}
	}

	for p := range written {
		res.Files = append(res.Files, p)
	}
	if res.Failed > 0 {
		res.Ok = false
		res.Msg = fmt.Sprintf("%d encrypted, %d already, %d could not be", res.Encrypted, res.Already, res.Failed)
	} else {
		res.Msg = fmt.Sprintf("%d encrypted, %d already", res.Encrypted, res.Already)
	}
	cryptJson(w, res)
}

/* SDOC: API
* POST /crypt/lock — Forget the Held Passphrase

	Drops whatever passphrase is being held in memory, if
	=crypt.unlockSeconds= let one be held at all.

	*Method:* =POST=

	*Response:* A =Result=.

	There is nothing to do here when nothing is held, which is the default, and
	saying so is not a failure.
EDOC */
func PostCryptLock(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	relock()
	cryptJson(w, common.ResultMsg{Ok: true, Msg: "locked"})
}
