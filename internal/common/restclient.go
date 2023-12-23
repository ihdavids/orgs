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
	resp, err := http.Get(base.String())
	if err != nil {
		log.Println(err)
	}
	if err == nil {
		body, rerr := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		if rerr != nil {
			log.Println(rerr)
		}
		return string(body)
	}
	return fmt.Sprintf("Rest call error: %s", err)
}

func (self *Rest) Post(api string, data []byte) ([]byte, error) {
	url := self.Url + "/" + api
	byteReader := bytes.NewReader(data)
	resp, err := http.Post(url, "application/json", byteReader)
	if err != nil {
		log.Println(err)
	}
	if err == nil {
		body, rerr := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		if rerr != nil {
			log.Println(rerr)
		}
		return body, nil
	}
	return nil, fmt.Errorf("Rest call error: %s", err)
}

func RestGet[T any](self *Rest, api string, ps map[string]string) T {
	var body []byte = []byte(self.Get(api, ps))
	var data T
	if err := json.Unmarshal(body, &data); err != nil {
		fmt.Println("failed to unmarshal:", err, "DATA:\n", string(body))
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
