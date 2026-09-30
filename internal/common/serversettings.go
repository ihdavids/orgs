package common

import (
	//"crypto/sha1"
	"crypto/rand"
	//"crypto/rsa"
	b64 "encoding/base64"
	"fmt"
	"log"
	mrand "math/rand"
	"os"
	"time"
)

const KBAD_SALT = "THIS IS A DEFAULT SALT DO NOT USE THIS! SET YOUR OWN"

// AttachSettings is where a heading's own files are kept and what is done to
// the heading when it gets one.
//
// The defaults are org-attach's own, so a database orgs attaches to is one
// emacs can read and the other way round.
// LogSettings is what orgs writes down when a heading changes state - org's
// org-log-done, org-log-repeat and org-log-into-drawer, under those names.
//
// Every field is a string rather than a bool because org's own settings are
// three-valued: not at all, a timestamp, or a timestamp and a note. A bool
// could not say the third thing and would have to grow a second setting beside
// it to do so.
type LogSettings struct {
	Done          string `yaml:"done"`
	Repeat        string `yaml:"repeat"`
	IntoDrawer    string `yaml:"intoDrawer"`
	States        bool   `yaml:"states"`
	RepeatToState string `yaml:"repeatToState"`
}

type AttachSettings struct {
	// The root of the attachment store, relative to the first orgDir. A
	// heading's folder is this, then its id split two characters deep.
	// "data" when empty, which is what org-attach-id-dir is.
	Dir string `yaml:"dir"`
	// The tag put on a heading the first time something is attached to it.
	// "ATTACH" when empty, which is what org-attach-auto-tag is; set it to
	// "-" to tag nothing.
	Tag string `yaml:"tag"`
	// The biggest single upload to take, in megabytes. 64 when unset.
	MaxMb int `yaml:"maxMb"`
}

// VoiceSettings is what a voice note needs: where the transcription service
// is, what it should transcribe with, and where the recording and the heading
// it becomes are put.
//
// Orgs transcribes nothing itself. It hands the recording to a go-whisper
// server over its own http api and files what comes back, which is what keeps
// the model, the hardware it runs on and the org side of this independent of
// one another.
type VoiceSettings struct {
	// The models directory. Naming one is what turns voice on: orgs starts a
	// gowhisper of its own against it and looks after it for as long as the
	// server runs. Left empty, orgs talks to whatever is already at Url.
	Models string `yaml:"models"`
	// The port to run it on. Also where orgs looks for it, so a server started
	// by hand on the same port is adopted rather than duplicated.
	Port int `yaml:"port"`
	// The gowhisper binary. Found on PATH, and in a few of the usual places,
	// when this is empty.
	Bin string `yaml:"bin"`
	// Let whisper use the GPU. On unless this says otherwise.
	Gpu *bool `yaml:"gpu"`
	// Anything else to put on the gowhisper command line.
	Args []string `yaml:"args"`
	// Where go-whisper is listening, for a server orgs does not start. Its api
	// lives under /api/whisper there. Worked out from Port when left empty.
	Url string `yaml:"url"`
	// The model id to transcribe with. Empty means ask the server for its
	// models and take the first - right for a machine with one installed.
	Model string `yaml:"model"`
	// A two letter language code, or empty to let whisper work it out.
	Language string `yaml:"language"`
	// Where recordings are kept, relative to the first orgDir, so that a
	// note's audio sits in the org database beside the heading linking to it.
	Dir string `yaml:"dir"`
	// Seconds to wait for a transcription. A long take on a large model is
	// minutes, so this is not the usual http timeout.
	Timeout int `yaml:"timeout"`
	// The largest single recording that will be accepted, in megabytes.
	MaxMb int `yaml:"maxMb"`
	// Tags put on every voice note heading.
	Tags []string `yaml:"tags"`
	// Where a voice note is filed when the client does not say. Same shape as
	// a capture template's target.
	Target Target `yaml:"target"`
}

