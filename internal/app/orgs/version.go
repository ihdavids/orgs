package orgs

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ihdavids/orgs/internal/common"
)

// The repository releases are published to, and how long an answer from it is
// trusted. A failed check is retried sooner, so a laptop that started offline
// finds out once it is back.
const (
	releaseRepo      = "ihdavids/orgs"
	releaseCacheFor  = 6 * time.Hour
	releaseRetryFail = 15 * time.Minute
)

var latestRelease struct {
	sync.Mutex
	tag, url, err string
	at            time.Time
}

// LatestRelease is the newest published release's tag and page, from the
// cache when it is fresh enough.
func LatestRelease() (tag, url string, err error) {
	latestRelease.Lock()
	defer latestRelease.Unlock()
	fresh := releaseCacheFor
	if latestRelease.err != "" {
		fresh = releaseRetryFail
	}
	if !latestRelease.at.IsZero() && time.Since(latestRelease.at) < fresh {
		if latestRelease.err != "" {
			return "", "", fmt.Errorf("%s", latestRelease.err)
		}
		return latestRelease.tag, latestRelease.url, nil
	}
	tag, url, err = fetchLatestRelease()
	latestRelease.tag, latestRelease.url, latestRelease.at = tag, url, time.Now()
	latestRelease.err = ""
	if err != nil {
		latestRelease.err = err.Error()
	}
	return tag, url, err
}

func fetchLatestRelease() (string, string, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("GET", "https://api.github.com/repos/"+releaseRepo+"/releases/latest", nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "orgs/"+common.Version)
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", "", fmt.Errorf("no release published yet")
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("github answered %s", resp.Status)
	}
	var rel struct {
		TagName string `json:"tag_name"`
		HtmlUrl string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", "", err
	}
	return rel.TagName, rel.HtmlUrl, nil
}

// ServerVersion is this server's version, with the latest release beside it
// unless the check is turned off.
func ServerVersion() common.VersionInfo {
	v := common.BuildVersion()
	if Conf().Server != nil && Conf().Server.NoUpdateCheck {
		v.CheckError = "update check turned off (noUpdateCheck)"
		return v
	}
	tag, url, err := LatestRelease()
	if err != nil {
		v.CheckError = err.Error()
		return v
	}
	v.Latest, v.ReleaseUrl = tag, url
	// A development build cannot be compared with a release, so it is never
	// told to update; Latest still says what is out.
	v.UpdateAvailable = common.NewerVersion(tag, v.Version)
	return v
}

/* SDOC: API
* GET /version — Server Version and Update Check

	Which version of orgs the server is, and whether a newer release is out.
	Clients (the vscode plugin, =orgs version=) use it to offer an update. The
	latest release is asked of github at most every six hours; set
	=noUpdateCheck: true= in the server settings to never ask.

	*Parameters:* None.

	*Response:*
	#+BEGIN_SRC json
	{"version": "v1.2.0", "commit": "da3ec47", "date": "2026-10-11T12:00:00Z",
	 "go": "go1.24.2", "os": "darwin", "arch": "arm64",
	 "latest": "v1.3.0", "updateAvailable": true,
	 "releaseUrl": "https://github.com/ihdavids/orgs/releases/tag/v1.3.0"}
	#+END_SRC

	=version= is ="dev"= for a build made without a version (plain =go build=),
	and such a build never reports an update. =checkError= says why =latest= is
	missing, when it is.
EDOC */

func RequestVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ServerVersion())
}

// versionLine is the version for the startup log.
func versionLine() string {
	v := common.BuildVersion()
	parts := []string{v.Version}
	if v.Commit != "" {
		parts = append(parts, v.Commit)
	}
	if v.Date != "" {
		parts = append(parts, v.Date)
	}
	return strings.Join(parts, " ")
}
