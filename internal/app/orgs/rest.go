//lint:file-ignore ST1006 allow the use of self
package orgs

import (
	b64 "encoding/base64"
	"errors"
	"encoding/json"
	"io"
	"log"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/assets"
	htmlexp "github.com/ihdavids/orgs/internal/app/orgs/plugs/html"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs/slides"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs/tangle"
	"github.com/ihdavids/orgs/internal/common"

	//"log"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/gorilla/mux"
	"math/rand"
	"time"
)

func RestApi(router *mux.Router) {
	router.Use(loggingMiddleware)

	// Public routes (no auth required)
	router.HandleFunc("/login", login).Methods("POST")
	// The salt a client hashes a password with before sending it. Public
	// because it is asked for by somebody who is not logged in yet.
	router.HandleFunc("/salt", salt).Methods("GET")
	// A one-use login link a logged in client made for a browser (ticket.go).
	router.HandleFunc("/login/ticket/{ticket}", RedeemLoginTicket).Methods("GET")
	// Where Spotify sends the browser back after signing in (dndmusic.go):
	// public, because it comes back on 127.0.0.1 without the server's
	// cookie; it takes only a sign in this server started.
	router.HandleFunc("/dnd/music/spotify/callback", RequestDndSpotifyCallback).Methods("GET")

	// Protected routes - all endpoints below require a valid JWS token
	api := router.PathPrefix("").Subrouter()
	if !Conf().Server.NoAuth {
		api.Use(authenticate)
	} else {
		// Still a middleware: the per-user endpoints read a username off every
		// request, and with nobody logged in they would all refuse.
		api.Use(noAuthUser)
		fmt.Fprintf(os.Stderr, "WARNING: Authentication is disabled (noAuth: true), everything is done as %q\n", NoAuthUsername)
	}

	// After anything that may have written a file, the database reads it
	// again before the answer goes back, so the next request sees it.
	api.Use(freshAfterWrite)

	api.HandleFunc("/refresh", refresh).Methods("POST")
	api.HandleFunc("/login/ticket", PostLoginTicket).Methods("POST") // a one-use login link for a browser (ticket.go)
	api.HandleFunc("/reloadconfig", PostReloadConfig).Methods("POST") // re-read the yaml: capture templates, filters, ... (reload.go)
	// Adding an account. Only an administrator may, and only when this server
	// has a keystore file to write - see adduser.go.
	api.HandleFunc("/users/add", PostAddUser).Methods("POST")
	api.HandleFunc("/editor/help", RequestEditorHelp).Methods("GET") // worg's file editor: keys and snippets, from docs/editor.org
	api.HandleFunc("/orgfile", PostOrgFile).Methods("POST") // the files tab's editor saving; before the GET, which takes any method
	api.HandleFunc("/orgfile", RequestOrgFile)
	api.HandleFunc("/findfile", RequestFindFileInDb)
	api.HandleFunc("/files", RequestFiles)
	api.HandleFunc("/file", CreateFile).Methods("POST")
	api.HandleFunc("/dirs", RequestDirs)
	api.HandleFunc("/newtemplates", RequestNewTemplates)
	api.HandleFunc("/file/{type}", RequestFile)                   // html etc
	api.HandleFunc("/pandoc", RequestPandoc).Methods("GET")
	api.HandleFunc("/book", RequestBook).Methods("GET") // export.go: a manuscript (docx) or an ebook (epub), written natively
	api.HandleFunc("/compile", PostCompile).Methods("POST")                 // writing.go: text the editor put together, exported
	api.HandleFunc("/writing/snapshots", RequestSnapshots).Methods("GET")  // writing.go: a file's snapshots
	api.HandleFunc("/writing/snapshots", PostSnapshot).Methods("POST")     // take or delete one
	api.HandleFunc("/writing/log", RequestWritingLog).Methods("GET")       // words per file per day
	api.HandleFunc("/writing/log", PostWritingLog).Methods("POST")
	api.HandleFunc("/media/resolve", PostMediaResolve).Methods("POST") // where a file's pictures can be fetched (inline images)
	api.HandleFunc("/languagetool/check", PostLanguageToolCheck).Methods("POST")         // writing mode's checker, passed to LanguageTool
	api.HandleFunc("/languagetool/status", RequestLanguageToolStatus).Methods("GET")     // the LanguageTool orgs runs
	api.HandleFunc("/languagetool/languages", RequestLanguageToolLanguages).Methods("GET")
	api.HandleFunc("/writing/goals", PostWritingGoals).Methods("POST") // daily, weekly, monthly word goals
	api.HandleFunc("/assistant/status", RequestAssistantStatus).Methods("GET") // the writing assistant's backends
	api.HandleFunc("/assistant/chat", PostAssistantChat).Methods("POST")
	api.HandleFunc("/assistant/key", PostAssistantKey).Methods("POST") // set a backend's key from worg
	api.HandleFunc("/harper/lint", PostHarperLint).Methods("POST")     // Harper run on the server, for phones
	api.HandleFunc("/harper/status", RequestHarperStatus).Methods("GET")
	api.HandleFunc("/filecontents/headings", RequestHeadings)     // Get all todos in file
	api.HandleFunc("/filters", RequestFilters)                    // Get all stored filters from the server
	api.HandleFunc("/html/themes", RequestHtmlThemes)             // The themes the html exporter can render with
	api.HandleFunc("/slides/themes", RequestSlideThemes)          // The themes the presentation exporters can render with
	api.HandleFunc("/exporters", RequestExporters).Methods("GET") // The exporters this server was configured with
	api.HandleFunc("/version", RequestVersion).Methods("GET")     // This server's version and whether a newer release is out (version.go)
	api.HandleFunc("/taggroups", RequestTagGroups)
	api.HandleFunc("/properties", RequestProperties).Methods("GET") // property names in use, for the query box (propnames.go)
	api.HandleFunc("/grep", RequestGrep)
	api.HandleFunc("/search", RequestTodosExpr)
	api.HandleFunc("/agenda", RequestAgenda).Methods("GET")            // what lands on each day (agenda.go)
	api.HandleFunc("/agenda/nodes", RequestAgendaNodes).Methods("GET") // a query's headings with their agenda facts
	api.HandleFunc("/lookuphash", RequestHash)
	api.HandleFunc("/todohtml/{hash}", RequestFullTodoHtml)
	api.HandleFunc("/logbook/{hash}", RequestLogbook)
	api.HandleFunc("/filehtml/{hash}", RequestFullFileHtml)
	api.HandleFunc("/todofull/{hash}", RequestFullTodo)
	api.HandleFunc("/hash/{hash}", RequestByHash)
	api.HandleFunc("/next/{hash}", RequestNextSibling)
	api.HandleFunc("/prev/{hash}", RequestPrevSibling)
	api.HandleFunc("/child/{hash}", RequestLastChild)
	api.HandleFunc("/id/{id}", RequestByAnyId)
	api.HandleFunc("/daypage/increment", RequestDayPageIncrement).Methods("GET")
	api.HandleFunc("/daypage/{date}", RequestDayPageAt).Methods("GET")
	api.HandleFunc("/daypage", PostCreateDayPage).Methods("POST")
	api.HandleFunc("/status/change", PostChangeStatus).Methods("POST")
	api.HandleFunc("/headline/change", PostRenameHeadline).Methods("POST")
	api.HandleFunc("/body/change", PostChangeBody).Methods("POST")
	api.HandleFunc("/body/{hash}", RequestTodoBody).Methods("GET")
	api.HandleFunc("/checklist", PostChecklistToggle).Methods("POST")
	api.HandleFunc("/addresses", RequestServerAddresses).Methods("GET")
	api.HandleFunc("/events", RequestEvents).Methods("GET")
	api.HandleFunc("/status", RequestDefaultStatus).Methods("GET")
	api.HandleFunc("/status/{hash}", RequestValidStatus)
	api.HandleFunc("/date/change", PostChangeDate).Methods("POST")
	api.HandleFunc("/date/change", DeleteDate).Methods("DELETE")
	api.HandleFunc("/property", PostChangeProperty).Methods("POST")
	api.HandleFunc("/alltags", RequestTags)
	api.HandleFunc("/tags", PostToggleTags).Methods("POST")
	api.HandleFunc("/heading/parts", PostHeadingParts).Methods("POST") // set tags and/or priority as a line edit
	api.HandleFunc("/capture", PostCapture).Methods("POST")
	api.HandleFunc("/capture/templates", RequestCaptureTemplates)
	api.HandleFunc("/voice/link", RequestVoiceLink).Methods("GET")
	api.HandleFunc("/delete", PostDelete).Methods("POST")
	api.HandleFunc("/refilefiles", RequestRefileTargets)
	api.HandleFunc("/refile", PostRefile).Methods("POST")
	api.HandleFunc("/archive", PostArchive).Methods("POST")
	api.HandleFunc("/move", PostMove).Methods("POST")                       // refile, copy or archive, one heading or many
	api.HandleFunc("/refile/targets", RequestRefileTargets2).Methods("GET") // where a heading can go, with hashes
	api.HandleFunc("/copy", PostCopy).Methods("POST")                       // a refile that leaves the original
	api.HandleFunc("/reformat", PostReformat).Methods("POST")
	api.HandleFunc("/reformat", RequestReformatCheck).Methods("GET")
	api.HandleFunc("/setexclusivemarker", PostMarker).Methods("POST")
	api.HandleFunc("/exclusivemarker", RequestMarker)
	api.HandleFunc("/update", PostUpdate).Methods("POST")
	api.HandleFunc("/clockin", PostClockIn).Methods("POST")
	api.HandleFunc("/clockout", PostClockOut).Methods("POST")
	api.HandleFunc("/clock", RequestClock)
	api.HandleFunc("/clockreport", RequestClockReport)
	api.HandleFunc("/timesheet", RequestTimesheet).Methods("GET") // timesheet.go: clocked time by client, csv/html invoices
	api.HandleFunc("/execb", PostExecb).Methods("POST")
	api.HandleFunc("/babel/exec", PostBabelExec).Methods("POST")
	api.HandleFunc("/dblocks", RequestDynBlocks).Methods("GET")
	api.HandleFunc("/dblocks/update", PostDynBlocksUpdate).Methods("POST")
	api.HandleFunc("/exectable", PostExect).Methods("POST")
	api.HandleFunc("/execalltables", PostExecAllT).Methods("POST")
	api.HandleFunc("/tableformulainfo", PostFormulaInfo).Methods("POST")
	api.HandleFunc("/tablerandomget", RequestTableRandomGet)
	api.HandleFunc("/tablenames", RequestTableNames)
	api.HandleFunc("/columns", RequestColumns).Methods("GET")       // org column view, with effort rolled up
	api.HandleFunc("/columns/spec", PostColumnSpec).Methods("POST") // write a file's #+COLUMNS: line
	api.HandleFunc("/columns/values", RequestColumnValues).Methods("GET") // every property in a file, with its values
	api.HandleFunc("/csv", RequestCsv).Methods("GET")                     // csvexport.go: a column view or a query as csv
	api.HandleFunc("/habits", RequestHabits).Methods("GET")
	api.HandleFunc("/habits/untick", PostHabitUntick).Methods("POST") // clear today's tick         // every habit and how it is going
	api.HandleFunc("/tables", RequestTables)
	api.HandleFunc("/table", RequestTable).Methods("GET")
	api.HandleFunc("/table", PostTable).Methods("POST")
	api.HandleFunc("/table/eval", PostTableEval).Methods("POST")
	api.HandleFunc("/table/calc", PostTableCalc).Methods("POST") // run a table sent as text, write nothing (tablecalc.go)
	api.HandleFunc("/table/import", PostTableImport).Methods("POST")
	api.HandleFunc("/tangle", RequestTangle)

	// Every source block in the database, read rather than merely located.
	api.HandleFunc("/code", RequestCode).Methods("GET")
	api.HandleFunc("/code/run", PostCodeRun).Methods("POST")
	api.HandleFunc("/code/update", PostCodeUpdate).Methods("POST")

	// The gantt chart: the schedule behind one, and adding a task to it
	api.HandleFunc("/gantt/tasks", RequestGanttTasks).Methods("GET")
	api.HandleFunc("/gantt/add", PostGanttAdd).Methods("POST")
	api.HandleFunc("/gantt/marker", PostGanttMarker).Methods("POST")
	// Suggestions for a chart set up from a pile of tickets, and writing a dependency (ganttsuggest.go, ganttlink.go)
	api.HandleFunc("/gantt/suggest", PostGanttSuggest).Methods("POST")
	api.HandleFunc("/gantt/link", PostGanttLink).Methods("POST")

	// A file's git history and the file at a revision, for worg's Diff tab (filehistory.go)
	api.HandleFunc("/history", RequestHistory).Methods("GET")
	api.HandleFunc("/history/text", RequestHistoryText).Methods("GET")

	// Records and collections: the contact book, and everything else somebody
	// wants to keep a list of.
	api.HandleFunc("/records", RequestRecords).Methods("GET")
	api.HandleFunc("/records/collections", RequestCollections).Methods("GET")
	api.HandleFunc("/records/fields", RequestRecordFields).Methods("GET")
	api.HandleFunc("/records/birthdays", RequestBirthdays).Methods("GET")
	api.HandleFunc("/records/collection", PostCollection).Methods("POST")
	api.HandleFunc("/records/collection/update", PostCollectionUpdate).Methods("POST")
	api.HandleFunc("/record", PostRecord).Methods("POST")
	api.HandleFunc("/record/update", PostRecordUpdate).Methods("POST")
	api.HandleFunc("/record/{hash}", RequestRecord).Methods("GET")
	// The contact book is records of type "contact"; these are the same
	// endpoints with the type filled in.
	api.HandleFunc("/contacts", RequestContacts).Methods("GET")
	api.HandleFunc("/contact", PostContact).Methods("POST")

	// Backlinks and the link graph
	api.HandleFunc("/links", RequestBacklinks).Methods("GET")
	api.HandleFunc("/links/graph", RequestLinkGraph).Methods("GET")
	api.HandleFunc("/links/stats", RequestLinkStats).Methods("GET")
	api.HandleFunc("/links/all", RequestAllLinks).Methods("GET") // every link, flat
	api.HandleFunc("/links/resolve", RequestResolveLink).Methods("GET")
	// Features with their routes in their own files.
	registerReviewRoutes(api)  // review_wizard.go: weekly review, inbox triage, someday sweep
	registerMentionRoutes(api) // mentions.go: heading backlinks, unlinked mentions
	registerFocusRoutes(api)   // focus.go: focus / pomodoro sessions
	registerPlannerRoutes(api)    // planner.go: time-blocking, the day planner
	registerInsightRoutes(api)    // insights.go: effort vs actual, dependency health
	registerQueryWatchRoutes(api) // querywatch.go: saved-query notifications
	registerRefactorRoutes(api)   // refactor.go: tag rename, split, merge, list to headings
	registerCryptRoutes(api)      // cryptui.go: revealing encrypted headings
	registerNotifyRoutes(api)     // notifydeliver.go: notification status and a test
	registerImportRoutes(api)  // ipynbimport.go: Jupyter notebooks into org files
	registerPlannerPrintRoutes(api) // plannerprint.go: GET /agenda/print, the agenda as a paper planner
	registerBeamerRoutes(api)       // beamerpdf.go: beamer decks as pdf, and the presets
	registerTypstRoutes(api)        // typstpdf.go: /typst/pdf, /typst/templates
	registerSiteRoutes(api)         // site.go: a folder of org files as a static web site
	registerRefreshRoutes(api)      // refresh.go: :schedule blocks, dynamic blocks and formulas kept current
	api.HandleFunc("/detangle", PostDetangle).Methods("POST") // detangle.go: a tangled file's edits back into its blocks
	registerBabelSessionRoutes(api) // babelsession.go: python :session list and stop
	registerForgeRoutes(api)   // forge.go: github / gitlab poll now, token check
	registerContactImportRoutes(api) // vcardimport.go: vCard files into the contact records

	// The database as a galaxy: every file or heading a star. Not the link
	// graph - that one only holds the things something links to, and a map has
	// to hold everything.
	api.HandleFunc("/starmap", RequestStarmap).Methods("GET")

	// Dungeons & Dragons character module
	api.HandleFunc("/dnd/rulesets", RequestDndRulesets).Methods("GET")
	api.HandleFunc("/dnd/reload", PostDndReload).Methods("POST")
	api.HandleFunc("/dnd/catalog", RequestDndCatalog).Methods("GET")
	api.HandleFunc("/dnd/characters", RequestDndCharacters).Methods("GET")
	api.HandleFunc("/dnd/books", RequestDndBooks).Methods("GET")      // files written as a dndbook
	api.HandleFunc("/files/themes", RequestFileThemes).Methods("GET") // files naming their own html theme
	api.HandleFunc("/files/search", RequestFileSearch).Methods("GET") // ripgrep over the text of every file
	api.HandleFunc("/pdf", RequestPdfView).Methods("GET")             // one file, run through pdflatex
	api.HandleFunc("/dnd/sheet", RequestDndSheet).Methods("GET")
	api.HandleFunc("/dnd/session", PostDndSession).Methods("POST")
	api.HandleFunc("/dnd/session/{id}", RequestDndSession).Methods("GET")
	api.HandleFunc("/dnd/session/{id}", DeleteDndSession).Methods("DELETE")
	api.HandleFunc("/dnd/answer", PostDndAnswer).Methods("POST")
	api.HandleFunc("/dnd/random", PostDndRandom).Methods("POST")
	api.HandleFunc("/dnd/generate", RequestDndGenerators).Methods("GET")       // the table's random generator (dndrandom.go)
	api.HandleFunc("/dnd/generate", PostDndGenerate).Methods("POST")
	api.HandleFunc("/dnd/generate/save", PostDndGenerateSave).Methods("POST")
	api.HandleFunc("/dnd/dungeon", RequestDndDungeonFields).Methods("GET") // random dungeons (dnddungeon.go)
	api.HandleFunc("/dnd/dungeon", PostDndDungeon).Methods("POST")
	api.HandleFunc("/dnd/dungeon/exits", PostDndDungeonExits).Methods("POST")
	api.HandleFunc("/dnd/dungeon/save", PostDndDungeonSave).Methods("POST")
	api.HandleFunc("/dnd/dungeon/saved", RequestDndDungeonsSaved).Methods("GET")
	api.HandleFunc("/dnd/dungeon/load", RequestDndDungeonLoad).Methods("GET")
	api.HandleFunc("/dnd/dungeon/state", PostDndDungeonState).Methods("POST")
	api.HandleFunc("/dnd/dungeon/image", RequestDndDungeonImage).Methods("GET")
	api.HandleFunc("/dnd/dungeon/pictures", RequestDndDungeonPictures).Methods("GET") // pictures to show the table (dndpictures.go)
	api.HandleFunc("/dnd/dungeon/pictures", PostDndDungeonPictures).Methods("POST")
	api.HandleFunc("/dnd/dungeon/pictures/edit", PostDndDungeonPictureEdit).Methods("POST")
	api.HandleFunc("/dnd/picture/fetch", PostDndPictureFetch).Methods("POST")
	api.HandleFunc("/dnd/picture/scrape", PostDndPictureScrape).Methods("POST") // every picture on a page (dndscrape.go)
	api.HandleFunc("/dnd/campaigns", RequestDndCampaigns).Methods("GET") // campaigns and their areas of maps (dndcampaign.go)
	api.HandleFunc("/dnd/journal", RequestDndJournal).Methods("GET")  // the DM's notes on a map (dndmapnotes.go)
	api.HandleFunc("/dnd/journal", PostDndJournal).Methods("POST")    // put notes on a map, or make new ones there
	api.HandleFunc("/dnd/campaign", PostDndCampaign).Methods("POST")
	api.HandleFunc("/dnd/maps", RequestDndMaps).Methods("GET")
	api.HandleFunc("/dnd/map/thumb", RequestDndMapThumb).Methods("GET")
	api.HandleFunc("/dnd/sounds", RequestDndSounds).Methods("GET") // the sound board (dndsounds.go)
	api.HandleFunc("/dnd/sounds/file", RequestDndSoundFile).Methods("GET")
	api.HandleFunc("/dnd/sounds/search", RequestDndSoundSearch).Methods("GET")
	api.HandleFunc("/dnd/sounds/cache", PostDndSoundCache).Methods("POST")
	api.HandleFunc("/dnd/music", RequestDndMusic).Methods("GET") // the music (dndmusic.go)
	api.HandleFunc("/dnd/music/file", RequestDndMusicFile).Methods("GET")
	api.HandleFunc("/dnd/music/spotify", RequestDndSpotify).Methods("GET")
	api.HandleFunc("/dnd/music/spotify/play", PostDndSpotifyPlay).Methods("POST")
	api.HandleFunc("/dnd/music/spotify/control", PostDndSpotifyControl).Methods("POST")
	api.HandleFunc("/dnd/music/spotify/login", RequestDndSpotifyLogin).Methods("GET")
	api.HandleFunc("/dnd/music/spotify/setup", PostDndSpotifySetup).Methods("POST")
	api.HandleFunc("/dnd/music/spotify/search", RequestDndSpotifySearch).Methods("GET")
	api.HandleFunc("/dnd/music/spotify/playlists", RequestDndSpotifyPlaylists).Methods("GET")
	api.HandleFunc("/dnd/dungeon/stock", PostDndDungeonStock).Methods("POST")
	api.HandleFunc("/dnd/dungeon/encounter", PostDndDungeonEncounter).Methods("POST")
	api.HandleFunc("/dnd/dungeon/lights", PostDndDungeonLights).Methods("POST")
	api.HandleFunc("/mapgen", RequestMapgenFields).Methods("GET") // generated maps (dndworld.go)
	api.HandleFunc("/mapgen/render", PostMapgenRender).Methods("POST")
	api.HandleFunc("/mapgen/names", RequestMapgenNames).Methods("GET")
	api.HandleFunc("/mapgen/settle", PostMapgenSettle).Methods("POST")
	api.HandleFunc("/mapgen/surf", PostMapgenSurf).Methods("POST")
	api.HandleFunc("/dnd/monsters", RequestDndMonsters).Methods("GET") // the initiative tracker (dndcombat.go)
	api.HandleFunc("/dnd/monster", RequestDndMonster).Methods("GET")
	api.HandleFunc("/dnd/party", RequestDndParty).Methods("GET")
	api.HandleFunc("/dnd/party", PostDndParty).Methods("POST")
	api.HandleFunc("/dnd/party/file", PostDndPartyFile).Methods("POST")
	api.HandleFunc("/dnd/save", PostDndSave).Methods("POST")
	api.HandleFunc("/dnd/levelup", PostDndLevelUp).Methods("POST")
	api.HandleFunc("/dnd/refresh", PostDndRefresh).Methods("POST")
	api.HandleFunc("/dnd/import", PostDndImport).Methods("POST")

	// Dungeons & Dragons inventory and spells
	api.HandleFunc("/dnd/items", RequestDndItems).Methods("GET")
	api.HandleFunc("/dnd/inventory", RequestDndInventory).Methods("GET")
	api.HandleFunc("/dnd/inventory", PostDndInventory).Methods("POST")
	api.HandleFunc("/dnd/undo", RequestDndUndo).Methods("GET")
	api.HandleFunc("/dnd/undo", PostDndUndo).Methods("POST")
	api.HandleFunc("/dnd/money", RequestDndMoney).Methods("GET")
	api.HandleFunc("/dnd/money", PostDndMoney).Methods("POST")
	api.HandleFunc("/dnd/spells", RequestDndSpells).Methods("GET")
	api.HandleFunc("/dnd/spellbook", RequestDndSpellbook).Methods("GET")
	api.HandleFunc("/dnd/spellbook", PostDndSpellbook).Methods("POST")
	api.HandleFunc("/dnd/slots", PostDndSlots).Methods("POST")
	api.HandleFunc("/dnd/rest", RequestDndRest).Methods("GET")
	api.HandleFunc("/dnd/rest", PostDndRest).Methods("POST")
	api.HandleFunc("/dnd/uses", PostDndUses).Methods("POST")
	api.HandleFunc("/dnd/conditions", RequestDndConditions).Methods("GET")
	api.HandleFunc("/dnd/conditions", PostDndConditions).Methods("POST")
	api.HandleFunc("/dnd/hp", RequestDndHealth).Methods("GET")
	api.HandleFunc("/dnd/hp", PostDndHealth).Methods("POST")
	api.HandleFunc("/dnd/inspiration", RequestDndInspiration).Methods("GET")
	api.HandleFunc("/dnd/inspiration", PostDndInspiration).Methods("POST")
	api.HandleFunc("/dnd/concentration", RequestDndConcentration).Methods("GET")
	api.HandleFunc("/dnd/concentration", PostDndConcentration).Methods("POST")

	// Dungeons & Dragons play session logs
	api.HandleFunc("/dnd/play/sessions", RequestDndPlaySessions).Methods("GET")
	api.HandleFunc("/dnd/play/search", RequestDndPlaySearch).Methods("GET")
	api.HandleFunc("/dnd/play/session", PostDndPlaySession).Methods("POST")
	api.HandleFunc("/dnd/play/session/{id}", RequestDndPlaySession).Methods("GET")
	api.HandleFunc("/dnd/play/session/{id}/roll", PostDndPlayRoll).Methods("POST")
	api.HandleFunc("/dnd/play/session/{id}/roll/{index}", PostDndPlayRollEdit).Methods("POST")
	api.HandleFunc("/dnd/play/session/{id}/roll/{index}", DeleteDndPlayRoll).Methods("DELETE")
	api.HandleFunc("/dnd/play/session/{id}/note", PostDndPlayNote).Methods("POST")
	api.HandleFunc("/dnd/play/session/{id}/note/{index}", PostDndPlayNoteEdit).Methods("POST")
	api.HandleFunc("/dnd/play/session/{id}/note/{index}", DeleteDndPlayNote).Methods("DELETE")
	api.HandleFunc("/dnd/play/session/{id}/summary", PostDndPlaySummary).Methods("POST")
	api.HandleFunc("/dnd/play/session/{id}/delete", PostDndPlayBlockDelete).Methods("POST")
	api.HandleFunc("/dnd/play/session/{id}/mark", PostDndPlayMark).Methods("POST")
	api.HandleFunc("/dnd/play/session/{id}/mark", DeleteDndPlayMark).Methods("DELETE")
	api.HandleFunc("/dnd/play/session/{id}/undo", RequestDndPlayUndo).Methods("GET")
	api.HandleFunc("/dnd/play/session/{id}/undo", PostDndPlayUndo).Methods("POST")

	// Per-user extensions: stored queries
	api.HandleFunc("/ext/queries", RequestStoredQueries).Methods("GET")
	api.HandleFunc("/ext/query", RequestStoredQuery).Methods("GET")
	api.HandleFunc("/ext/query", PostStoredQuery).Methods("POST")
	api.HandleFunc("/ext/query", DeleteStoredQuery).Methods("DELETE")

	// Per-user extensions: capture templates
	api.HandleFunc("/ext/capture/templates", RequestUserCaptureTemplates).Methods("GET")
	api.HandleFunc("/ext/capture/template", PostUserCaptureTemplate).Methods("POST")
	api.HandleFunc("/ext/capture/template", DeleteUserCaptureTemplate).Methods("DELETE")

	// Voice notes: record, transcribe through go-whisper, file as org
	api.HandleFunc("/voice/config", RequestVoiceConfig).Methods("GET")
	// org-crypt: a heading tagged :crypt: whose body is ciphertext on disk.
	api.HandleFunc("/crypt/config", RequestCryptConfig).Methods("GET")
	api.HandleFunc("/crypt/headings", RequestCryptHeadings).Methods("GET")
	api.HandleFunc("/crypt/encrypt", PostCryptEncrypt).Methods("POST")
	api.HandleFunc("/crypt/decrypt", PostCryptDecrypt).Methods("POST")
	api.HandleFunc("/crypt/update", PostCryptUpdate).Methods("POST")
	api.HandleFunc("/crypt/sweep", PostCryptSweep).Methods("POST")
	api.HandleFunc("/crypt/lock", PostCryptLock).Methods("POST")

	// Attachments: the files a heading owns. org-attach, so the folder is the
	// one emacs would use.
	api.HandleFunc("/attach", PostAttach).Methods("POST")
	api.HandleFunc("/attachments", RequestAttachments).Methods("GET")
	api.HandleFunc("/attachments/all", RequestAllAttachments).Methods("GET")
	api.HandleFunc("/attachment/{hash}/{name}", RequestAttachment).Methods("GET")
	api.HandleFunc("/attachment/{hash}/{name}", DeleteAttachment).Methods("DELETE")

	api.HandleFunc("/voice/recording", PostVoiceRecording).Methods("POST")
	api.HandleFunc("/voice/recordings", RequestVoiceRecordings).Methods("GET")
	api.HandleFunc("/voice/recording/{id}", RequestVoiceAudio).Methods("GET")
	api.HandleFunc("/voice/recording/{id}", DeleteVoiceRecording).Methods("DELETE")
	api.HandleFunc("/voice/transcribe", PostVoiceTranscribe).Methods("POST")
	api.HandleFunc("/voice/note", PostVoiceNote).Methods("POST")
	api.HandleFunc("/voice/append", PostVoiceAppend).Methods("POST")
	api.HandleFunc("/image/paste", PostImagePaste).Methods("POST")
	api.HandleFunc("/voice/whisper/restart", PostWhisperRestart).Methods("POST")

	// Per-user extensions: kanban boards
	api.HandleFunc("/ext/drill/decks", RequestDrillDecks).Methods("GET")
	api.HandleFunc("/ext/drill/decks", PostDrillDecks).Methods("POST")
	api.HandleFunc("/drill/cards", RequestDrillCards).Methods("GET")
	api.HandleFunc("/drill/review", PostDrillReview).Methods("POST")
	api.HandleFunc("/ext/gantt/views", RequestGanttViews).Methods("GET")
	api.HandleFunc("/ext/gantt/views", PostGanttViews).Methods("POST")
	api.HandleFunc("/ext/kanban/boards", RequestKanbanBoards).Methods("GET")
	api.HandleFunc("/ext/kanban/boards", PostKanbanBoards).Methods("POST")
	api.HandleFunc("/ext/kanban/board", PostKanbanBoard).Methods("POST")
	api.HandleFunc("/ext/kanban/board", DeleteKanbanBoard).Methods("DELETE")

}