type ServerSettings struct {
	/* SDOC: Settings
	* Orgs Keys
		There are 2 keys that will be generated
		if not provided by the system.
		#+BEGIN_SRC yaml
	  orgJWS: "this key is used to sign the JWT"
	  orgJWE: "This key is used to encrypt the JWT"
	  orgSalt: "Appended to the per user salt to help with rainbow tables"
		#+END_SRC
		EDOC */
	//OrgJWS  string `yaml:"orgJWS"`
	//OrgJWS  *rsa.PrivateKey `yaml:"orgJWS"`
	OrgJWS      string `yaml:"orgJWS"`
	OrgJWE      string `yaml:"orgJWE"`
	OrgSalt     string `yaml:"orgSalt"`
	TokenExpiry string `yaml:"tokenExpiry"`

	/* SDOC: Settings
	* Orgs Keystore
		Orgs has various ways of storing credentials. You need something to protect your
		data. By default the yaml keystore is just a file with usernames and creds.
		Other keystores are possible.
		#+BEGIN_SRC yaml
	  keystore: "path to yaml file"
		#+END_SRC
		EDOC */
	Keystore string `yaml:"keystore"`

	// Configuration options
	ServePath string `yaml:"servepath"`
	Port      int    `yaml:"port"`
	TLSPort   int    `yaml:"tlsport"`
	ServerCrt string `yaml:"servercrt"`
	ServerKey string `yaml:"serverkey"`
	/* SDOC: Settings
	* Org Dirs
		A list of directories containing your org files
		#+BEGIN_SRC yaml
	  orgDirs:
	    - "/Users/me/dev/gtd"
		#+END_SRC
		EDOC */
	OrgDirs      []string `yaml:"orgDirs"`
	CanFailWatch bool     `yaml:"canWatchFail"`
	/* SDOC: Settings
	* Use Project Tag
		How should we define a project. If this is set a project
		is defined as a heading with a :PROJECT: tag on it.

		#+BEGIN_SRC yaml
	  useProjectTag: true
		#+END_SRC

		If this is false then a project is defined to be a headline
		that has a child headline that has a status on it.

		#+BEGIN_SRC org
	  * Project
	  ** TODO This task makes it a project
		#+END_SRC
		EDOC */
	UseTagForProjects bool   `yaml:"useProjectTag"`
	AllowHttp         bool   `yaml:"allowHttp"`
	AllowHttps        bool   `yaml:"allowHttps"`
	NoAuth            bool   `yaml:"noAuth"`
	DefaultTodoStates string `yaml:"defaultTodoStates"`
	DefaultNextStates string `yaml:"defaultNextStates"`
	TemplatePath      string `yaml:"templatePath"`
	/* SDOC: Settings
	* Dnd Paths
		Where the dnd module looks for ruleset yaml modules (extra races,
		classes, backgrounds, spells or whole homebrew rulesets). The built in
		SRD data is always loaded first, these directories are merged on top.

		#+BEGIN_SRC yaml
	  dndPaths:
	    - "/Users/me/dnd/homebrew"
		#+END_SRC

		When this is empty the module still searches =<templatePath>/dnd=,
		=./templates/dnd= and =~/.orgs/dnd=.
		EDOC */
	DndPaths []string `yaml:"dndPaths"`
	/* SDOC: Settings
	* Dnd Session Path
		Where play session logs are written. Starting a session from a
		character sheet creates a dated org file in this folder and every roll
		you make and note you take is appended to it.

		#+BEGIN_SRC yaml
	  dndSessionPath: "/Users/me/dev/gtd/dndsessions"
	  dndSessionTemplate: "dndsession.tpl"
		#+END_SRC

		A relative path is taken relative to your first orgDir, and when the
		setting is left out entirely the folder is =<orgDir>/dndsessions=. Keep
		it inside an orgDir so that the sessions land in your org database and
		can be searched, agenda'd and archived like everything else.
		EDOC */
	DndSessionPath     string `yaml:"dndSessionPath"`
	DndSessionTemplate string `yaml:"dndSessionTemplate"`
	DayPageTemplate    string `yaml:"dayPageTemplate"`
	/* SDOC: Settings
	* Day Page
		The day page system has a number of settings that can be used to control
		its behaviour.

		The first and most important is where your daypages should be generated
		This should be a folder inside your orgDirs.
		#+BEGIN_SRC yaml
	  dayPagePath: "/Users/me/dev/gtd/worklog"
		#+END_SRC
		EDOC */
	DayPagePath          string      `yaml:"dayPagePath"`
	DayPageMode          string      `yaml:"dayPageMode"`
	DayPageModeWeekDay   string      `yaml:"dayPageModeWeekDay"`
	DayPageMaxSearchBack int         `yaml:"dayPageMaxSearch"`
	Plugins              []PluginDef `yaml:"plugins"`
	/* SDOC: Settings
	* Enabled Exporters, Plugins, Updaters
		The list of enabled exporter modules
		#+BEGIN_SRC yaml
		exporters:
	    - name: "gantt"
	    - name: "mermaid"
	    - name: "mindmap"
	    - name: "html"
	      props:
	        fontfamily: "Underdog"
	    - name: "revealjs"
	    - name: "impressjs"
	    - name: "latex"
		#+END_SRC

		The same is true for updaters and plugins.
		You must explicitly enable the modules you wish
		to be active in your orgs installation for them to be available.
		EDOC */
	Exporters        []ExportDef       `yaml:"exporters"`
	Updaters         []UpdaterDef      `yaml:"updaters"`
	CaptureTemplates []CaptureTemplate `yaml:"captureTemplates"`
	AccessControl    string            `yaml:"accessControl"`
	// These specify a valid set of org files that can be searched for valid
	// refile targets
	RefileTargets []string `yaml:"refileTargets"`
	/* SDOC: Settings
	* Voice Notes
		Recording and transcription. Orgs does not transcribe anything itself -
		it runs a [[https://github.com/mutablelogic/go-whisper][go-whisper]]
		server, hands it the recording, and files what comes back as an org
		heading.

		Two settings turn the whole thing on: where the models are, and the port
		to run whisper on.

		#+BEGIN_SRC yaml
	  voice:
	    models: "/Users/me/whisper/models"
	    port: 8081
		#+END_SRC

		With those, orgs starts =gowhisper= itself when the server starts and
		stops it when the server stops. The binary is looked for on your PATH
		and in the usual places; =bin= says where it is when it is somewhere
		else. Something already listening on that port is adopted rather than
		started again, so a whisper you run by hand still works.

		Everything else has a default worth leaving alone:

		#+BEGIN_SRC yaml
	  voice:
	    models: "/Users/me/whisper/models"
	    port: 8081
	    bin: "/usr/local/bin/gowhisper"   # when it is not on PATH
	    gpu: true                          # let whisper use the GPU
	    args: []                           # anything else for its command line
	    url: "http://otherbox:8081"        # a whisper orgs does not start
	    model: "ggml-medium-q5_0"          # empty asks the server for its first
	    language: "en"                     # empty lets whisper detect it
	    dir: "audio"                       # recordings, relative to your orgDir
	    timeout: 600                       # seconds to wait for a transcription
	    maxMb: 64                          # the largest single recording
	    tags:
	      - "voice"
	    target:
	      type: "file+headline"
	      filename: "inbox.org"
	      id: "Voice Notes"
		#+END_SRC

		=model= is the model id to transcribe with - when it is empty orgs asks
		the server what it has and uses the first, which is right for a machine
		with one model installed and wrong as soon as there are two.

		=dir= is where recordings are kept, relative to your first orgDir, so
		that a note's audio sits in the org database beside the heading that
		links to it. =timeout= is how long to wait for a transcription - a long
		take on a large model is minutes, not seconds.

		=target= is where a voice note is filed when the client does not say,
		and takes the same shape as a capture template's target. =tags= are put
		on every voice note heading.
		EDOC */
	Voice VoiceSettings `yaml:"voice"`
	/* SDOC: Settings
	* Attachments

		Files that belong to a heading - a pdf, a photograph of a whiteboard, a
		spreadsheet somebody sent - kept the way org-attach keeps them.

		#+BEGIN_SRC yaml
		attach:
		  dir: "data"
		  tag: "ATTACH"
		  maxMb: 64
		#+END_SRC

		Nothing here is required; every value above is the default.

		=dir= is where a heading's folder is made, relative to your first
		orgDir. A heading's folder is that directory, then the first two
		characters of its =:ID:=, then the rest of it - =data/8f/3c1a20-.../= -
		which is exactly what emacs' org-attach does, so a database written by
		either is readable by the other.

		A heading can also name its own folder with a =:DIR:= property (or the
		older =:ATTACH_DIR:=), which is read relative to the org file that
		holds the heading. A heading with one of those does not need an =:ID:=
		and will not be given one.

		=tag= is put on a heading the first time something is attached to it,
		so that =IsTask() && HasTags("ATTACH")= finds everything with a file on
		it. Set it to an empty string to tag nothing.

		=maxMb= is the biggest single upload that will be taken.
		EDOC */
	Attach AttachSettings `yaml:"attach"`
	/* SDOC: Settings
	* Encryption

		A heading tagged =:crypt:= whose body is ciphertext on disk - org-crypt's
		format exactly, so a heading encrypted here opens in emacs and one
		encrypted in emacs opens here.

		#+BEGIN_SRC yaml
		crypt:
		  key: ""             # a gpg key id. Empty means symmetric.
		  tag: "crypt"
		  bin: ""             # the gpg binary, found on PATH when empty
		  unlockSeconds: 0
		#+END_SRC

		Nothing here is required and there is nothing to turn on: if gpg is
		installed, =/crypt/*= works. =GET /crypt/config= says which mode it is
		in and how many tagged headings are still sitting in the clear.

		**What this protects, and what it does not.** It protects the file - a
		backup, a sync folder, a git remote, a stolen disk - and every part of
		this server that reads org files without going through the crypt
		endpoints: the search index, grep, the link graph, every exporter. None
		of them had to be taught to keep a secret, because there is no secret in
		what they read.

		It does not on its own protect against somebody who can reach the
		server. That is a question about who holds the key, which is why **the
		server holds no passphrase**: one arrives with the request that needs
		it, is handed to gpg down a pipe, and is forgotten.

		=key= is the arrangement worth having on a server other machines can
		reach. With a public key the server encrypts with no secret at all and
		**cannot decrypt**, whatever is done to it - decryption needs the
		private key, which lives where you keep it. A heading can name its own
		with a =:CRYPTKEY:= property, as org-crypt does.

		=unlockSeconds= holds a passphrase in memory for that long so it does
		not have to be retyped. It is off by default and is a number rather than
		a switch because it trades the property above away: for those seconds,
		reaching the server is enough.
		EDOC */
	Crypt CryptSettings `yaml:"crypt"`
	/* SDOC: Settings
	* Marking Something Done

		Org does three things when a heading reaches a DONE keyword, and orgs
		used to do none of them: it stamps =CLOSED:=, it writes a line saying
		which state the heading moved from and when, and - if the heading
		repeats - it moves the repeating date on instead of leaving the keyword
		sitting on DONE forever.

		These are the same three switches org has, under the same names, and
		they can be set here, per file with =#+STARTUP:=, or per heading with a
		=:LOGGING:= property. The narrowest one that says anything wins.

		#+BEGIN_SRC yaml
		log:
		  done: "time"          # none | time | note
		  repeat: "time"        # none | time | note
		  intoDrawer: "LOGBOOK" # a drawer name, or "" to write into the body
		  states: false         # log every state change, not only done ones
		  repeatToState: ""     # a keyword, "previous", or "" for the first one
		#+END_SRC

		=done= is org's =org-log-done=. =time= stamps =CLOSED:= and writes a
		=State "DONE" from "NEXT" [...]= line; =note= does the same and keeps a
		note with it; =none= does neither. =repeat= is =org-log-repeat= and says
		what to write when a repeating date moves on - a repeat is not a
		closure, so no =CLOSED:= is stamped and any existing one is taken off.

		=intoDrawer= is =org-log-into-drawer=. A name puts the lines in that
		drawer; empty writes them into the body as a plain list, which is what
		org does by default. It defaults to =LOGBOOK= here rather than to
		nothing, because the agenda's habit graph is built by reading those
		lines back out of =LOGBOOK= and can only work if that is where they go.

		=states= logs every change between keywords rather than only the ones
		into a DONE state. Per-keyword cookies in a =#+TODO:= line say the same
		thing for one keyword and always win: =NEXT(n!)= logs a timestamp on
		entering NEXT whatever =states= says, and =WAITING(w@)= keeps a note.

		=repeatToState= is =org-todo-repeat-to-state=: which keyword a repeating
		heading goes back to. Empty means the first keyword of the sequence,
		which is what org does; =previous= means whichever state it was in
		before it was marked done; anything else is used as the keyword.

		At the file level, the =#+STARTUP:= words org defines all work:
		=logdone=, =nologdone=, =lognotedone=, =logrepeat=, =nologrepeat=,
		=lognoterepeat=, =logdrawer= and =nologdrawer=. So does
		=#+PROPERTY: LOG_INTO_DRAWER=, and a heading's own =:LOG_INTO_DRAWER:=
		or =:LOGGING:= property.
		EDOC */
	Log LogSettings `yaml:"log"`
	/* SDOC: Settings
	* Column View

		=#+COLUMNS:= is org's way of saying "show me these properties as a table",
		and the part that makes it more than a table is that a column may ask for
		a total: =%EFFORT{:}= makes a project heading show the effort of
		everything under it.

		#+BEGIN_SRC yaml
		columns:
		  default: "%25ITEM %TODO %3PRIORITY %TAGS %EFFORT{:} %CLOCKSUM"
		#+END_SRC

		This is used only for a file that does not declare a =#+COLUMNS:= line of
		its own; one that does gets exactly what it asked for. The built-in
		default is org's own (=%25ITEM %TODO %3PRIORITY %TAGS=) with
		=%EFFORT{:} %CLOCKSUM= added, because adding effort up is the thing this
		view exists for and nearly no file declares a columns line.
		EDOC */
	Columns ColumnSettings `yaml:"columns"`
	/* SDOC: Settings
	* Running Source Blocks

		Orgs can run the =#+BEGIN_SRC= blocks in your files and hand the result
		back - what org calls babel. It is **off unless you turn it on**, and
		that is deliberate: a source block is somebody else's program, the
		server may be reachable from anything on your network, and =noAuth= is a
		setting people use. Reading your files and running them are different
		promises.

		#+BEGIN_SRC yaml
		babel:
		  enable: true
		  languages: ["python", "sh", "emacs-lisp"]
		  timeout: 30
		#+END_SRC

		=languages= is an allow list. Empty means every language orgs knows how
		to run, which is the shorter way of saying yes to all of them. The
		names are the ones you write after =#+BEGIN_SRC=, and the aliases go
		with them - allowing =python= allows =py= too.

		=timeout= is in seconds and is a wall clock limit on one block, so a
		loop that never ends costs half a minute rather than the server.

		=commands= replaces how a language is run, for an interpreter that is
		not on the path or one orgs does not know:

		#+BEGIN_SRC yaml
		babel:
		  enable: true
		  commands:
		    python: ["/usr/local/bin/python3.12"]
		    julia: ["julia", "--startup-file=no"]
		#+END_SRC

		The code is handed to the command on standard input.
	EDOC */
	Babel BabelSettings `yaml:"babel"`
	/* SDOC: Settings
	* Default Author
		Default author parameter to use when generating new templates
		#+BEGIN_SRC yaml
		 author: "John Smith"
		#+END_SRC
		EDOC */
}

