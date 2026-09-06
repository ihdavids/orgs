//lint:file-ignore ST1006 allow the use of self
package orgs

// ----------------------------------------------------------------------------
// Coin
//
// The coin tab on the html character sheet spends and earns money the same way
// the inventory tab picks things up: the change is applied to the character's
// own org file and a line is added to its Coin History section. There is no
// server side purse - the file is the purse - so the sheet, the org file and
// anything else reading it never disagree about how much gold is left.
// ----------------------------------------------------------------------------

import (
	"net/http"
	"strings"

	"github.com/ihdavids/orgs/internal/common/dnd"
)

// dndMoneyState is what every coin call answers with.
func dndMoneyState(c *dnd.Character, rs *dnd.Ruleset, path, msg string) dnd.MoneyState {
	history := c.MoneyLog
	if history == nil {
		history = []dnd.MoneyEvent{}
	}
	id := ""
	if rs != nil {
		id = rs.Id
	}
	return dnd.MoneyState{
		Id: dnd.CharacterId(c), Name: c.Name, Filename: path, Ruleset: id,
		Purse: dnd.ComputeMoney(c.Money), History: history, Msg: msg,
	}
}

/* SDOC: API
* GET /dnd/money — What A Character Is Carrying In Coin
	Returns the purse broken down by denomination, what the lot is worth, what it
	weighs, and the sheet's Coin History.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter  | Type   | Required | Description                                      |
	|------------+--------+----------+--------------------------------------------------|
	| =filename= | string | no       | The org character sheet (basename or path).      |
	| =id=       | string | no       | The character's =DND_ID=, used when no filename. |

	One of the two is required.

	*Response:* A =MoneyState=:
	#+BEGIN_SRC json
	{
	  "id": "lyra-silverleaf-4c1f2a", "name": "Lyra", "filename": "/gtd/lyra.org",
	  "purse": {
	    "coins": [
	      {"id": "pp", "name": "Platinum", "abbr": "pp", "qty": 1, "value": 1000, "gold": 10},
	      {"id": "gp", "name": "Gold", "abbr": "gp", "qty": 42, "value": 100, "gold": 42}
	    ],
	    "money": {"cp": 5, "sp": 0, "ep": 0, "gp": 42, "pp": 1},
	    "copper": 5205, "gold": 52.05, "total": "52 gp 5 cp", "count": 48, "weight": 0.96
	  },
	  "history": [{"date": "2025-09-05", "time": "19:32", "action": "spent",
	               "amount": {"gp": 50}, "balance": {"gp": 42, "pp": 1, "cp": 5},
	               "notes": "potion of healing"}]
	}
	#+END_SRC

	A coin is worth what it is worth in D&D: one gold piece is 100 cp, 10 sp, 2 ep
	or a tenth of a platinum, and fifty coins of any kind weigh a pound.
	EDOC */
func RequestDndMoney(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	filename := r.URL.Query().Get("filename")
	id := r.URL.Query().Get("id")
	if strings.TrimSpace(filename) == "" && strings.TrimSpace(id) == "" {
		dndError(w, http.StatusBadRequest, "pass a filename or a character id")
		return
	}
	dndInvLock.Lock()
	defer dndInvLock.Unlock()
	c, rs, path, err := dndFindCharacter(filename, id)
	if err != nil {
		dndError(w, http.StatusNotFound, "%s", err)
		return
	}
	dndJson(w, dndMoneyState(c, rs, path, ""))
}

/* SDOC: API
* POST /dnd/money — Spend, Earn Or Change Coin
	Applies one change to a character's purse and writes the sheet back, adding a line
	to its Coin History section.

	*Method:* =POST=

	*Body:* =MoneyRequest=
	| Field      | Type   | Required | Description                                                     |
	|------------+--------+----------+-----------------------------------------------------------------|
	| =filename= | string | no       | The org character sheet. One of filename or id is required.     |
	| =id=       | string | no       | The character's =DND_ID=.                                       |
	| =action=   | string | yes      | =spend=, =gain=, =set=, =consolidate= or =exchange=.            |
	| =amount=   | string | no       | The amount as text, =15 gp 3 sp=. A bare number means gold.     |
	| =money=    | object | no       | The same amount in fields, ={"gp": 15, "sp": 3}=. Wins if both. |
	| =from=     | string | no       | =exchange=: the coin being changed, =sp=.                       |
	| =to=       | string | no       | =exchange=: the coin wanted back, =gp=.                         |
	| =qty=      | number | no       | =exchange=: how many of =from= to change.                       |
	| =electrum= | bool   | no       | =consolidate=: also change electrum up, off by default.         |
	| =notes=    | string | no       | What it was for, kept in the history.                           |

	#+BEGIN_SRC json
	{"id": "lyra-silverleaf-4c1f2a", "action": "spend", "amount": "50 gp",
	 "notes": "potion of healing"}
	#+END_SRC

	*Response:* The =MoneyState= after the change.

	Spending makes change: paying two copper out of a purse holding one gold piece
	hands over the gold and takes back 9 sp 8 cp, so a cost is refused with a =400=
	only when the purse is genuinely worth less than the price. =consolidate= changes
	small coin up into the largest denominations that hold the same value, and
	=exchange= trades one denomination for another at the standard rate, refusing a
	swap that will not come out even.
	EDOC */
func PostDndMoney(w http.ResponseWriter, r *http.Request) {
	AccessControl(&w)
	var req dnd.MoneyRequest
	if !dndBody(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Filename) == "" && strings.TrimSpace(req.Id) == "" {
		dndError(w, http.StatusBadRequest, "pass a filename or a character id")
		return
	}
	dndInvLock.Lock()
	defer dndInvLock.Unlock()
	c, rs, path, err := dndFindCharacter(req.Filename, req.Id)
	if err != nil {
		dndError(w, http.StatusNotFound, "%s", err)
		return
	}
	event, err := dnd.ApplyMoney(c, req)
	if err != nil {
		dndError(w, http.StatusBadRequest, "%s", err)
		return
	}
	if err := dndWriteCharacter(c, rs, path); err != nil {
		dndError(w, http.StatusInternalServerError, "could not write %s: %s", path, err)
		return
	}
	dndJson(w, dndMoneyState(c, rs, path, dnd.MoneyEventMsg(event)))
}
