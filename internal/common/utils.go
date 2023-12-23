package common

import (
	"fmt"
	"reflect"
	"regexp"
	"runtime"
	"time"

	"github.com/ihdavids/go-org/org"
)

// Extract the title from the nodes in a headline.
func GetHeadlineTitle(h *org.Headline) string {
	var title string = ""
	if h != nil {
		for _, n := range h.Title {
			if n != nil {
				title += n.String()
			}
		}
	}
	return title
}

func GetHeadlineBody(h *org.Headline) string {
	if h != nil && h.Children != nil && len(h.Children) > 0 {
		w := org.NewOrgWriter()
		org.WriteNodes(w, h.Children...)
		return w.String()
	}
	return ""
}

// Convenience wrapper for a section to get the title.
func GetSectionTitle(cur *org.Section) string {
	return GetHeadlineTitle(cur.Headline)
}

func GetSectionBody(cur *org.Section) string {
	return GetHeadlineBody(cur.Headline)
}

// Build a seperator delimited path string for the given section
func BuildOutlinePath(s *org.Section, separator string) string {
	path := GetHeadlineTitle(s.Headline)
	for s.Parent != nil && path != "" {
		s = s.Parent
		canSkip := s.Parent == nil
		tit := GetHeadlineTitle(s.Headline)
		if !canSkip || tit != "" {
			path = tit + separator + path
		}
	}
	return path
}

// this will return the name of a function type
func GetFunctionName(i interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
}

type Match struct {
	Start   int
	End     int
	Capture string
	Name    string
	Idx     int
}

func (s *Match) String() string {
	return fmt.Sprintf("{I: %d, Name: %v, S: %d, E: %d, C: '%s'}", s.Idx, s.Name, s.Start, s.End, s.Capture)
}

func (s *Match) Have() bool {
	return s.Start != -1 && s.End != -1
}

func ReMatch(regEx *regexp.Regexp, txt string) (paramsMap []map[string]*Match) {
	matches := regEx.FindAllSubmatchIndex([]byte(txt), -1)
	res := []map[string]*Match{}
	for _, match := range matches {
		paramsMap := make(map[string]*Match)
		for i, name := range regEx.SubexpNames() {
			ssub := match[i*2]
			esub := match[i*2+1]
			if ssub >= 0 && ssub < esub && esub <= len(txt) {
				var m = &Match{}
				m.Start = ssub
				m.End = esub
				if name == "" {
					name = fmt.Sprintf("%d", i)
				}
				m.Idx = i
				m.Name = name
				if m.Start >= 0 && m.Start < m.End && m.End <= len(txt) {
					m.Capture = txt[m.Start:m.End]
					paramsMap[name] = m
				}
			}
		}
		res = append(res, paramsMap)
	}
	return res
}

func ReplaceSection(txt string, s, e int, toInsert string) string {
	if s >= e {
		return txt
	}
	if s < 0 {
		s = 0
	}
	if e > len(txt) {
		e = len(txt)
	}
	if s == 0 && e == len(txt) {
		return toInsert
	}
	if s == 0 {
		return toInsert + txt[e:]
	}
	if e == len(txt) {
		return txt[:s] + toInsert
	}
	return txt[:s] + toInsert + txt[e:]
}

func ParseDateString(p string) (time.Time, error) {
	var err error
	var tm time.Time
	// Built in time format
	if tm, err = time.Parse("<2006-01-02 Mon 15:04>", p); err == nil {
		return tm, nil
	}
	// RFC1123     = "Mon, 02 Jan 2006 15:04:05 MST"
	if tm, err = time.Parse(time.RFC1123, p); err == nil {
		return tm, nil
	}
	// RFC1123Z    = "Mon, 02 Jan 2006 15:04:05 -0700"
	if tm, err = time.Parse(time.RFC1123Z, p); err == nil {
		return tm, nil
	}
	// RFC3339     = "2006-01-02T15:04:05Z07:00"
	if tm, err = time.Parse(time.RFC3339, p); err == nil {
		return tm, nil
	}
	// ANSIC       = Mon Jan 02 15:04:05 2006
	if tm, err = time.Parse(time.ANSIC, p); err == nil {
		return tm, nil
	}
	// UnixDate Mon Jan 02 15:04:05 MST 2006
	if tm, err = time.Parse(time.UnixDate, p); err == nil {
		return tm, nil
	}
	return time.Time{}, fmt.Errorf("failed to parse date time")
}
