package common

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type Rest struct {
	Url    string
	Header http.Header
	// How long to wait for the server, when waiting forever is the wrong
	// answer. Zero means no limit, which is right for everything a person is
	// sitting in front of and wrong for a shell completion: a prompt that has
	// stopped responding because a server is slow is worse than one with no
	// suggestions in it.
	Timeout time.Duration
}

// client is the http client for this Rest, with its timeout if it has one.
func (self *Rest) client() *http.Client {
	if self.Timeout > 0 {
		return &http.Client{Timeout: self.Timeout}
	}
	return http.DefaultClient
}

func (self *Rest) Insecure() {
	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
}

func parseJSON[T any](s []byte) (T, error) {
	var r T
	if err := json.Unmarshal(s, &r); err != nil {
		return r, err
	}
	return r, nil
}

func toJSON(T any) ([]byte, error) {
	return json.Marshal(T)
}

func (self *Rest) Get(api string, ps map[string]string) string {
	coreurl := self.Url + "/" + api
	base, err := url.Parse(coreurl)
	if err != nil {
		return fmt.Sprintf("Rest parse error: %s", err)
	}
	if len(ps) > 0 {
		params := url.Values{}

		for k, v := range ps {
			params.Add(k, v)
			base.RawQuery = params.Encode()
		}
	}
	//fmt.Fprintf(os.Stderr, "URL STR: %s\n", base.String())
	req, err := http.NewRequest("GET", base.String(), nil)
	if err != nil {
		return fmt.Sprintf("Rest request error: %s", err)
	}
	req.Header = self.Header.Clone()
	resp, err := self.client().Do(req)
	if err != nil {
		log.Println(err)
		return fmt.Sprintf("Rest call error: %s", err)
	}
	body, rerr := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if rerr != nil {
		log.Println(rerr)
	}
	return string(body)
}

func (self *Rest) Post(api string, data []byte) ([]byte, error) {
	u := self.Url + "/" + api
	req, err := http.NewRequest("POST", u, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("Rest request error: %s", err)
	}
	req.Header = self.Header.Clone()
	req.Header.Set("Content-Type", "application/json")
	resp, err := self.client().Do(req)
	if err != nil {
		log.Println(err)
		return nil, fmt.Errorf("Rest call error: %s", err)
	}
	body, rerr := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if rerr != nil {
		log.Println(rerr)
	}
	return body, nil
}

// Delete is the fourth verb. Several endpoints answer to it and nothing on this
// side could ask - a stored query could be written and never removed, and the
// voice recordings could be listed and never tidied.
//
// It takes query parameters rather than a body, because that is what every
// DELETE handler here reads: `?name=` for a stored query, a path segment for a
// recording.
func (self *Rest) Delete(api string, ps map[string]string) ([]byte, error) {
	coreurl := self.Url + "/" + api
	base, err := url.Parse(coreurl)
	if err != nil {
		return nil, fmt.Errorf("Rest parse error: %s", err)
	}
	if len(ps) > 0 {
		params := url.Values{}
		for k, v := range ps {
			params.Add(k, v)
		}
		base.RawQuery = params.Encode()
	}
	req, err := http.NewRequest("DELETE", base.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("Rest request error: %s", err)
	}
	req.Header = self.Header.Clone()
	resp, err := self.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("Rest call error: %s", err)
	}
	body, rerr := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if rerr != nil {
		return nil, rerr
	}
	return body, nil
}

func RestDelete[T any](self *Rest, api string, ps map[string]string) (T, error) {
	var result T
	body, err := self.Delete(api, ps)
	if err != nil {
		return result, err
	}
	return parseJSON[T](body)
}

