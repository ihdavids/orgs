package common

// Voice notes: a recording made in a client, transcribed by a go-whisper
// server, filed as an org heading.
//
// The three steps are separate calls on purpose. The recording is saved first
// and on its own, so that a transcription that fails - whisper not running, a
// model still loading, a take longer than the timeout - costs the words that
// were said rather than the whole note. The transcript then comes back to the
// client to be read and corrected before anything is written into an org file,
// because a transcript nobody has looked at is a guess, and an org file is not
// the place to find that out.

// One stretch of a transcription, as go-whisper hands it back. Start and End
// are seconds from the beginning of the recording.
type VoiceSegment struct {
	Id      int     `json:"id"`
	Start   float64 `json:"start"`
	End     float64 `json:"end"`
	Text    string  `json:"text"`
	Speaker string  `json:"speaker,omitempty"`
}

// A recording the server is holding. Id is the file's own name, which is what
// makes it addressable without a store to keep in step with the folder.
type VoiceRecording struct {
	Id      string  `json:"id"`
	Bytes   int64   `json:"bytes"`
	Seconds float64 `json:"seconds"`
	Created string  `json:"created"`
	Mime    string  `json:"mime"`
	// The heading this was filed as, when it has been. A recording is left in
	// place after filing so the note's audio link keeps working.
	Filed string `json:"filed,omitempty"`
}

// What the client needs to know before it offers to record: whether there is
// anything to transcribe with, and what the defaults are.
type VoiceConfig struct {
	Ok    bool   `json:"Ok"`
	Msg   string `json:"Msg"`
	Ready bool   `json:"ready"`
	Url   string `json:"url"`
	// The model a transcription would use right now, which is the configured
	// one, or the server's first when nothing is configured.
	Model    string   `json:"model"`
	Models   []string `json:"models"`
	Language string   `json:"language"`
	MaxMb    int      `json:"maxMb"`
	Tags     []string `json:"tags"`
	Target   Target   `json:"target"`
}

type VoiceTranscribeRequest struct {
	Id       string `json:"id"`
	Model    string `json:"model"`
	Language string `json:"language"`
	// Words the model should expect - names, jargon - which whisper uses to
	// bias its own guesses rather than as an instruction.
	Prompt string `json:"prompt"`
}

type VoiceTranscription struct {
	Ok       bool           `json:"Ok"`
	Msg      string         `json:"Msg"`
	Text     string         `json:"text"`
	Language string         `json:"language"`
	Duration float64        `json:"duration"`
	Model    string         `json:"model"`
	Segments []VoiceSegment `json:"segments"`
}

// Filing a note. Text is whatever the client settled on, which may be an
// edited transcript, a typed note, or both - the server does not transcribe
// again here, because what is written has to be what was on screen.
type VoiceNoteRequest struct {
	Id       string            `json:"id"`
	Headline string            `json:"headline"`
	Text     string            `json:"text"`
	Tags     []string          `json:"tags"`
	Target   Target            `json:"target"`
	Props    map[string]string `json:"props"`
	// Write the timestamped segments under the note as a list.
	Segments []VoiceSegment `json:"segments"`
	WithTimes bool          `json:"withTimes"`
	// Keep the audio and link the heading to it. Off throws the recording away
	// once the words are safely in the file.
	KeepAudio bool    `json:"keepAudio"`
	Model     string  `json:"model"`
	Seconds   float64 `json:"seconds"`
}

type VoiceNoteResult struct {
	Ok       bool   `json:"Ok"`
	Msg      string `json:"Msg"`
	Filename string `json:"filename"`
	Line     int    `json:"line"`
	Headline string `json:"headline"`
	Audio    string `json:"audio"`
}