type MiddlewareFunc func(http.Handler) http.Handler

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Do stuff here
		log.Println("RQST: ", r.RequestURI)
		// Call the next handler, which can be another middleware in the chain, or the final handler.
		next.ServeHTTP(w, r)
	})
}

func AccessControl(w *http.ResponseWriter) {
	(*w).Header().Set("Access-Control-Allow-Origin", "*")
}

// Extract a hash parameter from an incomming URL, they have to be base64 encoded
// as the hash could have slashes or other non URL friendly characters
func GetHash(vars map[string]string, name string) (string, error) {
	if h, err := b64.URLEncoding.DecodeString(vars[name]); err == nil {
		return string(h), err
	} else {
		return "", err
	}
}

/*
		SDOC: API

	  - GET /files — List All Org Files
	    Returns a JSON array of absolute file paths for every org file the server is currently tracking.
	    These are the files discovered in the configured =orgDirs= directories. The list updates as the
	    server's file watcher detects new or removed files.

	    *Method:* =GET=

	    *Parameters:* None.

	    *Response:* A JSON array of strings, each being an absolute path to an org file.
	    #+BEGIN_SRC json
	    ["/home/user/org/todo.org", "/home/user/org/notes.org"]
	    #+END_SRC
	    EDOC
*/
func RequestFiles(w http.ResponseWriter, r *http.Request) {
	//vars := mux.Vars(r)
	//key := vars["id"]
	res := GetDb().GetFiles()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

/*
		SDOC: API

	  - GET /findfile — Locate a File in the Database
	    Searches the server's in-memory file database for a file matching the given filename.
	    Useful for resolving a short or relative filename into the full absolute path that the
	    server knows about. If the file is not tracked by the server, an error is returned.

	    *Method:* =GET=

	    *Query Parameters:*
	    | Parameter  | Type   | Required | Description                                                         |
	    |------------+--------+----------+---------------------------------------------------------------------|
	    | =filename= | string | yes      | The filename to search for. Can be a basename or a partial path.    |

	    *Response:* A =ResultMsg= JSON object.

	  - On success: ={"status": true, "msg": "/absolute/path/to/file.org"}=

	  - On failure: ={"status": false, "msg": "error description"}=
	    EDOC
*/
func RequestFindFileInDb(w http.ResponseWriter, r *http.Request) {
	fname := r.URL.Query().Get("filename")
	if res, err := FindFileInDb(fname); err != nil {
		w.Header().Set("Content-Type", "application/json")
		msg := common.ResultMsg{Ok: false, Msg: err.Error()}
		json.NewEncoder(w).Encode(msg)
	} else {
		w.Header().Set("Content-Type", "application/json")
		msg := common.ResultMsg{Ok: true, Msg: res}
		json.NewEncoder(w).Encode(msg)
	}
}

/*
		SDOC: API

	  - GET /grep — Search Across All Org Files
	    Performs a regular-expression search across the raw text of every org file the server
	    is tracking. Results are returned as an array of strings, each containing the filename,
	    line number, and matched line separated by the chosen delimiter (default =:=).

	    *Method:* =GET=

	    *Query Parameters:*
	    | Parameter    | Type   | Required | Description                                                            |
	    |--------------+--------+----------+------------------------------------------------------------------------|
	    | =query=      | string | yes      | The regular expression to search for.                                  |
	    | =delimeter=  | string | no       | Separator between filename, line number, and content. Defaults to =:=. |

	    *Response:* A JSON array of match strings.
	    #+BEGIN_SRC json
	    ["todo.org:12:TODO Buy groceries", "notes.org:45:Meeting with team"]
	    #+END_SRC
	    On error, returns an empty array.
	    EDOC
*/
func RequestGrep(w http.ResponseWriter, r *http.Request) {
	qry := r.URL.Query().Get("query")
	del := r.URL.Query().Get("delimeter")
	if del == "" {
		del = ":"
	}
	w.Header().Set("Content-Type", "application/json")
	if res, err := Grep(qry, del); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		json.NewEncoder(w).Encode([]string{})
	} else {
		json.NewEncoder(w).Encode(res)
	}
}

