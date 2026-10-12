package common

import (
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
)

// Version, Commit and BuildDate are set at build time:
//
//	go build -ldflags "-X github.com/ihdavids/orgs/internal/common.Version=v1.2.3 \
//	    -X github.com/ihdavids/orgs/internal/common.Commit=$(git rev-parse --short HEAD) \
//	    -X github.com/ihdavids/orgs/internal/common.BuildDate=$(date -u +%FT%TZ)" ./cmd/orgs
//
// .goreleaser.yaml does this for a release. A plain `go build` leaves Version
// at "dev", and BuildVersion fills the commit and date in from the vcs stamp
// Go records, so even a local build says which commit it was.
var (
	Version   = "dev"
	Commit    = ""
	BuildDate = ""
)

// VersionInfo is what `orgs version` prints and GET /version answers with.
// Latest, UpdateAvailable and ReleaseUrl are only filled in by the server,
// from the newest release on github.
type VersionInfo struct {
	Version         string `json:"version"`
	Commit          string `json:"commit,omitempty"`
	Date            string `json:"date,omitempty"`
	Go              string `json:"go"`
	Os              string `json:"os"`
	Arch            string `json:"arch"`
	Latest          string `json:"latest,omitempty"`
	UpdateAvailable bool   `json:"updateAvailable"`
	ReleaseUrl      string `json:"releaseUrl,omitempty"`
	// Why Latest is missing, when it is: the check is turned off, github
	// could not be reached, or there is no release yet.
	CheckError string `json:"checkError,omitempty"`
}

// BuildVersion is this binary's version.
func BuildVersion() VersionInfo {
	v := VersionInfo{Version: Version, Commit: Commit, Date: BuildDate, Go: runtime.Version(), Os: runtime.GOOS, Arch: runtime.GOARCH}
	if bi, ok := debug.ReadBuildInfo(); ok {
		// `go install github.com/ihdavids/orgs/cmd/orgs@v1.2.3` stamps the
		// module version rather than taking ldflags.
		if v.Version == "dev" && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
			v.Version = bi.Main.Version
		}
		dirty := false
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				if v.Commit == "" {
					v.Commit = s.Value
					if len(v.Commit) > 12 {
						v.Commit = v.Commit[:12]
					}
				}
			case "vcs.time":
				if v.Date == "" {
					v.Date = s.Value
				}
			case "vcs.modified":
				dirty = s.Value == "true"
			}
		}
		if dirty && Commit == "" && v.Commit != "" {
			v.Commit += "-dirty"
		}
	}
	return v
}

// IsRelease is whether v is a version number rather than a development build:
// only then can it be compared with a release.
func IsRelease(v string) bool {
	_, ok := parseSemver(v)
	return ok
}

// NewerVersion is whether a is a later version than b. Both are semver
// ("v1.2.3", "1.2.3-rc.1"); anything that is not is never newer.
func NewerVersion(a, b string) bool {
	pa, oka := parseSemver(a)
	pb, okb := parseSemver(b)
	if !oka || !okb {
		return false
	}
	for i := 0; i < 3; i++ {
		if pa.n[i] != pb.n[i] {
			return pa.n[i] > pb.n[i]
		}
	}
	// 1.2.3 is newer than 1.2.3-rc.1.
	switch {
	case pa.pre == pb.pre:
		return false
	case pa.pre == "":
		return true
	case pb.pre == "":
		return false
	}
	return comparePre(pa.pre, pb.pre) > 0
}

type semver struct {
	n   [3]int
	pre string
}

func parseSemver(v string) (semver, bool) {
	var s semver
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexByte(v, '+'); i >= 0 {
		v = v[:i]
	}
	if i := strings.IndexByte(v, '-'); i >= 0 {
		s.pre = v[i+1:]
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return s, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return s, false
		}
		s.n[i] = n
	}
	return s, true
}

// comparePre orders prerelease tags the semver way: dot separated, numeric
// parts numerically and below alphanumeric ones, a shorter list first.
func comparePre(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		an, aerr := strconv.Atoi(as[i])
		bn, berr := strconv.Atoi(bs[i])
		switch {
		case aerr == nil && berr == nil:
			if an != bn {
				if an > bn {
					return 1
				}
				return -1
			}
		case aerr == nil:
			return -1
		case berr == nil:
			return 1
		default:
			if c := strings.Compare(as[i], bs[i]); c != 0 {
				return c
			}
		}
	}
	return len(as) - len(bs)
}
