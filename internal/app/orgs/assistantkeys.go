package orgs

// Keys for the writing assistant set from worg (WritingAssistant.tsx's key
// helper) rather than written into orgs.yaml: each is tried against its
// service first, then kept in the OS keyring under service "orgs" as
// assistant-<backend>, or, where there is no keyring (a headless Linux box),
// in <config>_assistant_keys.yaml beside the config, readable only by its
// owner (always the file when ORGS_NO_KEYRING is set: a test server must not
// write the real keyring). A key set here is used ahead of orgs.yaml's and the environment's,
// so it takes effect at once, without a restart.
//
// Only an administrator may set or remove one (any user with noAuth on, as
// there is only one): the key is the server's, and every user's questions
// are billed to it. Keys are never sent back; the status says only where a
// backend's key comes from.

import (
	"context"
	"errors"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/ihdavids/orgs/internal/common"
	"gopkg.in/yaml.v3"
)

var (
	storedKeysMu sync.Mutex
	storedKeys   = map[string]string{} // backend -> key, as read or set; "" read and none
)

func assistantKeysPath() string {
	cfg := Conf().Config
	ext := filepath.Ext(cfg)
	return strings.TrimSuffix(cfg, ext) + "_assistant_keys" + ext
}

func readKeysFile() map[string]string {
	out := map[string]string{}
	raw, err := os.ReadFile(assistantKeysPath())
	if err == nil {
		yaml.Unmarshal(raw, &out)
	}
	return out
}

func writeKeysFile(m map[string]string) error {
	raw, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	p := assistantKeysPath()
	if err := os.WriteFile(p, raw, 0600); err != nil {
		return err
	}
	return os.Chmod(p, 0600)
}

// The key set from worg for a backend, or "".
func assistantStoredKey(name string) string {
	storedKeysMu.Lock()
	defer storedKeysMu.Unlock()
	if k, ok := storedKeys[name]; ok {
		return k
	}
	k := ""
	if !noKeyring() {
		k, _ = common.KeyringGet("assistant-" + name)
	}
	if k == "" {
		k = readKeysFile()[name]
	}
	storedKeys[name] = k
	return k
}

func setStoredKey(name, key string) (string, error) {
	storedKeysMu.Lock()
	defer storedKeysMu.Unlock()
	where := "the keyring"
	// The keyring has no delete here: an empty secret is no key.
	var err error = fmt.Errorf("no keyring")
	if !noKeyring() {
		err = common.KeyringSet("assistant-"+name, key)
	}
	if err != nil {
		m := readKeysFile()
		if key == "" {
			delete(m, name)
		} else {
			m[name] = key
		}
		if err := writeKeysFile(m); err != nil {
			return "", err
		}
		where = assistantKeysPath()
	} else if m := readKeysFile(); m[name] != "" {
		// A key left in the file from before would come back otherwise.
		delete(m, name)
		writeKeysFile(m)
	}
	storedKeys[name] = key
	return where, nil
}

func noKeyring() bool { return os.Getenv("ORGS_NO_KEYRING") != "" }

func mayChangeKeys(r *http.Request) error {
	if Conf().Server != nil && Conf().Server.NoAuth {
		return nil
	}
	who := GetUsername(r)
	if who == "" || !GetKeystore().IsAdmin(who) {
		return fmt.Errorf("only an administrator of this server can change its AI keys (orgs user admin %s on the server's machine)", who)
	}
	return nil
}

// Does this key work? One cheap request: the service's list of models.
func tryAssistantKey(ctx context.Context, name string, b *common.AssistantBackend, key string) error {
	switch name {
	case "claude":
		opts := []option.RequestOption{option.WithAPIKey(key), option.WithMaxRetries(0)}
		if b.Url != "" {
			opts = append(opts, option.WithBaseURL(b.Url))
		}
		client := anthropic.NewClient(opts...)
		_, err := client.Models.List(ctx, anthropic.ModelListParams{Limit: anthropic.Int(1)})
		var apierr *anthropic.Error
		if errors.As(err, &apierr) {
			switch apierr.StatusCode {
			case 401:
				return fmt.Errorf("Anthropic does not know this key (401)")
			case 403:
				return fmt.Errorf("this key may not be used for that (403)")
			}
			return fmt.Errorf("Anthropic answered %d", apierr.StatusCode)
		}
		return err
	case "chatgpt":
		base := strings.TrimRight(b.Url, "/")
		if base == "" {
			base = "https://api.openai.com/v1"
		}
		req, _ := http.NewRequestWithContext(ctx, "GET", base+"/models", nil)
		req.Header.Set("Authorization", "Bearer "+key)
		return keyCheckResponse(req)
	case "gemini":
		base := strings.TrimRight(b.Url, "/")
		if base == "" {
			base = geminiOpenAIBase
		}
		req, _ := http.NewRequestWithContext(ctx, "GET", base+"/models", nil)
		req.Header.Set("Authorization", "Bearer "+key)
		return keyCheckResponse(req)
	}
	return fmt.Errorf("no backend %s", name)
}

func keyCheckResponse(req *http.Request) error {
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 300 {
		return nil
	}
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	json.NewDecoder(res.Body).Decode(&e)
	if e.Error.Message != "" {
		return fmt.Errorf("%s", e.Error.Message)
	}
	return fmt.Errorf("the service answered %s", res.Status)
}

type assistantKeyReq struct {
	Backend string
	Key     string
	Remove  bool
}

/* SDOC: API
* POST /assistant/key — Set or Remove a Writing Assistant Key
	Body ={Backend, Key, Remove}=. The key is tried against the service (a list
	of its models) and kept only if it works: in the OS keyring (service
	=orgs=, account =assistant-<backend>=), else in
	=<config>_assistant_keys.yaml= beside the config, mode 0600. It is used
	ahead of =orgs.yaml='s key and the environment's, at once. =Remove= takes
	it away again. Administrators only (anyone with =noAuth=).

	*Response:* ={Ok, Msg}=; =Msg= says where it was kept, or what was wrong.
EDOC */
func PostAssistantKey(w http.ResponseWriter, r *http.Request) {
	var req assistantKeyReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writingJson(w, common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	if err := mayChangeKeys(r); err != nil {
		writingJson(w, common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	name := strings.ToLower(req.Backend)
	b := assistantBackend(name)
	if b == nil {
		writingJson(w, common.ResultMsg{Ok: false, Msg: "no backend " + req.Backend})
		return
	}
	key := strings.TrimSpace(req.Key)
	if req.Remove {
		if _, err := setStoredKey(name, ""); err != nil {
			writingJson(w, common.ResultMsg{Ok: false, Msg: err.Error()})
			return
		}
		writingJson(w, common.ResultMsg{Ok: true, Msg: "removed"})
		return
	}
	if key == "" {
		writingJson(w, common.ResultMsg{Ok: false, Msg: "paste the key first"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	if err := tryAssistantKey(ctx, name, b, key); err != nil {
		writingJson(w, common.ResultMsg{Ok: false, Msg: "that key did not work: " + err.Error()})
		return
	}
	where, err := setStoredKey(name, key)
	if err != nil {
		writingJson(w, common.ResultMsg{Ok: false, Msg: "the key works but could not be kept: " + err.Error()})
		return
	}
	fmt.Fprintf(os.Stderr, "ASSISTANT: a %s key was set from worg by %q, kept in %s\n", name, GetUsername(r), where)
	writingJson(w, common.ResultMsg{Ok: true, Msg: where})
}