/*
		SDOC: API

	  - GET /orgfile — Read Raw Org File Contents
	    Reads the raw text content of an org file from disk and returns it as a string.
	    This returns the file contents verbatim (not parsed), which is useful for editors
	    that need the original source text.

	    *Method:* =GET=

	    *Query Parameters:*
	    | Parameter  | Type   | Required | Description                                       |
	    |------------+--------+----------+---------------------------------------------------|
	    | =filename= | string | yes      | Absolute path to the org file to read.             |

	    *Response:* A =ResultMsg= JSON object.

	  - On success: ={"status": true, "msg": "...raw file contents..."}=

	  - On failure: ={"status": false, "msg": "error description"}=
	    EDOC
*/
func RequestOrgFile(w http.ResponseWriter, r *http.Request) {
	//vars := mux.Vars(r)
	filename := r.URL.Query().Get("filename")
	w.Header().Set("Content-Type", "application/json")
	if f, err := ioutil.ReadFile(filename); err == nil {
		msg := common.ResultMsg{Ok: true, Msg: string(f)}
		json.NewEncoder(w).Encode(msg)
	} else {
		res := fmt.Sprintf("[RequestOrgFile] error opening file: %v", err)
		log.Printf(res)
		msg := common.ResultMsg{Ok: false, Msg: res}
		json.NewEncoder(w).Encode(msg)
	}
}

/*
		SDOC: API

	  - GET /filecontents/headings — List All Headings in a File
	    Returns every heading (TODO item or otherwise) found in the specified org file.
	    The result is an array of =Todo= objects containing the headline text, status,
	    priority, tags, filename, line position, hash, and other metadata.

	    *Method:* =GET=

	    *Query Parameters:*
	    | Parameter  | Type   | Required | Description                                      |
	    |------------+--------+----------+--------------------------------------------------|
	    | =filename= | string | yes      | The filename (basename or path) of the org file.  |

	    *Response:* A JSON array of =Todo= objects.
	    EDOC
*/
func RequestHeadings(w http.ResponseWriter, r *http.Request) {
	//vars := mux.Vars(r)
	fname := r.URL.Query().Get("filename")
	res, _ := GetAllTodosInFile(fname)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

/*
		SDOC: API

	  - GET /taggroups — List Tag Groups
	    Returns the tag groups configured in the server's YAML config. Tag groups are named
	    sets of tags that can be referenced in queries using handlebars syntax (e.g. ={{ WORK }}).
	    A default set is seeded automatically unless =noInternalTagGroups= is set in config.

	    *Method:* =GET=

	    *Parameters:* None.

	    *Response:* A JSON object mapping group names to their tag expressions.
	    EDOC
*/
func RequestTagGroups(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Conf().TagGroups)
}

/*
		SDOC: API

	  - GET /filters — List Filters
	    Returns the named filters configured in the server's YAML config. Filters are reusable
	    query fragments that can be substituted into search expressions using handlebars syntax
	    (e.g. ={{ AllTasks }}=). A default set (=AllTasks=, =HomeTasks=, =WorkTasks=, etc.) is
	    seeded unless =noInternalFilters= is set.

	    *Method:* =GET=

	    *Parameters:* None.

	    *Response:* A JSON object mapping filter names to their query expressions.
	    EDOC
*/
func RequestFilters(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Conf().Filters)
}

/*
		SDOC: API

	  - GET /html/themes — List Html Export Themes
	    Returns the themes the html exporter can render a file with. A theme is a
	    =<name>_style.css= file in the =html_styles= folder under your template path,
	    optionally paired with an =html_<name>.tpl= template of its own.

	    Any of these names can be given to =/file/html= as =theme=<name>=, which
	    overrides the =#+HTML_THEME:= the file itself asks for. This is how the worg
	    files view renders every file in the theme the reader picked.

	    *Method:* =GET=

	    *Parameters:* None.

	    *Response:* ={"Ok": true, "Themes": ["dark", "default", "dnd", "docs"]}=
	    EDOC
*/
func RequestHtmlThemes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct {
		Ok     bool
		Themes []string
	}{Ok: true, Themes: htmlexp.HtmlThemes()})
}

/* SDOC: API
* GET /slides/themes — List Presentation Themes

	Returns the themes the four presentation exporters can render a deck with. A
	theme is a =slides_theme_<name>.css= file in your template folder, and the
	same file works in all four - reveal.js, impress.js, WebSlides and deck.js -
	because what it sets is a palette and a type scale rather than one library's
	class names.

	Any of these names can be given to =/file/revealjs= (or =impressjs=,
	=webslides=, =deckjs=) as =theme=<name>=, which overrides the
	=#+REVEAL_THEME:= the file itself asks for. A name that is *not* in this list
	is passed through to the framework, so reveal's own =dracula= and deck.js'
	=swiss= still work.

	*Method:* =GET=

	*Parameters:* None.

	*Response:* ={"Ok": true, "Themes": ["blueprint", "bloom", "graphite", ...]}=
	EDOC */

func RequestSlideThemes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct {
		Ok     bool
		Themes []string
	}{Ok: true, Themes: slides.ThemeNames()})
}

/*
	SDOC: API

* GET /exporters — What This Server Can Export To

	Answers with the names of the exporters configured under =server.exporters=,
	and of the ones that need no settings (=todotxt=, =ics=, =typst=, ...), which
	are there whether the yaml names them or not. Together they are exactly the
	={type}= values =/file/{type}= will accept.

	The list cannot be guessed from the outside: any other exporter that is
	compiled in but not named in the yaml is not there as far as a request is concerned, and
	the refusal for one that is missing looks the same as the refusal for one
	that does not exist. A client offering a format to pick from has to be able
	to ask.

	*Method:* =GET=

	*Response:*
	| Field  | Type  | Description                        |
	|--------+-------+------------------------------------|
	| =Ok=   | bool  | Always true                        |
	| =Names=| array | The exporter names, sorted         |
	EDOC
*/
func RequestExporters(w http.ResponseWriter, r *http.Request) {
	names := ExporterNames()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct {
		Ok    bool
		Names []string
	}{Ok: true, Names: names})
}