func (self *ServerSettings) GetTokenExpiry() time.Duration {
	if self.TokenExpiry == "" {
		return 1 * time.Hour
	}
	if d, err := time.ParseDuration(self.TokenExpiry); err == nil {
		return d
	}
	log.Default().Printf("WARNING: Invalid tokenExpiry %q, using default 1h\n", self.TokenExpiry)
	return 1 * time.Hour
}

func (self *ServerSettings) Validate() {
	// You HAVE to have a orgdir
	if len(self.OrgDirs) < 1 {
		log.Default().Fatalln("Config file must specify orgDirs parameter!", self.OrgDirs)
	}
	// You have to have a salt of some kind defined
	if self.OrgSalt == KBAD_SALT {
		fmt.Fprintf(os.Stderr, "B")
		log.Default().Printf("BAD SALT!\n>> You NEED to set orgSalt in your config file!\n")
	}
}

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*()"

func generateRandomString(length int) string {
	seededRand := mrand.New(mrand.NewSource(time.Now().UnixNano())) // Seed the random number generator
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}

func (self *ServerSettings) RandomKeyVal() string {
	//tHash := sha1.New()
	//tHash.Write([]byte(generateRandomString(32)))
	// tHash.Sum(nil)	// 32 bytes = 256 bits, recommended for HS256
	key := make([]byte, 32)
	_, err := rand.Read(key)
	if err != nil {
		log.Fatal(err)
	}
	return b64.StdEncoding.EncodeToString(key)
}

