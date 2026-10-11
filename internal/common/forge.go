package common

// ForgeSync is what one github or gitlab poller did on a poll, or found when
// asked whether its token works (User). The CLI prints it; the server's
// /forge endpoints answer with a list of them, one per configured poller.
type ForgeSync struct {
	Name string // github or gitlab
	File string
	Url  string
	Ok   bool
	Msg  string
	User string `json:",omitempty"`
	// Headings written for new items, and headings whose title, tags or
	// properties changed.
	Added   int
	Updated int
	// Keyword changes made because upstream changed: closed or merged there,
	// reopened there, or no longer in any of the queries.
	Closed   int
	Reopened int
	Gone     int
	// Closes and reopens sent upstream because a heading changed here.
	Pushed int
	// One line per thing worth telling a person, e.g. "closed o/r#12 upstream".
	Notes []string `json:",omitempty"`
}

type ForgeReply struct {
	Ok      bool
	Msg     string
	Results []ForgeSync
}