/*
		SDOC: API

	  - GET /file/{type} — Export a File via an Exporter Plugin
	    Runs the named exporter plugin against an org file and returns the exported result.
	    The ={type}= path segment selects the exporter (e.g. =html=, =latex=, =revealjs=,
	    =impressjs=, =gantt=, =mermaid=, =tangle=, etc.). The exporter must be enabled in the
	    server's config under =server.exporters=.

	    By default, the exported content is returned as a string in the response body.
	    When =local=t= is set, the exporter writes to the file specified by =filename=
	    instead and the response reports success/failure.

	    *Method:* =GET=

	    *Path Parameters:*
	    | Parameter | Type   | Description                                            |
	    |-----------+--------+--------------------------------------------------------|
	    | ={type}=  | string | Name of the exporter plugin to use (e.g. =html=).     |

	    *Query Parameters:*
	    | Parameter      | Type   | Required | Description                                                                        |
	    |----------------+--------+----------+------------------------------------------------------------------------------------|
	    | =filename=     | string | no       | Output filename when =local=t=. Ignored otherwise.                                 |
	    | =query=        | string | yes      | The org file to export (basename or path).                                         |
	    | =subtree=      | string | no       | Export only the heading with this hash and its subtree, titled by the heading.     |
	    | =local=        | string | no       | Set to =t= to write the result to =filename= on disk instead of returning it.     |
	    | =filelinks=    | string | no       | Set to =t= to include file-style links in the export.                              |
	    | =httpslinks=   | string | no       | Set to =t= to convert links to https-style links.                                  |
	    | =relto=        | string | no       | Write image and attachment links relative to this directory, for a page published beside its media. |
	    | =parent=       | string | no       | A parent property passed to the exporter (exporter-specific).                      |
	    | =printable=    | string | no       | Set to =t= for an exporter's printer friendly output where it has one.              |
	    | =theme=        | string | no       | Render with this theme instead of the one the file asks for. See =/html/themes=.   |
	    | =backdrop=     | string | no       | Pictures to wash out behind a d&d character sheet: files, folders or urls.          |
	    | =backdropCycle= | string | no      | How long each backdrop stays up ("8m", "4m-12m", "off"), 25 minutes at the most.   |
	    | =backdropOpacity= | string | no    | How strongly the backdrop shows through ("18%"), 14% by default.                   |

	    *Response:* A =ResultMsg= JSON object.

	  - When =local= is not set: ={"status": true, "msg": "...exported content..."}=

	  - When =local=t=: ={"status": true, "msg": "Success"}= or ={"status": false, "msg": "error"}=
	    EDOC
*/
func RequestFile(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	ptype := vars["type"]
	fname := r.URL.Query().Get("filename")
	query := r.URL.Query().Get("query")
	local := r.URL.Query().Get("local")
	filelinks := r.URL.Query().Get("filelinks")
	httpslinks := r.URL.Query().Get("httpslinks")
	props := map[string]string{}
	props["parent"] = r.URL.Query().Get("parent")
	props["printable"] = r.URL.Query().Get("printable")
	props["theme"] = r.URL.Query().Get("theme")
	props["to"] = r.URL.Query().Get("to")
	props["backdrop"] = r.URL.Query().Get("backdrop")
	props["backdropCycle"] = r.URL.Query().Get("backdropCycle")
	props["backdropOpacity"] = r.URL.Query().Get("backdropOpacity")
	// The presentation exporters: a theme, an impress.js layout and a deck.js
	// transition can be overridden per request. The org file says what it
	// wants (`#+REVEAL_THEME:` and friends) and these are how somebody looking
	// at a preview tries another one without editing the file. An empty value
	// changes nothing.
	props["layout"] = r.URL.Query().Get("layout")
	props["transition"] = r.URL.Query().Get("transition")
	opts := common.ExportToFile{Name: ptype, Filename: fname, Query: query, Opts: "", Props: props}
	if filelinks == "t" {
		opts.Opts += "filelinks;"
	}
	if httpslinks == "t" {
		opts.Opts += "httpslinks;"
	}
	if relto := r.URL.Query().Get("relto"); relto != "" {
		opts.Opts += "relto=" + url.QueryEscape(relto) + ";"
	}
	// subtree=<hash>: export that heading and what is under it, not the file.
	var from common.ODb = db
	if sub := r.URL.Query().Get("subtree"); sub != "" {
		view, err := subtreeView(db, query, sub)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: err.Error()})
			return
		}
		from = view
	}
	// The page is computed from the file as it would read with its dynamic
	// blocks refreshed and its formulas run (refresh.go) - in memory, so an
	// export never rewrites the file. A subtree export reads the file itself.
	if r.URL.Query().Get("subtree") == "" && r.URL.Query().Get("refresh") != "f" {
		from = exportView(from, query)
	}
	var res common.ResultMsg
	if local == "t" {
		res, _ = ExportToFile(from, &opts)
	} else {
		res, _ = ExportToString(from, &opts)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

/*
		SDOC: API

	  - GET /tangle — Tangle Source Blocks from an Org File
	    Extracts source code blocks from an org file following Org mode tangle conventions.
	    Source blocks with a =:tangle= header argument are collected, grouped by target file,
	    and their content is assembled. Noweb references (=<<name>>=) are expanded when
	    =:noweb yes= is set on a block.

	    By default, the assembled content is returned in the JSON response *without* writing
	    any files to disk. Set =write=t= to also write the tangled files.

	    *Method:* =GET=

	    *Query Parameters:*
	    | Parameter  | Type   | Required | Description                                                                  |
	    |------------+--------+----------+------------------------------------------------------------------------------|
	    | =filename= | string | yes      | The org file to tangle (basename or path).                                   |
	    | =write=    | string | no       | Set to =t= to write tangled output files to disk in addition to returning.   |

	    *Response:* A JSON object containing an array of tangled files, each with the assembled content:
	    #+BEGIN_SRC json
	    {
	    "files": [
	    {
	    "filename": "/path/to/output.py",
	    "content": "#!/usr/bin/env python\nprint('hello')\n",
	    "lang": "python",
	    "lines": 2
	    }
	    ]
	    }
	    #+END_SRC

	    *Supported Block Header Arguments:*
	    | Header Arg     | Description                                                                    |
	    |----------------+--------------------------------------------------------------------------------|
	    | =:tangle=      | Output file path (relative to the org file directory, or absolute); =yes= for the org file's name with the language's extension; =no= to skip. |
	    | =:noweb=       | =yes=, =tangle=, =no-export= or =strip-export= expand =<<name>>= references; =strip-tangle= removes them. |
	    | =:noweb-ref=   | Defines this block as a reusable fragment that others can reference.            |
	    | =:noweb-sep=   | Separator when concatenating multiple blocks with the same =:noweb-ref=.       |
	    | =:mkdirp=      | Set to =yes= to create parent directories if they do not exist.                |
	    | =:padline=     | Set to =no= to suppress the blank line inserted between blocks.                |
	    | =:shebang=     | A shebang line (e.g. =#!/bin/bash=) prepended to the output file.              |
	    | =:tangle-mode= | File permissions, e.g. =(identity #o755)= or =0644=.                           |
	    | =:comments=    | =link= writes org's =[[file:x.org::*Heading][Heading:N]]= / =Heading:N ends here= markers around each block (what =orgs detangle= reads), =org= the text before the block as comments, =both= both. |

	    Header arguments are inherited as org inherits them: =#+PROPERTY: header-args=,
	    then =header-args:<lang>=, then each heading's =header-args= and
	    =header-args:<lang>= properties from the outermost in, then the block's own.

	    *Errors:*

	  - =400= if =filename= is missing.

	  - =500= if the file is not found or tangling fails.
	    EDOC
*/
func RequestTangle(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("filename")
	if query == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "filename parameter required"})
		return
	}
	writeToDisk := r.URL.Query().Get("write") == "t"
	result, err := tangle.Tangle(db, query, writeToDisk)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

/*
		SDOC: API

	  - GET /refilefiles — List Valid Refile Targets
	    Returns a list of headings across all org files that are valid targets for refiling.
	    The set of files considered is controlled by the =refileTargets= config setting,
	    which accepts a list of filename regex patterns (defaults to =.*\\.org=).

	    *Method:* =GET=

	    *Parameters:* None.

	    *Response:* A JSON array of refile target objects, each describing a heading that
	    can receive refiled content.
	    EDOC
*/
func RequestRefileTargets(w http.ResponseWriter, r *http.Request) {
	//vars := mux.Vars(r)
	//ptype := vars["type"]
	//fname := r.URL.Query().Get("filename")
	//query := r.URL.Query().Get("query")
	//local := r.URL.Query().Get("local")
	targets := GetRefileTargetsList([]string{})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(targets)
}

/*
		SDOC: API

	  - GET /filehtml/{hash} — Render Full File as HTML
	    Given the base64-URL-encoded hash of any heading in a file, renders the entire
	    containing org file as HTML and returns it. Useful for previewing a full document
	    when you only have a reference to one of its headings.

	    *Method:* =GET=

	    *Path Parameters:*
	    | Parameter | Type   | Description                                                          |
	    |-----------+--------+----------------------------------------------------------------------|
	    | ={hash}=  | string | Base64-URL-encoded hash of a heading in the target file.             |

	    *Response:* The rendered HTML string, or an error object on failure.
	    EDOC
*/
func RequestFullFileHtml(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	if h, err := GetHash(vars, "hash"); err == nil {
		var hash common.TodoHash = common.TodoHash(h)
		reply, err := QueryFullFileHtml(&hash)
		if err == nil {
			json.NewEncoder(w).Encode(reply)
		} else {
			answerErr(w, err)
		}
	} else {
		answerErr(w, err)
	}
}

/*
		SDOC: API

	  - GET /todohtml/{hash} — Render a Single Heading as HTML
	    Renders the heading identified by the given hash as HTML. Only the subtree
	    rooted at that heading is rendered, not the full file.

	    *Method:* =GET=

	    *Path Parameters:*
	    | Parameter | Type   | Description                                              |
	    |-----------+--------+----------------------------------------------------------|
	    | ={hash}=  | string | Base64-URL-encoded hash of the heading to render.        |

	    *Query Parameters:*
	    | Parameter | Type   | Required | Description                                                                 |
	    |-----------+--------+----------+-----------------------------------------------------------------------------|
	    | =theme=   | string | no       | Render through the html exporter with this theme and return its stylesheet.  |

	    Without =theme= the heading is rendered by go-org's own writer and =style= is
	    empty - a bare fragment. With one, the heading goes through the html
	    exporter instead, so pictures and recordings resolve to urls this server
	    serves, and =style= carries the theme's css for a client that has no
	    document to hang a =<head>= on. See =/html/themes= for the names.

	    *Response:* A =FullTodo= with the rendered html in =content= (and =style= when
	    a theme was asked for), or an error object on failure.
	    EDOC
*/
func RequestFullTodoHtml(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	if h, err := GetHash(vars, "hash"); err == nil {
		var hash common.TodoHash = common.TodoHash(h)
		reply, err := QueryFullTodoHtmlThemed(&hash, r.URL.Query().Get("theme"))
		if err == nil {
			json.NewEncoder(w).Encode(reply)
		} else {
			answerErr(w, err)
		}
	} else {
		answerErr(w, err)
	}
}

/*
		SDOC: API

	  - GET /todofull/{hash} — Get Full Heading Data
	    Returns the complete =Todo= data for the heading identified by the given hash.
	    This includes the headline text, status, priority, tags, properties, scheduling
	    timestamps, filename, position, and all other metadata the server tracks.

	    *Method:* =GET=

	    *Path Parameters:*
	    | Parameter | Type   | Description                                              |
	    |-----------+--------+----------------------------------------------------------|
	    | ={hash}=  | string | Base64-URL-encoded hash of the heading.                  |

	    *Response:* A full =Todo= JSON object, or an error on failure.
	    EDOC
*/
func RequestFullTodo(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	if h, err := GetHash(vars, "hash"); err == nil {
		var hash common.TodoHash = common.TodoHash(string(h))
		reply, err := QueryFullTodo(&hash)
		if err == nil {
			json.NewEncoder(w).Encode(reply)
		} else {
			fmt.Fprintln(os.Stderr, "ERROR during request", err)
			answerErr(w, err)
		}
	} else {
		fmt.Fprintln(os.Stderr, "ERROR getting hash value", err)
		answerErr(w, err)
	}
}

/*
		SDOC: API

	  - POST /file — Create a New Org File
	    Creates a new org file on disk, optionally from a template, and adds it to the server's
	    file database. If no title is given, the filename (without extension) is used as the title.
	    Templates are looked up from the =new/= subdirectory of the configured =templatePath=.

	    *Method:* =POST=

	    *Request Body (JSON):*
	    | Field      | Type   | Required | Description                                                      |
	    |------------+--------+----------+------------------------------------------------------------------|
	    | =filename= | string | yes      | Absolute path where the new org file should be created.           |
	    | =title=    | string | no       | Title for the file. Defaults to the basename without extension.   |
	    | =template= | string | no       | Template name (e.g. =new/journal.tpl=) to use for initial content.|
	    | =mustBeNew= | bool  | no       | Fail (=409=) if the file already exists, rather than reloading it.|

	    The file must end in =.org= and sit inside one of the configured =orgDirs=;
	    folders on the way to it are created. A template must be one of =/newtemplates=.

	    *Response:*

	  - On success (=200=): A JSON array containing the new file's absolute path.

	  - On failure (=400= / =500=): A =ResultMsg= with =status: false=.
	    EDOC
*/
func CreateFile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	body, _ := io.ReadAll(r.Body)
	var req common.NewFileRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: fmt.Sprintf("Invalid request: %s", err)})
		return
	}
	if req.Filename == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: "Filename is required"})
		return
	}
	// Only org files, only under the org directories: this endpoint writes
	// to disk, and a path from a request must not reach anywhere else.
	fname, ok := insideOrgDirs(req.Filename)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: "the file has to be inside one of the org directories"})
		return
	}
	if filepath.Ext(fname) != ".org" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: "the file has to end in .org"})
		return
	}
	req.Filename = fname
	if req.Template != "" && (strings.Contains(req.Template, "..") || !strings.HasPrefix(req.Template, "new/") || !strings.HasSuffix(req.Template, ".tpl")) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: "unknown template " + req.Template})
		return
	}
	if req.MustBeNew {
		if _, err := os.Stat(fname); err == nil {
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: fname + " already exists"})
			return
		}
	}
	title := req.Title
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(req.Filename), filepath.Ext(req.Filename))
	}
	file := GetDb().CreateOrgFileFromTemplate(req.Filename, title, req.Template)
	if file != nil {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(common.FileList{file.Filename})
	} else {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: "Failed to create file"})
	}
}

/*
		SDOC: API

	  - GET /dirs — List All Known Directories
	    Returns a deduplicated list of directories that contain org files. This includes
	    the configured =orgDirs= plus the parent directory of every individual file the
	    server is tracking. Useful for file-browser UIs that need to know where org content lives.

	    *Method:* =GET=

	    *Parameters:* =all= (optional): any value lists every folder under the =orgDirs=,
	    empty ones included (hidden folders and =node_modules= left out).

	    *Response:* A JSON array of absolute directory paths.
	    EDOC
*/
func RequestDirs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	dirSet := map[string]bool{}
	// Every folder under the org directories, empty ones too - what a "put the
	// new file here" picker wants. Hidden folders and node_modules are skipped.
	if r.URL.Query().Get("all") != "" {
		for _, root := range Conf().Server.OrgDirs {
			filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
				if err != nil || !d.IsDir() {
					return nil
				}
				name := d.Name()
				if p != root && (strings.HasPrefix(name, ".") || name == "node_modules") {
					return filepath.SkipDir
				}
				if len(dirSet) >= 20000 {
					return filepath.SkipAll
				}
				dirSet[p] = true
				return nil
			})
		}
	}
	files := GetDb().GetFiles()
	for _, f := range files {
		dirSet[filepath.Dir(f)] = true
	}
	for _, d := range Conf().Server.OrgDirs {
		dirSet[d] = true
	}
	dirs := make([]string, 0, len(dirSet))
	for d := range dirSet {
		dirs = append(dirs, d)
	}
	json.NewEncoder(w).Encode(dirs)
}

/*
		SDOC: API

	  - GET /newtemplates — List New-File Templates
	    Returns the list of available templates that can be used when creating a new org file
	    via =POST /file=. Templates are =.tpl= files found in the =new/= subdirectory of the
	    configured =templatePath=.

	    *Method:* =GET=

	    *Parameters:* None.

	    *Response:* A JSON array of template names (e.g. =["new/journal.tpl", "new/project.tpl"]=).
	    Returns an empty array if no templates are found.
	    EDOC
*/
func RequestNewTemplates(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	// The built in ones and any added to the template folder, each name once.
	templates := assets.GlobTemplates("new/*.tpl", Conf().Server.TemplatePath)
	json.NewEncoder(w).Encode(templates)
}

/*
		SDOC: API

	  - POST /status/change — Change Heading Status
	    Changes the TODO/DONE status keyword of a heading identified by its hash.
	    The new status value must be a valid keyword for the file (as defined by the
	    file's =#+TODO= line or the server's =defaultTodoStates= / =defaultNextStates=).

	    *Method:* =POST=

	    *Request Body (JSON):*
	    | Field   | Type   | Required | Description                                                  |
	    |---------+--------+----------+--------------------------------------------------------------|
	    | =Hash=  | string | yes      | The dynamic hash identifying the heading.                    |
	    | =Value= | string | yes      | The new status keyword (e.g. =DONE=, =TODO=, =NEXT=, =""=). |

	    *Response:* A =Result= JSON object with ={"status": true}= on success.
	    EDOC
*/
func PostChangeStatus(w http.ResponseWriter, r *http.Request) {
	body, _ := ioutil.ReadAll(r.Body)
	var args common.TodoItemChange
	var err = json.Unmarshal(body, &args)
	if err == nil {
		var reply common.Result
		reply, err = ChangeStatus(&args)
		if err == nil {
			json.NewEncoder(w).Encode(reply)
		} else {
			answerErr(w, err)
		}
	} else {
		answerErr(w, err)
	}
}

