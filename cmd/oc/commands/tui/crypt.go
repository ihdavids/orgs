package tui

// Encrypted headings in the tui: reveal, hide, encrypt, lock. See the note at
// the top of tui.go for what is held and when it is dropped.

import (
	"regexp"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/ihdavids/orgs/cmd/oc/commands"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/rivo/tview"
)

const armorBegin = "-----BEGIN PGP MESSAGE-----"

var (
	planningRe  = regexp.MustCompile(`^\s*(SCHEDULED|DEADLINE|CLOSED):`)
	drawerOpen  = regexp.MustCompile(`^\s*:([A-Za-z][A-Za-z0-9_\-]*):\s*$`)
	drawerClose = regexp.MustCompile(`^\s*:END:\s*$`)
)

// A heading's body split where crypt.go splits it: the planning line and
// drawers that stay in the clear, and the rest - which is the secret, and is
// ciphertext when its first non-blank line is the armour header.
func splitSecret(body string) (lead string, secret string, encrypted bool) {
	lines := strings.Split(body, "\n")
	i := 0
	for i < len(lines) {
		l := lines[i]
		if strings.TrimSpace(l) == "" || planningRe.MatchString(l) {
			i++
			continue
		}
		if drawerOpen.MatchString(l) {
			j := i + 1
			for j < len(lines) && !drawerClose.MatchString(lines[j]) {
				j++
			}
			if j >= len(lines) {
				break
			}
			i = j + 1
			continue
		}
		break
	}
	lead = strings.TrimRight(strings.Join(lines[:i], "\n"), " \t\n")
	secret = strings.Join(lines[i:], "\n")
	return lead, secret, strings.HasPrefix(strings.TrimSpace(secret), armorBegin)
}

func (self *Tui) isCrypt(t common.Todo) bool {
	tag := self.crypt.Tag
	if tag == "" {
		return false
	}
	for _, x := range t.Tags {
		if strings.EqualFold(x, tag) {
			return true
		}
	}
	return false
}

// How long a revealed body stays up untouched: the server's unlock window if
// it has one, else five minutes - the same rule as worg's.
func (self *Tui) idle() time.Duration {
	secs := self.crypt.UnlockSeconds
	if secs <= 0 {
		secs = 300
	}
	if secs < 60 {
		secs = 60
	}
	return time.Duration(secs) * time.Second
}

func (self *Tui) hide() {
	if self.secret == nil {
		return
	}
	if self.secret.timer != nil {
		self.secret.timer.Stop()
	}
	self.secret = nil
}

func (self *Tui) say(msg string) {
	self.err = msg
	self.drawHead()
}

// Straight to the server rather than through SendReceivePost, which prints the
// body on -dry-run - and this body carries a passphrase.
func (self *Tui) cryptPost(path string, req common.CryptOp) (common.CryptOpResult, error) {
	return common.RestPost[common.CryptOpResult](&self.core.Rest, path, &req)
}

func (self *Tui) toggleReveal() {
	t, ok := self.current()
	if !ok {
		return
	}
	if self.secret != nil && self.secret.hash == t.Hash {
		self.hide()
		self.refreshPreview()
		return
	}
	if !self.showing {
		self.togglePreview()
	}
	if self.crypt.Mode == "off" || self.crypt.Mode == "" {
		self.say("this server cannot decrypt (no gpg, or no crypt support)")
		return
	}
	req := common.CryptOp{Hash: t.Hash}
	// Held passphrase or a key: no question to ask.
	if st, err := commands.SendReceiveGetErr[common.CryptState](self.core, "crypt/state", nil); err == nil && st.Ok {
		self.crypt = st
	}
	if self.crypt.Unlocked || self.crypt.Mode == "key" {
		res, err := self.cryptPost("crypt/reveal", req)
		if err == nil && res.Ok {
			self.show(t, res.Text)
			return
		}
		if err == nil && !res.NeedPassphrase && !res.WrongPassphrase {
			self.say(res.Msg)
			return
		}
	}
	self.askSecret(" passphrase: "+t.Headline+" ", false, func(pass string, keep bool) {
		req.Passphrase = pass
		req.Keep = keep
		res, err := self.cryptPost("crypt/reveal", req)
		switch {
		case err != nil:
			self.say(err.Error())
		case !res.Ok:
			self.say(res.Msg)
		default:
			self.say("")
			self.show(t, res.Text)
		}
	})
}

