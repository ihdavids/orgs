package lint

// The two checkers, both on the server: Harper (harper-ls, /harper/lint) and
// LanguageTool (passed through by /languagetool/check). Each answers with
// offsets into the text it was given, in UTF-16 units, which para.place turns
// back into a line and a column of the file.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/ihdavids/orgs/cmd/oc/commands"
)

// found is one problem before it has a file to belong to.
type found struct {
	at       pos
	severity string
	rule     string
	msg      string
	fix      []string
}

type harperStatus struct {
	Ok      bool
	Enabled bool
	State   string
	Msg     string
}

type ltStatus struct {
	Ok      bool
	Running bool
	Url     string
	State   string
	Msg     string
}

type harperLint struct {
	Rule  string   `json:"rule"`
	Kind  string   `json:"kind"`
	Start int      `json:"start"`
	End   int      `json:"end"`
	Msg   string   `json:"msg"`
	Fixes []string `json:"fixes"`
}

type harperReq struct {
	Texts   []string
	Dialect string
	Off     []string
}

type harperResp struct {
	Ok    bool
	Msg   string
	Lints [][]harperLint
}

// A kind of problem that is a matter of taste rather than a mistake is info,
// so `-format quickfix` and an editor can show it quieter.
func severityOf(kind string) string {
	switch strings.ToLower(kind) {
	case "style", "readability", "enhancement", "wordchoice", "redundancy", "repetition",
		"typographical", "whitespace", "formatting", "regionalism", "miscellaneous":
		return "info"
	}
	return "warning"
}

// harperBatch is how many paragraphs go in one request; the server refuses
// more than 200.
const harperBatch = 100

func runHarper(core *commands.Core, paras []*para, dialect string, off []string) ([][]found, error) {
	out := make([][]found, len(paras))
	for lo := 0; lo < len(paras); lo += harperBatch {
		hi := lo + harperBatch
		if hi > len(paras) {
			hi = len(paras)
		}
		req := harperReq{Dialect: dialect, Off: off}
		for _, p := range paras[lo:hi] {
			req.Texts = append(req.Texts, string(p.text))
		}
		// Not SendReceivePostErr: this POST writes nothing, and -dry-run
		// would stop a check that has nothing to hold back.
		body, err := json.Marshal(req)
		if err != nil {
			return nil, err
		}
		raw, err := core.Rest.Post("harper/lint", body)
		if err != nil {
			return nil, err
		}
		var res harperResp
		if err := json.Unmarshal(raw, &res); err != nil {
			return nil, fmt.Errorf("harper: %s", strings.TrimSpace(string(raw)))
		}
		if !res.Ok {
			return nil, fmt.Errorf("harper: %s", res.Msg)
		}
		for i, lints := range res.Lints {
			p := paras[lo+i]
			for _, l := range lints {
				out[lo+i] = append(out[lo+i], found{at: p.place(l.Start), severity: severityOf(l.Kind),
					rule: l.Rule, msg: l.Msg, fix: l.Fixes})
			}
		}
	}
	return out, nil
}

type ltResp struct {
	Matches []struct {
		Message      string
		Offset       int
		Length       int
		Replacements []struct{ Value string }
		Rule         struct {
			Id        string
			IssueType string
		}
	}
}

// ltChunk is roughly how much text goes in one LanguageTool request. Each
// paragraph is checked in context - LanguageTool's sentence rules want the
// sentences around it - so paragraphs are joined with a blank line, and the
// answer's offsets are walked back to the paragraph they fall in.
const ltChunk = 20000

func runLanguageTool(core *commands.Core, paras []*para, lang string, off []string) ([][]found, error) {
	out := make([][]found, len(paras))
	for lo := 0; lo < len(paras); {
		hi := lo
		size := 0
		starts := []int{}
		var text strings.Builder
		for hi < len(paras) && (hi == lo || size+paras[hi].u16[len(paras[hi].text)] < ltChunk) {
			if hi > lo {
				text.WriteString("\n\n")
				size += 2
			}
			starts = append(starts, size)
			text.WriteString(string(paras[hi].text))
			size += paras[hi].u16[len(paras[hi].text)]
			hi++
		}
		res, err := ltCheck(core, text.String(), lang, off)
		if err != nil {
			return nil, err
		}
		for _, m := range res.Matches {
			k := 0
			for k+1 < len(starts) && starts[k+1] <= m.Offset {
				k++
			}
			p := paras[lo+k]
			f := found{at: p.place(m.Offset - starts[k]), severity: severityOf(m.Rule.IssueType),
				rule: m.Rule.Id, msg: m.Message}
			if m.Rule.IssueType == "misspelling" || m.Rule.IssueType == "grammar" {
				f.severity = "warning"
			}
			for i, r := range m.Replacements {
				if i == 3 {
					break
				}
				f.fix = append(f.fix, r.Value)
			}
			out[lo+k] = append(out[lo+k], f)
		}
		lo = hi
	}
	return out, nil
}

// ltCheck posts the form LanguageTool's /v2/check takes. The server passes it
// through as it is, so it goes form encoded rather than through RestPost's
// json.
func ltCheck(core *commands.Core, text, lang string, off []string) (ltResp, error) {
	var res ltResp
	form := url.Values{}
	form.Set("text", text)
	form.Set("language", lang)
	if len(off) > 0 {
		form.Set("disabledRules", strings.Join(off, ","))
	}
	req, err := http.NewRequest("POST", core.Rest.Url+"/languagetool/check", strings.NewReader(form.Encode()))
	if err != nil {
		return res, err
	}
	req.Header = core.Rest.Header.Clone()
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return res, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return res, fmt.Errorf("languagetool: %s", strings.TrimSpace(string(raw)))
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return res, fmt.Errorf("languagetool: %s", strings.TrimSpace(string(raw)))
	}
	return res, nil
}