/*
		SDOC: API

	  - POST /headline/change — Rename a Heading
	    Changes the headline text of a heading identified by its hash. The new text
	    replaces the headline title (the part after the status keyword and priority).

	    *Method:* =POST=

	    *Request Body (JSON):*
	    | Field   | Type   | Required | Description                              |
	    |---------+--------+----------+------------------------------------------|
	    | =Hash=  | string | yes      | The dynamic hash identifying the heading.|
	    | =Value= | string | yes      | The new headline text.                   |

	    *Response:* A =Result= JSON object with ={"status": true}= on success.
	    EDOC
*/
func PostRenameHeadline(w http.ResponseWriter, r *http.Request) {
	body, _ := ioutil.ReadAll(r.Body)
	var args common.TodoItemChange
	var err = json.Unmarshal(body, &args)
	if err == nil {
		var reply common.Result
		reply, err = RenameHeadline(&args)
		if err == nil {
			json.NewEncoder(w).Encode(reply)
		} else {
			answerErr(w, err)
		}
	} else {
		answerErr(w, err)
	}
}

/*
		SDOC: API

	  - POST /body/change — Replace Heading Body Content
	    Replaces the body content (everything below the headline, before the next heading)
	    of a heading identified by its hash. The value should be raw org-mode text.

	    *Method:* =POST=

	    *Request Body (JSON):*
	    | Field   | Type   | Required | Description                                     |
	    |---------+--------+----------+-------------------------------------------------|
	    | =Hash=  | string | yes      | The dynamic hash identifying the heading.        |
	    | =Value= | string | yes      | The new body content (raw org-mode text).        |

	    *Response:* A =Result= JSON object with ={"status": true}= on success.
	    EDOC
*/
func PostChangeBody(w http.ResponseWriter, r *http.Request) {
	body, _ := ioutil.ReadAll(r.Body)
	var args common.TodoItemChange
	var err = json.Unmarshal(body, &args)
	if err == nil {
		var reply common.Result
		reply, err = ChangeBody(&args)
		if err == nil {
			json.NewEncoder(w).Encode(reply)
		} else {
			answerErr(w, err)
		}
	} else {
		answerErr(w, err)
	}
}

/*
		SDOC: API

	  - POST /date/change — Set or Change a Date on a Heading
	    Sets or updates a scheduling timestamp on a heading. Supports =SCHEDULED=, =DEADLINE=,
	    =CLOSED=, and =TIMESTAMP= date types. Pass an empty =Value= to clear the date.

	    *Method:* =POST=

	    *Request Body (JSON):*
	    | Field   | Type   | Required | Description                                                                   |
	    |---------+--------+----------+-------------------------------------------------------------------------------|
	    | =Hash=  | string | yes      | The dynamic hash identifying the heading.                                     |
	    | =Name=  | string | yes      | The date type: =SCHEDULED=, =DEADLINE=, =CLOSED=, or =TIMESTAMP=.            |
	    | =Value= | string | yes      | Org date string (e.g. =<2024-01-15 Mon>=) or =""= to clear.                  |

	    *Response:* A =Result= JSON object with ={"status": true}= on success.
	    EDOC
*/
func PostChangeDate(w http.ResponseWriter, r *http.Request) {
	body, _ := ioutil.ReadAll(r.Body)
	var args common.TodoDateChange
	var err = json.Unmarshal(body, &args)
	if err == nil {
		var reply common.Result
		reply, err = ChangeDate(&args)
		if err == nil {
			json.NewEncoder(w).Encode(reply)
		} else {
			json.NewEncoder(w).Encode(common.Result{Ok: false, Msg: err.Error()})
		}
	} else {
		json.NewEncoder(w).Encode(common.Result{Ok: false, Msg: err.Error()})
	}
}

/*
		SDOC: API

	  - DELETE /date/change — Remove a Date from a Heading
	    Removes a scheduling timestamp from a heading. The =Value= field in the request
	    body is ignored; the date identified by =Name= is unconditionally cleared.

	    *Method:* =DELETE=

	    *Request Body (JSON):*
	    | Field   | Type   | Required | Description                                                          |
	    |---------+--------+----------+----------------------------------------------------------------------|
	    | =Hash=  | string | yes      | The dynamic hash identifying the heading.                            |
	    | =Name=  | string | yes      | The date type to remove: =SCHEDULED=, =DEADLINE=, =CLOSED=, or =TIMESTAMP=. |

	    *Response:* A =Result= JSON object with ={"status": true}= on success.
	    EDOC
*/
func DeleteDate(w http.ResponseWriter, r *http.Request) {
	body, _ := ioutil.ReadAll(r.Body)
	var args common.TodoDateChange
	var err = json.Unmarshal(body, &args)
	if err == nil {
		args.Value = ""
		var reply common.Result
		reply, err = ChangeDate(&args)
		if err == nil {
			json.NewEncoder(w).Encode(reply)
		} else {
			json.NewEncoder(w).Encode(common.Result{Ok: false, Msg: err.Error()})
		}
	} else {
		json.NewEncoder(w).Encode(common.Result{Ok: false, Msg: err.Error()})
	}
}

/*
		SDOC: API

	  - POST /property — Set or Change a Property on a Heading
	    Sets or updates a property in the property drawer of a heading. If the heading
	    does not have a property drawer, one is created. Common properties include
	    =EFFORT=, =CUSTOM_ID=, =CATEGORY=, etc., but any key/value pair is accepted.

	    *Method:* =POST=

	    *Request Body (JSON):*
	    | Field   | Type   | Required | Description                                     |
	    |---------+--------+----------+-------------------------------------------------|
	    | =Hash=  | string | yes      | The dynamic hash identifying the heading.        |
	    | =Name=  | string | yes      | The property key (e.g. =EFFORT=, =CUSTOM_ID=).  |
	    | =Value= | string | yes      | The property value.                              |

	    *Response:* A =Result= JSON object with ={"status": true}= on success.
	    EDOC
*/
func PostChangeProperty(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(os.Stderr, "PostChangeProperty")
	body, _ := ioutil.ReadAll(r.Body)
	var args common.TodoPropertyChange
	var err = json.Unmarshal(body, &args)
	if err == nil {
		fmt.Fprintln(os.Stderr, "Deserialized")
		var reply common.Result
		reply, err = ChangeProperty(&args)
		if err == nil {
			json.NewEncoder(w).Encode(reply)
		} else {
			answerErr(w, err)
		}
	} else {
		fmt.Fprintln(os.Stderr, "Failed to deserialize")
		answerErr(w, err)
	}
}

/*
		SDOC: API

	  - POST /tags — Toggle Tags on a Heading
	    Toggles a tag on a heading identified by its hash. If the tag is present, it is
	    removed; if absent, it is added. The =Value= field contains the tag name to toggle
	    (without the surrounding colons).

	    *Method:* =POST=

	    *Request Body (JSON):*
	    | Field   | Type   | Required | Description                                     |
	    |---------+--------+----------+-------------------------------------------------|
	    | =Hash=  | string | yes      | The dynamic hash identifying the heading.        |
	    | =Value= | string | yes      | The tag name to toggle (e.g. =WORK=, =urgent=). |

	    *Response:* A =Result= JSON object with ={"status": true}= on success.
	    EDOC
*/
func PostToggleTags(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(os.Stderr, "PostToggleTags")
	body, _ := ioutil.ReadAll(r.Body)
	var args common.TodoItemChange
	var err = json.Unmarshal(body, &args)
	if err == nil {
		fmt.Fprintln(os.Stderr, "Deserialized")
		var reply common.Result
		reply, err = ToggleTag(&args)
		if err == nil {
			json.NewEncoder(w).Encode(reply)
		} else {
			answerErr(w, err)
		}
	} else {
		fmt.Fprintln(os.Stderr, "Failed to deserialize")
		answerErr(w, err)
	}
}

/* SDOC: API
* POST /reformat — Reformat Org Files
	Re-serializes and re-saves the specified org files, normalizing whitespace,
	indentation, and heading structure. This is a lossless rewrite that ensures
	the files conform to the parser's canonical output format.

	*Method:* =POST=

	*Request Body (JSON):* A JSON array of absolute file paths to reformat.
	#+BEGIN_SRC json
	["/home/user/org/todo.org", "/home/user/org/notes.org"]
	#+END_SRC

	*Response:* A =Result= JSON object with ={"status": true}= on success.
	EDOC */
/* SDOC: API
* GET /reformat — What Reformatting Would Change
	Answers with the text the org writer would produce for a file, and whether
	that is already what is on disk - without writing anything. The POST of the
	same path does the write; this is the same question asked without doing it,
	which is what a pre-commit hook or a CI step needs.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter  | Type   | Required | Description                                          |
	|------------+--------+----------+------------------------------------------------------|
	| =filename= | string | yes      | The org file to check.                               |
	| =text=     | string | no       | Set to =f= to leave the formatted text out of the answer. |

	*Response:* A =ReformatCheck= JSON object.
	#+BEGIN_SRC json
	{"Ok": true, "Formatted": false, "Text": "* TODO Something\n"}
	#+END_SRC
EDOC */
func RequestReformatCheck(w http.ResponseWriter, r *http.Request) {
	filename := r.URL.Query().Get("filename")
	w.Header().Set("Content-Type", "application/json")
	res := common.ReformatCheck{}
	if filename == "" {
		res.Msg = "missing filename parameter"
		json.NewEncoder(w).Encode(res)
		return
	}
	f := GetDb().FindByFile(filename)
	if f == nil || f.Doc == nil {
		res.Msg = fmt.Sprintf("no org file called %s", filename)
		json.NewEncoder(w).Encode(res)
		return
	}
	// Exactly what WriteOutOrgFile would put on disk, built the same way and
	// not written. Any other way of producing it would be a second answer to
	// drift from the first.
	ow := org.NewOrgWriter()
	f.Doc.Write(ow)
	want := ow.String()

	have, err := ioutil.ReadFile(f.Filename)
	if err != nil {
		res.Msg = err.Error()
		json.NewEncoder(w).Encode(res)
		return
	}
	res.Ok = true
	// Compared without the trailing newline, because that is the one difference
	// between a file somebody saved and the writer's output that nobody means
	// anything by.
	res.Formatted = strings.TrimRight(string(have), "\n") == strings.TrimRight(want, "\n")
	if r.URL.Query().Get("text") != "f" {
		res.Text = want
	}
	json.NewEncoder(w).Encode(res)
}

func PostReformat(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(os.Stderr, "PostReformat")
	body, _ := ioutil.ReadAll(r.Body)
	var args common.FileList
	var err = json.Unmarshal(body, &args)
	if err == nil {
		fmt.Fprintln(os.Stderr, "Deserialized")
		var reply common.Result
		reply, err = Reformat(&args)
		if err == nil {
			json.NewEncoder(w).Encode(reply)
		} else {
			answerErr(w, err)
		}
	} else {
		fmt.Fprintln(os.Stderr, "Failed to deserialize")
		answerErr(w, err)
	}
}

/*
		SDOC: API

	  - GET /search — Search Headings by Query Expression
	    Searches all tracked org files for headings matching the given query expression.
	    The query language supports TODO status filtering, tag matching, property comparisons,
	    and boolean logic. Filters and tag groups defined in config can be referenced with
	    handlebars syntax (e.g. ={{ AllTasks }}=).

	    *Method:* =GET=

	    *Query Parameters:*
	    | Parameter | Type   | Required | Description                                              |
	    |-----------+--------+----------+----------------------------------------------------------|
	    | =query=   | string | yes      | The search expression (e.g. =TODO="TODO"+HOME=).         |

	    *Response:* A JSON array of =Todo= objects matching the query. Returns an error
	    object on failure.
	    EDOC
*/
func RequestTodosExpr(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	var args common.StringQuery
	args.Query = query
	reply, err := QueryStringTodos(&args)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		// Was Encode(err), which for govaluate's errors is `{}` - an object
		// that said something went wrong and nothing about what. Still an
		// object, so a client that checks for a list behaves as before.
		json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	// No matches is `[]`, not `null`: a nil list encodes as null, and worg
	// read that as "not a list" and reported a parse error for a query that
	// had simply found nothing.
	if reply == nil || *reply == nil {
		w.Write([]byte("[]\n"))
		return
	}
	json.NewEncoder(w).Encode(reply)
}

/*
		SDOC: API

	  - GET /lookuphash — Look Up Hash by File Position
	    Given a filename and a line number (row), returns the hash of the heading that
	    contains that position. This is useful for editors that know a cursor position
	    and need to map it to the server's internal hash identifier.

	    *Method:* =GET=

	    *Query Parameters:*
	    | Parameter  | Type   | Required | Description                                       |
	    |------------+--------+----------+---------------------------------------------------|
	    | =filename= | string | yes      | The org filename (basename or path).               |
	    | =pos=      | int    | yes      | The 1-based line number in the file.               |

	    *Response:* A =Todo= JSON object for the heading at that position, or an error string.
	    EDOC
*/
func RequestHash(w http.ResponseWriter, r *http.Request) {
	strPos := r.URL.Query().Get("pos")
	fname := r.URL.Query().Get("filename")
	pos, serr := strconv.Atoi(strPos)
	w.Header().Set("Content-Type", "application/json")
	if serr != nil {
		json.NewEncoder(w).Encode("Failed to convert position value")
		return
	}
	reply, err := FindNodeInFile(pos, fname)
	if err == nil {
		json.NewEncoder(w).Encode(reply)
	} else {
		answerErr(w, err)
	}
}

/*
		SDOC: API

	  - GET /hash/{hash} — Get Heading by Hash
	    Retrieves the =Todo= data for a heading by its dynamic hash. The hash must be
	    base64-URL-encoded in the URL path because it may contain characters that are
	    not URL-safe (such as =/=).

	    *Method:* =GET=

	    *Path Parameters:*
	    | Parameter | Type   | Description                                              |
	    |-----------+--------+----------------------------------------------------------|
	    | ={hash}=  | string | Base64-URL-encoded dynamic hash of the heading.          |

	    *Response:* A =Todo= JSON object, or an error string if not found.
	    EDOC
*/
func RequestByHash(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	w.Header().Set("Content-Type", "application/json")
	if h, err := GetHash(vars, "hash"); err == nil {
		var hash common.TodoHash = common.TodoHash(h)
		var err error = nil
		res := FindByHash(&hash)
		if res == nil || err != nil {
			if err == nil {
				err = fmt.Errorf("")
			}
			json.NewEncoder(w).Encode(fmt.Sprintf("could not find hash %s %s", hash, err))
		} else {
			json.NewEncoder(w).Encode(res)
		}
	} else {
		answerErr(w, err)
	}
}