func (self *Tui) show(t common.Todo, text string) {
	self.hide()
	r := &revealed{hash: t.Hash, text: text}
	r.timer = time.AfterFunc(self.idle(), func() {
		self.app.QueueUpdateDraw(func() {
			if self.secret == r {
				self.hide()
				self.refreshPreview()
			}
		})
	})
	self.secret = r
	self.refreshPreview()
}

func (self *Tui) encrypt() {
	t, ok := self.current()
	if !ok {
		return
	}
	if self.crypt.Mode == "off" || self.crypt.Mode == "" {
		self.say("this server cannot encrypt (no gpg, or no crypt support)")
		return
	}
	seal := func(pass string, keep bool) {
		res, err := self.cryptPost("crypt/seal", common.CryptOp{Hash: t.Hash, Passphrase: pass, Keep: keep})
		switch {
		case err != nil:
			self.say(err.Error())
		case !res.Ok:
			self.say(res.Msg)
		default:
			self.hide()
			self.reload()
			// After the reload, which clears the head line on success.
			self.say(t.Headline + ": " + res.Msg)
		}
	}
	if commands.DryRun {
		self.say("dry run: " + t.Headline + " would be encrypted")
		return
	}
	if self.crypt.Mode == "key" {
		seal("", false)
		return
	}
	self.askSecret(" new passphrase: "+t.Headline+" ", true, seal)
}

func (self *Tui) lock() {
	self.hide()
	if _, err := common.RestPost[common.ResultMsg](&self.core.Rest, "crypt/lock", &common.Empty{}); err != nil {
		self.say(err.Error())
	} else {
		self.crypt.Unlocked = false
		self.say("locked")
	}
	self.refreshPreview()
}

// A masked box (twice when setting a passphrase), with "keep unlocked" only
// when the server is willing to hold one. Esc gives up.
func (self *Tui) askSecret(title string, twice bool, done func(pass string, keep bool)) {
	form := tview.NewForm()
	var pass, again string
	keep := false
	form.AddPasswordField("Passphrase", "", 32, '•', func(v string) { pass = v })
	if twice {
		form.AddPasswordField("Again", "", 32, '•', func(v string) { again = v })
	}
	if self.crypt.UnlockSeconds > 0 {
		form.AddCheckbox("Keep unlocked", false, func(v bool) { keep = v })
	}
	close := func() {
		self.pages.RemovePage("secret")
		self.app.SetFocus(self.table)
	}
	submit := func() {
		if pass == "" {
			self.say("an empty passphrase is not one")
			return
		}
		if twice && pass != again {
			self.say("those two do not match")
			return
		}
		p, k := pass, keep
		pass, again = "", ""
		close()
		done(p, k)
	}
	form.AddButton("OK", submit)
	form.AddButton("Cancel", close)
	form.SetCancelFunc(close)
	form.SetBorder(true).SetTitle(title)
	// Enter in a field submits, rather than only moving to the next field.
	form.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		if ev.Key() == tcell.KeyEnter {
			if idx, _ := form.GetFocusedItemIndex(); idx >= 0 {
				if _, isBox := form.GetFormItem(idx).(*tview.InputField); isBox {
					if !twice || idx > 0 || again != "" {
						submit()
						return nil
					}
				}
			}
		}
		return ev
	})
	height := 7
	if twice {
		height += 2
	}
	if self.crypt.UnlockSeconds > 0 {
		height += 2
	}
	self.pages.AddPage("secret", centred(form, 52, height), true, true)
	self.app.SetFocus(form)
}

func encrypted(body string) bool {
	_, _, enc := splitSecret(body)
	return enc
}
