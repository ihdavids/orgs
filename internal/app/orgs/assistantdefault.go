package orgs

// POST /assistant/default - which backend and model the writing assistant
// uses when a request names none, written back into orgs.yaml.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ihdavids/orgs/internal/common"
)

type assistantDefaultReq struct {
	Backend string
	Model   string
}

/* SDOC: API
* POST /assistant/default — Choose the Writing Assistant's Default
	Body ={Backend, Model}=. Makes =Backend= the one used when a request names
	none, and =Model= (optional) that backend's default model. The model must
	be one =GET /assistant/status= lists for it, as for a chat. Writes
	=assistant.backend= and =assistant.<backend>.model= into the server's own
	yaml - those lines only, after keeping the file as it was in
	=<file>.before-assistant= - and takes them up at once, without a restart.
	Administrators only (anyone with =noAuth=).

	For claude, chatgpt and gemini the writer picks from =model= and =models=:
	a default that is in neither is no longer offered once replaced, and =Msg=
	says so.

	*Response:* ={Ok, Msg}=
EDOC */
func PostAssistantDefault(w http.ResponseWriter, r *http.Request) {
	var req assistantDefaultReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writingJson(w, common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	if err := mayChangeKeys(r); err != nil {
		writingJson(w, common.ResultMsg{Ok: false, Msg: strings.Replace(err.Error(), "its AI keys", "its writing assistant", 1)})
		return
	}
	name := strings.ToLower(strings.TrimSpace(req.Backend))
	model := strings.TrimSpace(req.Model)
	b := assistantBackend(name)
	if b == nil {
		writingJson(w, common.ResultMsg{Ok: false, Msg: "no backend " + req.Backend + ": claude, chatgpt, gemini, claudecode, ollama or local"})
		return
	}
	if !assistantOffered(name, b) {
		writingJson(w, common.ResultMsg{Ok: false, Msg: name + " is not turned on (assistant." + name + " in orgs.yaml)"})
		return
	}
	old := assistantModel(name, b)
	if model != "" {
		if model != old && !slices.Contains(assistantModels(name, b), model) {
			writingJson(w, common.ResultMsg{Ok: false, Msg: "model " + model + " is not one the server offers for " + name + " (orgs ask status lists them)"})
			return
		}
	}

	file, _, err := editConfigFile(".before-assistant", func(lines []string) ([]string, error) {
		out, err := setYamlPath(lines, []string{"server", "assistant", "backend"}, name)
		if err != nil || model == "" {
			return out, err
		}
		return setYamlPath(out, []string{"server", "assistant", name, "model"}, model)
	})
	if err != nil {
		writingJson(w, common.ResultMsg{Ok: false, Msg: fmt.Sprintf("%s: %s", filepath.Base(file), err)})
		return
	}
	// ReloadConfig does not take up the assistant's settings (keys and local
	// runtimes hang off them), so set the two that were written.
	a := &Conf().Server.Assistant
	a.Backend = name
	if model != "" {
		b.Model = model
	}

	msg := name
	if m := assistantModel(name, b); m != "" {
		msg += " " + m
	}
	msg += ", saved in " + filepath.Base(file)
	if lost := droppedModel(name, b, old); lost != "" {
		msg += fmt.Sprintf("; %s is no longer offered (add it to assistant.%s.models to keep it)", lost, name)
	}
	fmt.Fprintf(os.Stderr, "ASSISTANT: default set to %s by %q\n", msg, GetUsername(r))
	writingJson(w, common.ResultMsg{Ok: true, Msg: msg})
}

// The model a change of default took out of the list the writer picks from:
// only the cloud backends' list is the yaml's (ollama and local list what is
// installed, claudecode the rest of the family).
func droppedModel(name string, b *common.AssistantBackend, old string) string {
	if old == "" || old == assistantModel(name, b) || slices.Contains(assistantModels(name, b), old) {
		return ""
	}
	return old
}