/*
		SDOC: API

	  - GET /id/{id} — Get Heading by ID or CUSTOM_ID
	    Retrieves the =Todo= data for a heading by its =ID= or =CUSTOM_ID= property.
	    The server searches all tracked files for a heading whose property drawer
	    contains a matching =ID= or =CUSTOM_ID= value.

	    *Method:* =GET=

	    *Path Parameters:*
	    | Parameter | Type   | Description                                              |
	    |-----------+--------+----------------------------------------------------------|
	    | ={id}=    | string | The =ID= or =CUSTOM_ID= value to search for.            |

	    *Response:* A =Todo= JSON object, or an error string if not found.
	    EDOC
*/
func RequestByAnyId(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	var hash common.TodoHash = common.TodoHash(vars["id"])
	var err error = nil
	res := FindByAnyId(&hash)

	if res == nil || err != nil {
		if err == nil {
			err = fmt.Errorf("")
		}
		json.NewEncoder(w).Encode(fmt.Sprintf("could not find by any id %s %s", hash, err))
	} else {
		json.NewEncoder(w).Encode(res)
	}
}

/*
		SDOC: API

	  - POST /daypage — Create Today's Day Page
	    Creates a new day page for the current date using the configured =dayPageTemplate=.
	    Day pages are generated in the =dayPagePath= directory. If a day page already exists
	    for today, the existing file is returned. The =dayPageMode= setting controls whether
	    pages are created daily or weekly.

	    *Method:* =POST=

	    *Request Body:* Empty (no body required).

	    *Response:*

	  - On success (=200=): A JSON object with the day page file path.

	  - On failure (=400=): An error string.
	    EDOC
*/
func PostCreateDayPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	res, err := CreateDayPage()
	if res == nil || err != nil {
		if err == nil {
			err = fmt.Errorf("")
		}
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(fmt.Sprintf("Failed creating day page %s", err))
	} else {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(res)
	}
}

/*
		SDOC: API

	  - GET /daypage/{date} — Get Day Page at a Specific Date
	    Returns the day page file corresponding to the given date. The date format is
	    =YYYY-DD-MM=. If no day page exists for that date, the response includes an error.

	    *Method:* =GET=

	    *Path Parameters:*
	    | Parameter | Type   | Description                                              |
	    |-----------+--------+----------------------------------------------------------|
	    | ={date}=  | string | Date in =YYYY-DD-MM= format.                            |

	    *Response:* A JSON object with the day page data, or an error string.
	    EDOC
*/
func RequestDayPageAt(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	var args common.Date = common.Date(vars["date"])
	res, err := GetDayPageAt(&args)
	if res == nil || err != nil {
		if err == nil {
			err = fmt.Errorf("")
		}
		json.NewEncoder(w).Encode(fmt.Sprintf("RequestDayPageAt could not get day page %s %s", args, err))
	} else {
		json.NewEncoder(w).Encode(res)
	}
}

/*
		SDOC: API

	  - GET /daypage/increment — Get Day Page Increment
	    Returns the number of days between day pages. When =dayPageMode= is =week=,
	    returns =7=; otherwise returns =1=. Clients use this to calculate the next/previous
	    day page dates for navigation.

	    *Method:* =GET=

	    *Parameters:* None.

	    *Response:* A JSON integer (=1= or =7=).
	    EDOC
*/
func RequestDayPageIncrement(w http.ResponseWriter, r *http.Request) {
	if Conf().Server.DayPageMode == "week" {
		fmt.Fprintln(os.Stderr, "DAYPAGE INC: 7")
		json.NewEncoder(w).Encode(7)
	} else {
		fmt.Fprintln(os.Stderr, "DAYPAGE INC: 1")
		json.NewEncoder(w).Encode(1)
	}
}

/*
		SDOC: API

	  - GET /capture/templates — List Capture Templates
	    Returns the list of capture templates available to the authenticated user. This merges
	    templates defined in the server config (=captureTemplates=) with any per-user templates
	    stored in the extensions file. Templates define the target location, heading type, and
	    template text used by the capture system.

	    *Method:* =GET=

	    *Parameters:* None (user identity is derived from the auth token).

	    Each template's =template= string comes back with its self-answering
	    placeholders *answered*: ={{uuid}}=, ={{now}}=, ={{username}}= and the
	    rest of the table in =internal/common/captemplate.go=. The answer is
	    written in as the placeholder's default rather than over the top of it -
	    ={{uuid}}= becomes ={{uuid|=f81d4fae-...}}= - so a client that does
	    nothing about it has a usable value in the box, and one that would
	    rather answer the name itself still can. A name with no answer is left
	    exactly as written.

	    *Response:* A JSON array of =CaptureTemplate= objects:
	    #+BEGIN_SRC json
	    [
	    {
	    "name": "Todo",
	    "type": "entry",
	    "target": {"Filename": "todo.org", "Id": "Tasks", "Type": "file+headline"},
	    "template": "* TODO %?"
	    }
	    ]
	    #+END_SRC
	    EDOC
*/
func RequestCaptureTemplates(w http.ResponseWriter, r *http.Request) {
	username := GetUsername(r)
	res, err := QueryCaptureTemplates(username)
	// The placeholders nobody can answer differently - {{now}}, {{uuid}},
	// {{username}} - are filled in here rather than left for the client. See
	// capturefill.go: everything a person has to answer is left as written.
	res = FillCaptureTemplateAutos(res, username)
	if err != nil {
		fmt.Fprintf(os.Stderr, "QueryCaptureTemplates: %s", err.Error())
	}
	if res == nil || err != nil {
		if err == nil {
			err = fmt.Errorf("")
		}
		json.NewEncoder(w).Encode(fmt.Sprintf("could not get capture template list %s", err))
	} else {
		json.NewEncoder(w).Encode(res)
	}
}

/*
		SDOC: API

	  - POST /capture — Capture a New Entry
	    Creates a new heading or entry using the capture template system. The template
	    is expanded with the provided data, and the result is inserted at the target
	    location defined by the template. This is the server-side equivalent of
	    =org-capture= in Emacs.

	    *Method:* =POST=

	    *Request Body (JSON):*
	    | Field      | Type      | Required | Description                                              |
	    |------------+-----------+----------+----------------------------------------------------------|
	    | =Template= | string    | yes      | Name of the capture template to use.                     |
	    | =NewNode=  | NewNode   | yes      | Object containing the data for the new entry.            |

	    *Response:* A =ResultMsg= JSON object.

	  - ={"status": true, "msg": "..."}= on success.

	  - An error on failure.
	    EDOC
*/
func PostCapture(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(os.Stderr, "PostCapture")
	username := GetUsername(r)
	body, _ := io.ReadAll(r.Body)
	var args common.Capture
	var err = json.Unmarshal(body, &args)
	if err == nil {
		fmt.Fprintln(os.Stderr, "  Deserialized", args)
		var reply common.ResultMsg
		reply, err = Capture(db, &args, username)
		if err == nil {
			json.NewEncoder(w).Encode(reply)
		} else {
			fmt.Fprintln(os.Stderr, "Capture failed to operate")
			answerErr(w, err)
		}
	} else {
		fmt.Fprintln(os.Stderr, "Failed to deserialize", err, string(body))
		answerErr(w, err)
	}
}

/*
		SDOC: API

	  - GET /exclusivemarker — Get Exclusive Marker
	    Retrieves the heading that currently holds the named exclusive marker tag. Exclusive
	    markers are tags that can only be present on one heading at a time across all files,
	    acting as named bookmarks (e.g. =:NOW:=, =:FOCUS:=).

	    *Method:* =GET=

	    *Query Parameters:*
	    | Parameter | Type   | Required | Description                        |
	    |-----------+--------+----------+------------------------------------|
	    | =name=    | string | yes      | The marker tag name (e.g. =NOW=).  |

	    *Response:* The =Todo= object of the heading holding the marker, or an error string
	    if no heading has the marker.
	    EDOC
*/
func RequestMarker(w http.ResponseWriter, r *http.Request) {
	// This a parameter rather than path
	args := r.URL.Query().Get("name")
	res, err := GetMarkerTag(args)
	if err != nil {
		Log().Errorf("GetMarkerErr: %s", err.Error())
	}
	if res == nil || err != nil {
		if err == nil {
			err = fmt.Errorf("GetMarker Error")
		}
		json.NewEncoder(w).Encode(fmt.Sprintf("could not get exclusive marker %s", err))
	} else {
		json.NewEncoder(w).Encode(res)
	}
}

/*
		SDOC: API

	  - POST /setexclusivemarker — Set Exclusive Marker
	    Moves an exclusive marker tag to a new heading. The tag is first removed from
	    whatever heading currently holds it (if any), then applied to the target heading.
	    This ensures at most one heading in the entire database has the marker at any time.

	    *Method:* =POST=

	    *Request Body (JSON):*
	    | Field  | Type   | Required | Description                                                   |
	    |--------+--------+----------+---------------------------------------------------------------|
	    | =Name= | string | yes      | The marker tag name (e.g. =NOW=, =FOCUS=).                   |
	    | =ToId= | Target | yes      | A =Target= identifying the heading to receive the marker.     |

	    The =Target= object has fields: =Filename=, =Id=, =Type= (one of =file+headline=,
	    =id=, =customid=, =hash=, =file+line=), and optionally =Lvl=.

	    *Response:* A =Result= JSON object with ={"status": true}= on success.
	    EDOC
*/
func PostMarker(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(os.Stderr, "PostMarker")
	body, _ := io.ReadAll(r.Body)
	var args common.ExclusiveTagMarker
	var err = json.Unmarshal(body, &args)
	if err == nil {
		fmt.Fprintln(os.Stderr, "  Deserialized", args)
		var reply common.Result
		reply, err = SetMarkerTag(&args)
		if err == nil {
			json.NewEncoder(w).Encode(reply)
		} else {
			fmt.Fprintln(os.Stderr, "Today failed to operate")
			answerErr(w, err)
		}
	} else {
		fmt.Fprintln(os.Stderr, "Failed to deserialize", err, string(body))
		answerErr(w, err)
	}
}

/*
		SDOC: API

	  - POST /delete — Delete a Heading
	    Removes the heading (and its entire subtree) identified by the target from the org file.
	    The file is re-saved to disk after the deletion. This operation is destructive and
	    cannot be undone through the API.

	    *Method:* =POST=

	    *Request Body (JSON):* A =Target= object identifying the heading to delete.
	    | Field      | Type   | Required | Description                                                      |
	    |------------+--------+----------+------------------------------------------------------------------|
	    | =Filename= | string | varies   | The org filename (used with =file+headline= and =file+line=).    |
	    | =Id=       | string | varies   | The identifier (headline text, hash, id, or line number).        |
	    | =Type=     | string | yes      | One of =file+headline=, =id=, =customid=, =hash=, =file+line=.  |
	    | =Lvl=      | int    | no       | If non-zero, only match headings at this level.                  |

	    *Response:* A =ResultMsg= JSON object.
	    EDOC
*/
func PostDelete(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(os.Stderr, "PostDelete")
	body, _ := io.ReadAll(r.Body)
	var args common.Target
	var err = json.Unmarshal(body, &args)
	if err == nil {
		fmt.Fprintln(os.Stderr, "  Deserialized", args)
		var reply common.ResultMsg
		reply, err = Delete(db, &args)
		if err == nil {
			json.NewEncoder(w).Encode(reply)
		} else {
			fmt.Fprintln(os.Stderr, "Delete failed to operate")
			answerErr(w, err)
		}
	} else {
		fmt.Fprintln(os.Stderr, "Delete to deserialize", err, string(body))
		answerErr(w, err)
	}
}

/*
		SDOC: API

	  - POST /update — Run an Updater Plugin on a Heading
	    Invokes a named updater plugin against the heading identified by the target.
	    Updater plugins perform external synchronization (e.g. the =jira= updater pushes
	    changes to Jira). The updater must be enabled in the server's config under
	    =server.updaters=.

	    *Method:* =POST=

	    *Request Body (JSON):*
	    | Field    | Type   | Required | Description                                               |
	    |----------+--------+----------+-----------------------------------------------------------|
	    | =Name=   | string | yes      | The updater plugin name (e.g. =jira=).                    |
	    | =Target= | Target | yes      | A =Target= identifying the heading to update.             |

	    *Response:* A =ResultMsg= JSON object.
	    EDOC
*/
func PostUpdate(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(os.Stderr, "PostUpdate")
	body, _ := io.ReadAll(r.Body)
	var args common.Update
	var err = json.Unmarshal(body, &args)
	if err == nil {
		fmt.Fprintln(os.Stderr, "  Deserialized", args)
		var reply common.ResultMsg
		reply, err = PluginUpdateTarget(db, &args.Target, args.Name)
		if err == nil {
			json.NewEncoder(w).Encode(reply)
		} else {
			fmt.Fprintln(os.Stderr, "Update failed to operate")
			answerErr(w, err)
		}
	} else {
		fmt.Fprintln(os.Stderr, "Update to deserialize", err, string(body))
		answerErr(w, err)
	}
}

/*
		SDOC: API

	  - POST /refile — Refile a Heading to a New Location
	    Moves a heading (and its subtree) from its current location to a new target.
	    The heading is removed from the source file and inserted as a child of the
	    target heading. Both files are re-saved to disk.

	    *Method:* =POST=

	    *Request Body (JSON):*
	    | Field    | Type   | Required | Description                                              |
	    |----------+--------+----------+----------------------------------------------------------|
	    | =FromId= | Target | yes      | A =Target= identifying the heading to move.              |
	    | =ToId=   | Target | yes      | A =Target= identifying the destination heading.          |

	    *Response:* A =ResultMsg= JSON object.
	    EDOC
*/
func PostRefile(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(os.Stderr, "PostRefile")
	body, _ := io.ReadAll(r.Body)
	var args common.Refile
	var err = json.Unmarshal(body, &args)
	if err == nil {
		fmt.Fprintln(os.Stderr, "  Deserialized", args)
		var reply common.ResultMsg
		reply, err = Refile(db, &args, nil, false)
		if err == nil {
			json.NewEncoder(w).Encode(reply)
		} else {
			fmt.Fprintln(os.Stderr, "Refile failed to operate")
			answerErr(w, err)
		}
	} else {
		fmt.Fprintln(os.Stderr, "Refile to deserialize", err, string(body))
		answerErr(w, err)
	}
}