func (self *ServerSettings) Init() {
	// Really you should not use these and should provide your own!
	// But at least these are cryptographically sound as a time based
	// randomly generated string that we sha1 hash and base 64 encode
	// to produce something that should work as our keyset
	//privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	self.OrgJWS = self.RandomKeyVal()
	self.OrgJWE = self.RandomKeyVal()
	// The default keystore is useless, we need to force the user to make one of their own
	self.Keystore = ""
	self.OrgSalt = KBAD_SALT
	self.TokenExpiry = "1h"
	self.ServePath = "/org"
	self.Port = 8010
	self.TLSPort = 443
	self.ServerCrt = "server.crt"
	self.ServerKey = "server.key"
	self.AllowHttp = false
	self.AllowHttps = true
	self.TemplatePath = "./templates"
	self.DayPageTemplate = "daypage.tpl"
	self.DndSessionTemplate = "dndsession.tpl"
	self.DndSessionPath = "dndsessions"
	self.DayPagePath = "./daypages"
	self.DayPageMode = "week"
	self.DayPageModeWeekDay = "Monday"
	self.DayPageMaxSearchBack = 30 // How many weeks back should we look to pull last weeks tasks from.
	self.UseTagForProjects = true
	self.CaptureTemplates = []CaptureTemplate{}
	self.AccessControl = "null"
	self.RefileTargets = []string{".*\\.org"}
	gpu := true
	self.Babel = BabelSettings{
		// Off. Turning it on is a decision somebody has to make on purpose.
		Enable:    false,
		Languages: []string{},
		Timeout:   30,
		Commands:  map[string][]string{},
	}
	self.Voice = VoiceSettings{
		Models:   "",
		Port:     8081,
		Gpu:      &gpu,
		Url:      "",
		Model:    "",
		Language: "",
		Dir:      "audio",
		Timeout:  600,
		MaxMb:    64,
		Tags:     []string{"voice"},
	}
}

// What orgs may do about running a source block.
//
// Off unless it is turned on. Reading somebody's org files and executing the
// programs inside them are different promises, and the second one has to be
// made deliberately - the server may be reachable from anything on the
// network, and noAuth is a setting people use.
type BabelSettings struct {
	Enable bool `yaml:"enable"`
	// The languages that may be run. Empty means every one orgs knows how to
	// run; a list means only those, by the name written after #+BEGIN_SRC.
	Languages []string `yaml:"languages"`
	// Seconds one block may take before it is killed.
	Timeout int `yaml:"timeout"`
	// How to run a language, replacing what orgs would have used. The code is
	// handed to the command on standard input.
	Commands map[string][]string `yaml:"commands"`
}
