package common

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
)

type Rest struct {
	Url    string
	Header http.Header
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
	//fmt.Printf("URL STR: %s\n", base.String())
	req, err := http.NewRequest("GET", base.String(), nil)
	if err != nil {
		return fmt.Sprintf("Rest request error: %s", err)
	}
	req.Header = self.Header.Clone()
	resp, err := http.DefaultClient.Do(req)
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
	resp, err := http.DefaultClient.Do(req)
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
	resp, err := http.DefaultClient.Do(req)
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
		return data, fmt.Errorf("%s answered with something that is not json: %v", api, err)
	}
	return data, nil
}
