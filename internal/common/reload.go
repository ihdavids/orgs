package common

// ReloadResult is what POST /reloadconfig answers with: what was taken from the
// file again, and what changed in it that only a restart will pick up.
type ReloadResult struct {
	Ok           bool     `json:"ok"`
	Msg          string   `json:"msg"`
	File         string   `json:"file"`
	Reloaded     []string `json:"reloaded"`
	NeedsRestart []string `json:"needsRestart,omitempty"`
}