/*
	SDOC: API

* POST /move — Refile, Copy or Archive, One Heading or Many

	The one endpoint for moving headings about. =Op= is =refile=, =copy= or
	=archive=; =From= is a list of targets and =To= is where they go (ignored by
	=archive=, which works its own destination out of org's archive rules).

	*It is a list on purpose, and a client must not loop over =/refile= itself.*
	A heading's hash is accumulated from the document name and the chain of
	headline titles above it, so it is stable only while the file's structure is:
	move one heading out of a file and every heading after it has a different
	hash. Twenty hashes collected from one search and posted one at a time get
	the first right and then address headings that are not there - or headings
	that have since inherited those hashes. This resolves every source before it
	writes anything, and addresses each by its outline path afterwards.

	A heading whose ancestor is also in the list is *skipped*, not moved: moving
	the ancestor takes it along, and moving it as well would be moving it out of
	the thing that has just moved.

	*Method:* =POST=

	*Request Body (JSON):* A =MoveRequest=.
	| Field    | Type     | Required | Description                                            |
	|----------+----------+----------+--------------------------------------------------------|
	| =Op=     | string   | no       | =refile= (the default), =copy= or =archive=.           |
	| =From=   | []Target | yes      | The headings to move.                                  |
	| =To=     | Target   | for both | Where they go. Not read for =archive=.                 |
	| =Create= | bool     | no       | Create the destination heading when it is missing.     |

	*Response:* A =MoveResponse=: =Ok= only when everything worked, =Results= one
	per heading either way, and =Done= / =Failed= / =Skipped= counts.
	EDOC
*/
func PostMove(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var args common.MoveRequest
	if err := json.Unmarshal(body, &args); err != nil {
		json.NewEncoder(w).Encode(common.MoveResponse{Ok: false, Msg: err.Error()})
		return
	}
	reply, err := Move(db, &args)
	if err != nil && reply.Msg == "" {
		reply.Msg = err.Error()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(reply)
}

/*
	SDOC: API

* POST /copy — Copy a Heading Somewhere Else

	A refile that leaves the original where it is. Same request body as
	=/refile=.

	The copy has its =ID= and =CUSTOM_ID= taken off it. Those are how an
	=[[id:...]]= link finds a heading, and two headings answering to one id is
	not a duplicate - it is a link that points at whichever of them the database
	registered last.

	*Method:* =POST=

	*Request Body (JSON):* A =Refile= object: =FromId= and =ToId=.

	*Response:* A =ResultMsg=.
	EDOC
*/
func PostCopy(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var args common.Refile
	if err := json.Unmarshal(body, &args); err != nil {
		json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: err.Error()})
		return
	}
	reply, err := Copy(db, &args, false)
	if err != nil && reply.Msg == "" {
		reply.Msg = err.Error()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(reply)
}

/*
	SDOC: API

* GET /refile/targets — Where a Heading Can Go

	Every place a heading could be refiled or copied to: each file, and each
	heading down to =depth=.

	This is the structured sibling of =/refilefiles=, and differs from it twice.
	It **falls back to every file** when the =refileTargets= setting is unset -
	which is the default, and which made the flat one answer with nothing at
	all, so a refile dialog had no targets to offer on a fresh server. And a
	target carries its **hash, outline path, keyword and tags**, where the flat
	form is ="file|H1|H2"= - which cannot say which of two headings with the
	same name is meant, and has nowhere to put what tells them apart.

	*Method:* =GET=

	*Query Parameters:*
	| Parameter | Type   | Required | Description                                      |
	|-----------+--------+----------+--------------------------------------------------|
	| =depth=   | int    | no       | How deep to walk. Defaults to 3.                 |
	| =file=    | string | no       | Only this file. Repeatable.                      |

	*Response:* A =RefileTargetList=. =Files= is how many files were looked at,
	and =Truncated= says the walk stopped early.
	EDOC
*/
func RequestRefileTargets2(w http.ResponseWriter, r *http.Request) {
	depth := 0
	fmt.Sscanf(r.URL.Query().Get("depth"), "%d", &depth)
	files := r.URL.Query()["file"]
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(RefileTargets(files, depth))
}

/*
		SDOC: API

	  - POST /archive — Archive a Heading
	    Archives the heading identified by the target. The heading is moved from its current
	    file into the corresponding =_archive= file (e.g. =todo.org_archive=) following
	    standard Org mode archiving conventions. The original file is re-saved.

	    *Method:* =POST=

	    *Request Body (JSON):* A =Target= object identifying the heading to archive.
	    | Field      | Type   | Required | Description                                                      |
	    |------------+--------+----------+------------------------------------------------------------------|
	    | =Filename= | string | varies   | The org filename.                                                |
	    | =Id=       | string | varies   | The identifier.                                                  |
	    | =Type=     | string | yes      | One of =file+headline=, =id=, =customid=, =hash=, =file+line=.  |

	    *Response:* A =ResultMsg= JSON object.
	    EDOC
*/
func PostArchive(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(os.Stderr, "PostArchive")
	body, _ := io.ReadAll(r.Body)
	var args common.Target
	var err = json.Unmarshal(body, &args)
	if err == nil {
		fmt.Fprintln(os.Stderr, "  Deserialized", args)
		var reply common.ResultMsg
		reply, err = Archive(db, &args)
		if err == nil {
			json.NewEncoder(w).Encode(reply)
		} else {
			fmt.Fprintln(os.Stderr, "Archive failed to operate")
			answerErr(w, err)
		}
	} else {
		fmt.Fprintln(os.Stderr, "Archive to deserialize", err, string(body))
		answerErr(w, err)
	}
}

/*
		SDOC: API

	  - GET /clock — Get Current Clock Status
	    Returns the current clocking state. If a heading is actively being clocked,
	    the response includes the start time, the target heading, and =Active: true=.
	    If no clock is running, =Active= is =false=.

	    *Method:* =GET=

	    *Parameters:* None.

	    *Response:* A JSON object:
	    #+BEGIN_SRC json
	    {
	    "Active": true,
	    "Time": { "start": "...", "end": "..." },
	    "Target": { "Filename": "...", "Id": "...", "Type": "..." }
	    }
	    #+END_SRC
	    EDOC
*/
func RequestClock(w http.ResponseWriter, r *http.Request) {
	type ClockData struct {
		Active bool
		Time   org.OrgDate
		Target common.Target
	}
	data := ClockData{}
	active := Clock().IsClockActive()
	data.Active = active
	if active {
		data.Time = *Clock().GetTime()
		data.Target = *Clock().GetTarget()
	}
	json.NewEncoder(w).Encode(data)
}

/*
		SDOC: API

	  - POST /clockin — Clock In to a Heading
	    Starts a clock on the heading identified by the target. If another heading is
	    currently clocked in, it is automatically clocked out first. A =CLOCK:= entry
	    with the start time is added to the heading's logbook drawer.

	    *Method:* =POST=

	    *Request Body (JSON):* A =Target= object identifying the heading to clock into.
	    | Field      | Type   | Required | Description                                                      |
	    |------------+--------+----------+------------------------------------------------------------------|
	    | =Filename= | string | varies   | The org filename.                                                |
	    | =Id=       | string | varies   | The identifier.                                                  |
	    | =Type=     | string | yes      | One of =file+headline=, =id=, =customid=, =hash=, =file+line=.  |

	    *Response:* A =ResultMsg= JSON object.
	    EDOC
*/
func PostClockIn(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(os.Stderr, "PostClockIn")
	body, _ := io.ReadAll(r.Body)
	var args common.Target
	var err = json.Unmarshal(body, &args)
	if err == nil {
		fmt.Fprintln(os.Stderr, "  Deserialized", args)
		var reply common.ResultMsg
		reply, err = Clock().ClockIn(&args)
		if err == nil {
			// The clock publishes its own clockin/clockout events (clock.go).
			json.NewEncoder(w).Encode(reply)
		} else {
			fmt.Fprintln(os.Stderr, "ClockIn failed to operate")
			answerErr(w, err)
		}
	} else {
		fmt.Fprintln(os.Stderr, "ClockIn to deserialize", err, string(body))
		answerErr(w, err)
	}
}

/*
		SDOC: API

	  - POST /clockout — Clock Out
	    Stops the currently active clock. The end time is recorded on the open =CLOCK:=
	    entry and the duration is calculated. If no clock is active, this is a no-op.

	    *Method:* =POST=

	    *Request Body:* Ignored (body is read but not used).

	    *Response:* A =ResultMsg= JSON object.
	    EDOC
*/
func PostClockOut(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(os.Stderr, "PostClockOut")
	body, err := io.ReadAll(r.Body)
	if err == nil {
		var reply common.ResultMsg
		reply, err = Clock().ClockOut()
		if err == nil {
			json.NewEncoder(w).Encode(reply)
		} else {
			fmt.Fprintln(os.Stderr, "ClockOut failed to operate")
			answerErr(w, err)
		}
	} else {
		fmt.Fprintln(os.Stderr, "ClockOut to deserialize", err, string(body))
		answerErr(w, err)
	}
}

/*
		SDOC: API

	  - GET /clockreport — Generate a Clock Report
	    Generates a summary of clocked time across all headings for the specified time block.
	    Supports blocks like =today=, =yesterday=, =thisweek=, =lastweek=, =thismonth=, etc.

	    *Method:* =GET=

	    *Query Parameters:*
	    | Parameter | Type   | Required | Description                                                  |
	    |-----------+--------+----------+--------------------------------------------------------------|
	    | =block=   | string | no       | Time block to report on. Defaults to =today=.                |

	    *Response:* A JSON array of =ClockEntry= objects, each containing =headline=,
	    =filename=, =level=, and =mins= (total minutes clocked).
	    EDOC
*/
func RequestClockReport(w http.ResponseWriter, r *http.Request) {
	block := r.URL.Query().Get("block")
	if block == "" {
		block = "today"
	}
	report := GenerateClockReport(block)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(report)
}

/*
		SDOC: API

	  - GET /logbook/{hash} — Get Logbook Entries for a Heading
	    Returns the LOGBOOK clock entries for a heading identified by its hash. Each entry
	    contains the start time, end time (if clocked out), and duration in minutes.

	    *Method:* =GET=

	    *Path Parameters:*
	    | Parameter | Type   | Description                                              |
	    |-----------+--------+----------------------------------------------------------|
	    | ={hash}=  | string | Base64-URL-encoded hash of the heading.                  |

	    *Response:* A =Logbook= JSON object:
	    #+BEGIN_SRC json
	    {
	    "entries": [
	    {"start": "2024-01-15T09:00:00Z", "end": "2024-01-15T10:30:00Z", "mins": 90}
	    ],
	    "totalMin": 90
	    }
	    #+END_SRC
	    Returns =404= if the hash is not found, =400= on invalid hash encoding.
	    EDOC
*/
func RequestLogbook(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	if h, err := GetHash(vars, "hash"); err == nil {
		hash := string(h)
		if s := GetDb().FindByHash(hash); s != nil {
			var logbook common.Logbook
			drawer := s.Headline.FindDrawer(Conf().ClockIntoDrawer)
			if drawer != nil && drawer.Children != nil {
				for _, c := range drawer.Children {
					if c.GetType() == org.ClockNode {
						clk := c.(org.Clock)
						entry := common.LogbookEntry{
							Start: clk.Date.Start.Format(time.RFC3339),
							Mins:  float64(clk.Date.DurationMins),
						}
						if !clk.Date.End.IsZero() {
							entry.End = clk.Date.End.Format(time.RFC3339)
						}
						logbook.Entries = append(logbook.Entries, entry)
						logbook.TotalMin += entry.Mins
					}
				}
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(logbook)
		} else {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
		}
	} else {
		w.WriteHeader(http.StatusBadRequest)
		answerErr(w, err)
	}
}

/*
		SDOC: API

	  - POST /execb — Execute a Source Block
	    Executes the source block at the specified position in an org file. The block is
	    identified by a =PreciseTarget= which combines a =Target= (to find the heading)
	    with a =Row= offset (to locate the specific block within the heading's body).
	    The block is executed according to its language and the result is returned.

	    *Method:* =POST=

	    *Request Body (JSON):*
	    | Field          | Type    | Required | Description                                                |
	    |----------------+---------+----------+------------------------------------------------------------|
	    | =Target=       | Target  | yes      | Identifies the heading containing the block.               |
	    | =Row=          | int     | yes      | Line offset within the heading to locate the block.        |

	    *Response:* A =ResultMsg= JSON object. On success, =msg= contains the execution result.
	    EDOC
*/
func PostExecb(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(os.Stderr, "PostExecb")
	body, err := io.ReadAll(r.Body)
	if err == nil {
		var args common.PreciseTarget
		var err = json.Unmarshal(body, &args)
		if err == nil {
			var reply common.ResultMsg
			reply, err = ExecBlock(db, &args)
			if err == nil {
				json.NewEncoder(w).Encode(reply)
			} else {
				fmt.Fprintln(os.Stderr, "BlockExec failed to exec")
				answerErr(w, err)
			}
		} else {
			fmt.Fprintln(os.Stderr, "BlockExec failed to deserialize", err, string(body))
			answerErr(w, err)
		}
	} else {
		fmt.Fprintln(os.Stderr, "BlockExec failed to read body", err)
		answerErr(w, err)
	}
}

/*
		SDOC: API

	  - GET /tablerandomget — Get a Random Row from a Named Table
	    Selects a random row from a named table (one that has a =#+NAME:= keyword above it)
	    and returns it as a pipe-delimited org table row string. Useful for flashcard-style
	    random selection from data tables.

	    *Method:* =GET=

	    *Query Parameters:*
	    | Parameter | Type   | Required | Description                                             |
	    |-----------+--------+----------+---------------------------------------------------------|
	    | =name=    | string | yes      | The =#+NAME:= of the table to select from.              |

	    *Response:* A =ResultMsg= JSON object. On success, =msg= contains a row like =| col1 | col2 |=.
	    EDOC
*/
func RequestTableRandomGet(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(os.Stderr, "RequestTableRandomGet")
	/*
		vars := mux.Vars(r)
	*/
	name := r.URL.Query().Get("name")
	name = strings.TrimSpace(name)
	var rep common.ResultMsg
	if name != "" {
		tables := GetDb().GetNamedTables(name)
		if len(tables) > 0 {
			table := tables[0]
			rows := tableDataRows(table.Table)
			if len(rows) == 0 {
				json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: fmt.Sprintf("table %s has no data rows", name)})
				return
			}
			row := table.Table.Rows[rows[rand.Intn(len(rows))]]
			w := org.NewOrgWriter()

			res := "|"
			for _, col := range row.Columns {
				res += " "
				res += w.WriteNodesAsString(col.Children...)
				res += " |"
			}
			if len(res) == 1 {
				res += "|"
			}
			rep = common.ResultMsg{Ok: true, Msg: res}
			fmt.Fprintf(os.Stderr, "RESULT: %v\n", rep)
		} else {
			rep = common.ResultMsg{Ok: false, Msg: fmt.Sprintf("Failed to find tables with name %s", name)}
		}
	} else {
		fmt.Fprintln(os.Stderr, "ERR: Name must be specified for table query!")
		rep = common.ResultMsg{Ok: false, Msg: "Failed to find table did you specify a name?"}
	}
	json.NewEncoder(w).Encode(rep)
	//json.NewEncoder(w).Encode(data)
}