// PostFile uploads one file as multipart/form-data, which is the only shape
// `POST /voice/recording` takes - it is an upload rather than a document, and
// base64 in a json body would be a third more bytes for no gain.
//
// field is the form field the handler reads ("audio"), name is the filename it
// is told, which is how the far end knows what container it is looking at.
func (self *Rest) PostFile(api, field, name string, data []byte,
	extra map[string]string) ([]byte, error) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for k, v := range extra {
		if err := w.WriteField(k, v); err != nil {
			return nil, err
		}
	}
	part, err := w.CreateFormFile(field, name)
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", self.Url+"/"+api, &body)
	if err != nil {
		return nil, fmt.Errorf("Rest request error: %s", err)
	}
	req.Header = self.Header.Clone()
	// Set after the clone, because the boundary is this request's own and must
	// not be carried over from whatever the header already said.
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := self.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("Rest call error: %s", err)
	}
	defer resp.Body.Close()
	return ioutil.ReadAll(resp.Body)
}

func RestPostFile[T any](self *Rest, api, field, name string, data []byte,
	extra map[string]string) (T, error) {
	var result T
	body, err := self.PostFile(api, field, name, data, extra)
	if err != nil {
		return result, err
	}
	return parseJSON[T](body)
}

func RestGet[T any](self *Rest, api string, ps map[string]string) T {
	var body []byte = []byte(self.Get(api, ps))
	var data T
	if err := json.Unmarshal(body, &data); err != nil {
		// Stderr: this is a complaint, not an answer, and a caller piping the
		// answer somewhere should not have it spliced into the stream.
		fmt.Fprintln(os.Stderr, "failed to unmarshal:", err, "DATA:\n", string(body))
	}
	return data
}

func RestPost[T any](self *Rest, api string, data any) (T, error) {
	var result T
	sdata, err := toJSON(data)
	if err != nil {
		return result, err
	}
	body, perr := self.Post(api, sdata)
	if perr != nil {
		return result, perr
	}
	return parseJSON[T](body)
}

// GetRaw is Get without the assumption that the answer is a string worth
// keeping. It hands back the bytes, what the server said they were and the
// status, so a caller can tell a pdf from an error page - which Get cannot,
// having already turned both into a string with no status beside it.
func (self *Rest) GetRaw(api string, ps map[string]string) ([]byte, string, int, error) {
	coreurl := self.Url + "/" + api
	base, err := url.Parse(coreurl)
	if err != nil {
		return nil, "", 0, fmt.Errorf("rest parse error: %s", err)
	}
	if len(ps) > 0 {
		params := url.Values{}
		for k, v := range ps {
			params.Add(k, v)
		}
		base.RawQuery = params.Encode()
	}
	req, err := http.NewRequest("GET", base.String(), nil)
	if err != nil {
		return nil, "", 0, fmt.Errorf("rest request error: %s", err)
	}
	req.Header = self.Header.Clone()
	resp, err := self.client().Do(req)
	if err != nil {
		return nil, "", 0, fmt.Errorf("rest call error: %s", err)
	}
	defer resp.Body.Close()
	body, rerr := ioutil.ReadAll(resp.Body)
	if rerr != nil {
		return nil, "", resp.StatusCode, rerr
	}
	return body, resp.Header.Get("Content-Type"), resp.StatusCode, nil
}

// RestGetErr is RestGet with the error handed back rather than printed.
//
// RestGet logs and returns a zero value, which is right for a command that is
// about to print something anyway and wrong for anything that has to decide
// what to do next - a tui that must stay up, or an mcp tool that has to say
// what went wrong rather than answer with an empty list.
func RestGetErr[T any](self *Rest, api string, ps map[string]string) (T, error) {
	var data T
	body, _, status, err := self.GetRaw(api, ps)
	if err != nil {
		return data, err
	}
	if status >= 400 {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 240 {
			msg = msg[:240] + "..."
		}
		return data, fmt.Errorf("%s: %s", http.StatusText(status), msg)
	}
	if err := json.Unmarshal(body, &data); err != nil {
		// A handler that fails answers {Ok: false, Msg} with a 200, which is
		// json, just not the shape asked for. Say what it said.
		var fail struct {
			Ok  bool
			Msg string
		}
		if json.Unmarshal(body, &fail) == nil && !fail.Ok && fail.Msg != "" {
			return data, fmt.Errorf("%s", fail.Msg)
		}
		return data, fmt.Errorf("%s answered with something that is not json: %v", api, err)
	}
	return data, nil
}
