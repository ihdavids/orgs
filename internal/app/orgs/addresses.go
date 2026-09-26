package orgs

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
)

// Where else this server can be reached.
//
// The reason this exists: the address in the browser's bar is almost always
// `localhost`, and a phone pointed at `localhost` reaches itself. The page has
// no way to find out the machine's address on the network - only the machine
// does - so it asks.
//
// Every non-loopback address the machine has, because which one a phone can
// reach depends on which network it is on, and a laptop on wifi with a docking
// station has several. They are offered rather than guessed between.

type ServerAddresses struct {
	Ok bool
	// Reachable urls, best guess first.
	Urls []string
	// What the server is listening on, for saying so when there are none.
	Port    int
	TlsPort int
	Msg     string
}

// A private address is the one a phone on the same wifi can actually reach, so
// those go first; anything else the machine happens to hold comes after.
func privateIPv4(ip net.IP) bool {
	v4 := ip.To4()
	if v4 == nil {
		return false
	}
	switch {
	case v4[0] == 10:
		return true
	case v4[0] == 172 && v4[1] >= 16 && v4[1] <= 31:
		return true
	case v4[0] == 192 && v4[1] == 168:
		return true
	}
	return false
}

func RequestServerAddresses(w http.ResponseWriter, r *http.Request) {
	res := ServerAddresses{Urls: []string{}}
	sets := Conf().Server
	res.Port = sets.Port
	res.TlsPort = sets.TLSPort

	ifaces, err := net.Interfaces()
	if err != nil {
		res.Msg = err.Error()
		addressJson(w, res)
		return
	}

	var private, other []string
	for _, iface := range ifaces {
		// A cable that is unplugged still has an interface, and its address is
		// of no use to anybody.
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			// IPv4 only: an address a phone can be typed at, and a link-local
			// v6 address needs a zone index that means nothing off this
			// machine.
			if ip == nil || ip.To4() == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}
			host := ip.String()
			if sets.AllowHttp {
				url := fmt.Sprintf("http://%s:%d", host, sets.Port)
				if privateIPv4(ip) {
					private = append(private, url)
				} else {
					other = append(other, url)
				}
			}
			if sets.AllowHttps {
				url := fmt.Sprintf("https://%s:%d", host, sets.TLSPort)
				if privateIPv4(ip) {
					private = append(private, url)
				} else {
					other = append(other, url)
				}
			}
		}
	}

	res.Urls = append(private, other...)
	res.Ok = true
	if len(res.Urls) == 0 {
		res.Msg = "this machine has no address on a network other than its own"
	}
	addressJson(w, res)
}

func addressJson(w http.ResponseWriter, res ServerAddresses) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}