/*
		SDOC: API

	  - GET /tablenames — List All Named Tables
	    Returns the names of all tables across all org files that have a =#+NAME:= keyword.
	    These names can be used with other table endpoints like =/tablerandomget=.

	    *Method:* =GET=

	    *Parameters:* None.

	    *Response:* A JSON object:
	    #+BEGIN_SRC json
	    {"Ok": true, "NamedTables": ["vocabulary", "contacts", "inventory"]}
	    #+END_SRC
	    Returns ={"Ok": false, "NamedTables": null}= if no named tables exist.
	    EDOC
*/
func RequestTableNames(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(os.Stderr, "RequestTableNames")
	/*
		vars := mux.Vars(r)
	*/
	type ResultTableNames struct {
		Ok          bool
		NamedTables []string
	}
	var rep ResultTableNames
	tables := GetDb().GetTableNames()
	if len(tables) > 0 {
		keys := make([]string, 0, len(tables))
		for k := range tables {
			keys = append(keys, k)
		}
		rep = ResultTableNames{Ok: true, NamedTables: keys}
	} else {
		rep = ResultTableNames{Ok: false, NamedTables: nil}
	}
	json.NewEncoder(w).Encode(rep)
}

/*
		SDOC: API

	  - POST /tableformulainfo — Get Table Formula Details
	    Returns detailed information about the table and its formulas at the specified position
	    in an org file. This includes the table structure, cell references, and any =#+TBLFM:=
	    formula lines attached to the table.

	    *Method:* =POST=

	    *Request Body (JSON):* A =PreciseTarget= object.
	    | Field    | Type   | Required | Description                                            |
	    |----------+--------+----------+--------------------------------------------------------|
	    | =Target= | Target | yes      | Identifies the heading containing the table.           |
	    | =Row=    | int    | yes      | Line offset within the heading to locate the table.    |

	    *Response:* A =ResultTableDetailsMsg= JSON object on success, or a =ResultMsg=
	    with ={"status": false}= on failure.
	    EDOC
*/
func PostFormulaInfo(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(os.Stderr, "PostFormulaInfo")
	body, err := io.ReadAll(r.Body)
	if err == nil {
		var args common.PreciseTarget
		var err = json.Unmarshal(body, &args)
		if err == nil {
			var reply common.ResultTableDetailsMsg
			reply, err = FormulaDetailsAt(db, &args)
			if err == nil {
				fmt.Fprintf(os.Stderr, "RETURNING OK\n")
				json.NewEncoder(w).Encode(reply)
			} else {
				fmt.Fprintln(os.Stderr, "Table Info failed: ", err.Error())
				rep := common.ResultMsg{Ok: false, Msg: err.Error()}
				json.NewEncoder(w).Encode(rep)
			}
		} else {
			fmt.Fprintln(os.Stderr, "TableInfo failed to deserialize", err, string(body))
			rep := common.ResultMsg{Ok: false, Msg: err.Error()}
			json.NewEncoder(w).Encode(rep)
		}
	} else {
		fmt.Fprintln(os.Stderr, "TableFailed failed to read body", err)
		answerErr(w, err)
	}
}

/*
		SDOC: API

	  - POST /exectable — Execute Table Formulas
	    Evaluates the =#+TBLFM:= formulas on the table at the specified position in an
	    org file and updates the table cells with the computed results. The file is
	    re-saved to disk after the update.

	    *Method:* =POST=

	    *Request Body (JSON):* A =PreciseTarget= object.
	    | Field    | Type   | Required | Description                                            |
	    |----------+--------+----------+--------------------------------------------------------|
	    | =Target= | Target | yes      | Identifies the heading containing the table.           |
	    | =Row=    | int    | yes      | Line offset within the heading to locate the table.    |

	    *Response:* A =ResultMsg= JSON object.
	    EDOC
*/
func PostExect(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(os.Stderr, "PostExecT")
	body, err := io.ReadAll(r.Body)
	if err == nil {
		var args common.PreciseTarget
		var err = json.Unmarshal(body, &args)
		if err == nil {
			var reply common.ResultMsg
			reply, err = ExecTableAt(db, &args)
			if err == nil {
				json.NewEncoder(w).Encode(reply)
			} else {
				fmt.Fprintln(os.Stderr, "Table Execution failed: ", err.Error())
				rep := common.ResultMsg{Ok: false, Msg: err.Error()}
				json.NewEncoder(w).Encode(rep)
			}
		} else {
			fmt.Fprintln(os.Stderr, "TableExec failed to deserialize", err, string(body))
			rep := common.ResultMsg{Ok: false, Msg: err.Error()}
			json.NewEncoder(w).Encode(rep)
		}
	} else {
		fmt.Fprintln(os.Stderr, "TableExec failed to read body", err)
		answerErr(w, err)
	}
}

/*
		SDOC: API

	  - POST /execalltables — Execute All Table Formulas in a File
	    Evaluates =#+TBLFM:= formulas on every table in the specified org file and updates
	    all table cells with the computed results. The file is re-saved to disk after the update.

	    *Method:* =POST=

	    *Request Body (JSON):* A JSON string containing the org filename.
	    #+BEGIN_SRC json
	    "todo.org"
	    #+END_SRC

	    *Response:* A =ResultMsg= JSON object on success, or a =ResultMsg= with
	    ={"status": false}= containing concatenated error messages on failure.
	    EDOC
*/
func PostExecAllT(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(os.Stderr, "PostExecAllT")
	body, err := io.ReadAll(r.Body)
	if err == nil {
		var args string
		var err = json.Unmarshal(body, &args)
		if err == nil {
			reply, errs := ExecAllTables(db, args)
			if len(errs) <= 0 {
				json.NewEncoder(w).Encode(reply)
			} else {
				msg := ""
				for _, e := range errs {
					msg += e.Error() + "\n"
				}
				err = fmt.Errorf(msg)
				fmt.Fprintln(os.Stderr, "All Table Execution failed: ", err.Error())
				rep := common.ResultMsg{Ok: false, Msg: err.Error()}
				json.NewEncoder(w).Encode(rep)
			}
		} else {
			fmt.Fprintln(os.Stderr, "AllTableExec failed to deserialize", err, string(body))
			rep := common.ResultMsg{Ok: false, Msg: err.Error()}
			json.NewEncoder(w).Encode(rep)
		}
	} else {
		fmt.Fprintln(os.Stderr, "AllTableExec failed to read body", err)
		answerErr(w, err)
	}
}

/* SDOC: API
* GET /status/{hash} — Get Valid Status Keywords for a Heading
	Returns the list of valid TODO status keywords that can be applied to the heading
	identified by the given hash. This takes into account both the global =defaultTodoStates= /
	=defaultNextStates= from server config and any file-level =#+TODO:= or =#+SEQ_TODO:=
	definitions.

	*Method:* =GET=

	*Path Parameters:*
	| Parameter | Type   | Description                                              |
	|-----------+--------+----------------------------------------------------------|
	| ={hash}=  | string | Base64-URL-encoded hash of the heading.                  |

	*Response:* A JSON array of valid status keyword strings (e.g. =["TODO", "NEXT", "DONE"]=),
	or an error string.
	EDOC */
/* SDOC: API
* GET /status — The Todo Keywords This Server Accepts

	The keywords from =defaultTodoStates= in the server config, split the way
	org splits them: everything before the =|= is a state a task is still in,
	everything after it is a state it has finished in. A file with its own
	=#+TODO:= line overrides these for its own headings - use =/status/{hash}=
	to ask what one particular heading may be set to.

	*Method:* =GET=

	*Response:* ={"Active": ["TODO", ...], "Done": ["DONE", ...]}=

	The order is the order they were configured in, which is the order they are
	meant to be read in.
EDOC */
func RequestDefaultStatus(w http.ResponseWriter, r *http.Request) {
	active, done := ParseTodoStates(Conf().Server.DefaultTodoStates)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(common.TodoStatesResult{Active: active, Done: done})
}

func RequestValidStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	if h, err := GetHash(vars, "hash"); err == nil {
		var args common.TodoHash = common.TodoHash(h)
		res, err := ValidStatus(&args)
		if err != nil {
			if err == nil {
				err = fmt.Errorf("")
			}
			json.NewEncoder(w).Encode(fmt.Sprintf("could not get valid status for hash %s %s", args, err))
		} else {
			json.NewEncoder(w).Encode(res)
		}
	} else {
		answerErr(w, err)
	}
}

/*
		SDOC: API

	  - GET /next/{hash} — Get Next Sibling Heading
	    Returns the =Todo= data for the next sibling heading (the heading at the same level
	    immediately following the given heading). Returns an error string if there is no
	    next sibling.

	    *Method:* =GET=

	    *Path Parameters:*
	    | Parameter | Type   | Description                                              |
	    |-----------+--------+----------------------------------------------------------|
	    | ={hash}=  | string | Base64-URL-encoded hash of the heading.                  |

	    *Response:* A =Todo= JSON object, or an error string.
	    EDOC
*/
func RequestNextSibling(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	if h, err := GetHash(vars, "hash"); err == nil {
		var args common.TodoHash = common.TodoHash(h)
		res := NextSibling(&args)
		if res == nil {
			json.NewEncoder(w).Encode(fmt.Sprintf("could not get next sibling for hash %s", args))
		} else {
			json.NewEncoder(w).Encode(res)
		}
	} else {
		answerErr(w, err)
	}
}

/*
		SDOC: API

	  - GET /prev/{hash} — Get Previous Sibling Heading
	    Returns the =Todo= data for the previous sibling heading (the heading at the same level
	    immediately before the given heading). Returns an error string if there is no
	    previous sibling.

	    *Method:* =GET=

	    *Path Parameters:*
	    | Parameter | Type   | Description                                              |
	    |-----------+--------+----------------------------------------------------------|
	    | ={hash}=  | string | Base64-URL-encoded hash of the heading.                  |

	    *Response:* A =Todo= JSON object, or an error string.
	    EDOC
*/
func RequestPrevSibling(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	if h, err := GetHash(vars, "hash"); err == nil {
		var args common.TodoHash = common.TodoHash(h)
		res := PrevSibling(&args)
		if res == nil {
			json.NewEncoder(w).Encode(fmt.Sprintf("could not get prev sibling for hash %s", args))
		} else {
			json.NewEncoder(w).Encode(res)
		}
	} else {
		answerErr(w, err)
	}
}

/*
		SDOC: API

	  - GET /child/{hash} — Get Last Child Heading
	    Returns the =Todo= data for the last child heading of the given heading.
	    Returns an error string if the heading has no children.

	    *Method:* =GET=

	    *Path Parameters:*
	    | Parameter | Type   | Description                                              |
	    |-----------+--------+----------------------------------------------------------|
	    | ={hash}=  | string | Base64-URL-encoded hash of the parent heading.           |

	    *Response:* A =Todo= JSON object, or an error string.
	    EDOC
*/
func RequestLastChild(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	if h, err := GetHash(vars, "hash"); err == nil {
		var args common.TodoHash = common.TodoHash(h)
		res := LastChild(&args)
		if res == nil {
			json.NewEncoder(w).Encode(fmt.Sprintf("could not get last child for hash %s", args))
		} else {
			json.NewEncoder(w).Encode(res)
		}
	} else {
		answerErr(w, err)
	}
}

/*
		SDOC: API

	  - GET /alltags — List All Tags
	    Returns a deduplicated list of every tag found across all headings in all tracked
	    org files. Useful for building tag-completion UIs.

	    *Method:* =GET=

	    *Parameters:* None.

	    *Response:* A JSON array of tag strings (e.g. =["WORK", "HOME", "urgent", "PROJECT"]=),
	    or an error string if the tag list cannot be retrieved.
	    EDOC
*/
func RequestTags(w http.ResponseWriter, r *http.Request) {
	// No tags anywhere is an empty list, not an error: worg feeds this
	// straight to an autocomplete, and a string there took the page down.
	res := GetDb().GetAllTags()
	if res == nil {
		res = []string{}
	}
	json.NewEncoder(w).Encode(res)
}

// insideOrgDirs cleans a path and says whether it is inside one of the org
// directories (a relative path is taken from the first). The cleaned path is
// what should be used.
func insideOrgDirs(p string) (string, bool) {
	dirs := Conf().Server.OrgDirs
	if len(dirs) == 0 {
		return "", false
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(dirs[0], p)
	}
	p = filepath.Clean(p)
	for _, d := range dirs {
		root, err := filepath.Abs(d)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(root, p)
		if err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return p, true
		}
	}
	return "", false
}

// The rows of a table worth picking: not the |---+---| rules (rows with no
// columns), not the header (everything above the first rule, when there are
// rows below it - org's own reading), and not the special rows a formula
// table marks with ! # * $ / in the first column. /tablerandomget used to
// pick from every row, so it handed back the header and the rules.
func tableDataRows(t *org.Table) []int {
	if t == nil {
		return nil
	}
	firstRule := -1
	for i, r := range t.Rows {
		if len(r.Columns) == 0 {
			firstRule = i
			break
		}
	}
	headerEnds := 0
	if firstRule > 0 {
		for j := firstRule + 1; j < len(t.Rows); j++ {
			if len(t.Rows[j].Columns) > 0 {
				headerEnds = firstRule
				break
			}
		}
	}
	var out []int
	for i := headerEnds; i < len(t.Rows); i++ {
		r := t.Rows[i]
		if len(r.Columns) == 0 || r.IsSpecial {
			continue
		}
		out = append(out, i)
	}
	return out
}

// answerErr is how a handler says it failed: the error's own text in a
// ResultMsg, with a status that says whose fault it was - 400 for a request
// that could not be read (bad json, a hash that is not base64), 500 for
// everything that went wrong doing the work.
//
// Handlers used to encode the error value itself, json.NewEncoder(w).Encode(err),
// and most error types have no exported fields, so the answer was `{}` - a
// reply that said nothing, not even that it had failed.
// Writes from handlers do not all read their file back; this does it for
// them, after the handler and before the response is flushed.
func freshAfterWrite(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		if r.Method == http.MethodPost || r.Method == http.MethodDelete || r.Method == http.MethodPut {
			if d := odb; d != nil {
				d.ReloadChanged()
			}
		}
	})
}

func answerErr(w http.ResponseWriter, err error) {
	msg := "the request failed"
	if err != nil {
		msg = err.Error()
	}
	status := http.StatusInternalServerError
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	var b64Err b64.CorruptInputError
	if errors.As(err, &syntaxErr) || errors.As(err, &typeErr) || errors.As(err, &b64Err) ||
		errors.Is(err, io.ErrUnexpectedEOF) {
		status = http.StatusBadRequest
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(common.ResultMsg{Ok: false, Msg: msg})
}
