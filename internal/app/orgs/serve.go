package orgs

import (
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/gorilla/mux"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs/autoclockout"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs/slides"
	"github.com/ihdavids/orgs/internal/assets"
	"github.com/ihdavids/orgs/internal/common"
	"github.com/ihdavids/orgs/worg"
	"github.com/rs/cors"
)

var unicorn string = `
                                               ::.                  
                                              -::.                  
                                             =-.=                   
                                            ==.+                    
                                           =+ =-                    
                                         .=#.+-                     
                                    .:.  -+:-*.                     
                            ..     -@%* -%=:*-                      
                           :-:.    **-*-** *+                       
                          :#:- :::--==::+.*#:                       
                         .*#+=:-=-:. ...: +*##*=.                   
                       :--#+#++++===----:.-*%%%%*.                  
                  .:-===::+-=**=----:  .:-=:  .+@@#*=.              
          .-=++++**+=-:   =: ::     ...    :.   -#%%@%%%#*+*#%%%%*: 
        :+***++++=:       -=         :=***+-       .=*####++#%%%%@@ 
      :+**=:      ..       ==.                           :-:.    -% 
    .+**-        ..        .=+.                         ..     :*@% 
   -**-        ..            +*.  .                    ..:=*#**#%*: 
  -*=.        ..        :: .: ==  .::.         ::..:-==: .:. .:--   
 :*=                   :-==:-+:::.  .--::....-#%%%%%%%%**+-.  .:.   
 ++.       ...         -= --:=-  .. .+#%%%%%%@%++**==--=+##*+==:    
 *:       ..         .==:     ::   .*+==++**+=.                     
 *.     .            .-.      -.   :*.                              
 *.   .. .            -=:    .-    =+                               
 *: ...           .:--: ::   .-    =-                               
 +=..              .::-=..    -:   ==                               
 -+             .:  -=. :.    :-.  :+-                              
 .=:          :--=+=..:        :-.  :=+=:                           
  :=.       . .::. :==:         :-:   :=++-.                        
   :--  .-- ...:-                :--.   .-++=.                      
    .---=-==.:::..                 --:     :=+=.                    
      .-#%::-:.::.                  .-:      .=+-                   
        :%@=                         .--       :+=.                 
         :*@%*:                  ..:-:.:.        =*:                
           -#@@%#+-:.    :=*#%%%%%%%%%%#+===-.    =*-               
             :+#%%%%%%%%%%%%%%###*+===++++======-: :*=              
                 :=+**##*+-.                 :-==++= ++             
                                                 :=+*:-:            
                                                    -+-:            
                                                                    
`

var unicorn2 string = `
                                                                                    
                                                           :..                      
                                                          +=-.                      
                                                        .-+.+                       
                                                       :*= +-                       
                                                       ++:+:                        
                                                     :#+ *=                         
                                                     ++.==                          
                                                   :%@.=+:                          
                                            :**:  .*#.-+-                           
                                   --:     :@@@#  %@*:*+:                           
                                  +=:..    =@-*#::@#-**:                            
                                 -#.= .::---++==-@*-%@+                             
                                .%=*-. ....    .:= .:=:                             
                               .+@:#-:==: .. .::.:- .*%%%#*-                        
                            .---*#.#+*#+*++*+===+**+==+#%%%%+                       
                        ::--=-. += =#*= :-:.: .:-:-==-.   .*@%#*=.                  
               :--==+++*+=--.   =-  --.      ....:--.-:    .=#%%@%%%%%%%%%%####%#-  
           .:=+*******+-.       -+           .-+**+++=.        .-*#%%#+-:=*#%%%%@@+ 
         .=***=-...    ..        ==             .---.                 .  :.     .*% 
       .=**=:         ..          ==.                                 .==-.      =% 
      -**=.         ...           .++.                                .:.     :+%@# 
    .+*=.          ...             .+*.                             :--=*##*+*###+. 
   :+*-          ..           .   .: ++.  .:.                       . .:: :=-:.-.   
  :++:           .           :=+-.-+- =-..  .--:..       =#%%%%%%%%#+=: :-.  ..-    
 .++.             .          =-:+:-:+=-.-:.   :-:::...:=#@%#%%%%%%#%%%%%#+---==:    
 =+:          ....           -= :-: :-.   .. :=++*#####%#=             :--:.        
 +=          ..           .:==:      .-.     *+===++**+-                            
 +:          .            :::.       -:     =+                                      
 *:      . .               ==:      .-.   ..#:                                      
 +-    ..   .             : -*=.    .-    .-#                                       
 ++  ...               ::-=+= :-     -.   :=#                                       
 -*...                ..:-..=+=:.    -:   .:#.                                      
 .+-                :-..=+=:  ::     .-.    =*:                                     
  -=             .----+=: .-.         :-.    =++=.                                  
   =-            .:--. -++=:.          :-:    .=+*+-                                
   .-:         .: =-                    .--.     :=+*+:                             
    .--:  :==- :.=.-=                     :-:       -+*=:                           
      :--==:-=+ - =:::                     .--.       .=+=.                         
        :-#%- ==-: --.                       :-:        .=+-                        
          .#@= .:.                            .-:         :++.                      
           .#@%=                               .-:          =*-                     
             =%@%*-                    ..:--===-..           :++.                   
               =#@@%#*=:.      :-+#%%%%%%%%%%%%%%*====-:       +*.                  
                 .+#%%%%%%%%%%%%%%%%##***+=--:-==+++++++==-.    =*:                 
                     .-+*#####*+=:                     .-+++=-:. +#.                
                                                           :=++==.=#:               
                                                              .-+*=-=               
                                                                 :++:.              
`

var unicorn3 string = `
                                                                     
                                                ░░                   
                                               ▒ ░                   
                                              ▓░░▒                   
                                             ▒░░▒                    
                                            ▒▒ ▓░                    
                                          ░█▓ ▒░                     
                                     ░░   ▓▓ ▒▒                      
                             ░░     ███░ ▓█ ▒▓░                      
                            ▒░░    ▒▓ ▓▒▒█░░▓▒                       
                           ▓▒░  ░░▒░░░░░░░ ▓▓                        
                          ▒█ ▓ ░▒░         ▒▓▓▓▓░                    
                       ░▒▒▒▓░▓░▒▒░░    ░ ░ ░▓████▒                   
                  ░░▒▒▓▓▒░░▒ ▒▒▒ ░░░░     ░░    ▒███▓░               
           ░▒▓▓▓▓██▓▓▓▒░   ░  ░                  ░▓███████▓▒▓█████▓░ 
        ░▓██▓▓▓▓▒▒░        ░           ░▒▒▒░         ░▒▓▓▓▓▒▒▓██████ 
      ░▓█▓▒░                ░                                     ░█ 
    ░▓█▓░                    ░░                                  ▒██ 
   ▒█▓░                       ▒░                           ░▒▓▒▒▓█▓░ 
  ▒█▒                          ▒                    ░░░░        ░▒   
 ░█▒                      ░ ░░░░      ░░      ▒█████████▓▓▓░░   ░░   
 ▓▒                     ░ ░   ░      ░▓███▓▓████▒▓▓▒▒░░░▒▓▓▓▓▒▒▒░    
 █░                    ░░           ░▓▒▒▒▒▓▓▓▒░                      
 █                             ░    ▒▒                               
 █                     ░░     ░    ░▓                                
 █░                 ░░░  ░    ░    ▒▓                                
 █▒                   ░░░      ░   ░▓                                
 ▒▓                 ░░░        ░    ▒▓░                              
  ▓░            ░░░░            ░    ░▒▒░                            
  ░▓░               ░░░          ░░    ░▒▒▒░                         
   ▒▓▒          ░                  ░     ░░▒▒░                       
    ░▓▓▒░░░░   ░                    ░░      ░▒▓░                     
      ░▒▓█░ ░                         ░       ░▒▒░                   
        ░▓█▒                           ░        ░▓░                  
          ▓██▓░                     ░░            ▒▓                 
           ░▓███▓▒░░      ░░▓▓██████████▓▒▒▒░░     ▒▓░               
              ▒▓████████████████▓▓▓▒▒▒▒▒▒▒▒▒▒▒▒▒▒░░ ▒▓░              
                  ░▒▒▓▓▓▓▓▒░                  ░░▒▒▒▒░ ▓░             
                                                  ░▒▓▓ ░             
                                                     ░▒░             
                                                                     
`

var unicorn4 string = `
                                                ..                   
                                               - .                   
                                              +..-                   
                                             -..-                    
                                            -- +.                    
                                          .#+ -.                     
                                     ..   ++ --                      
                             ..     ###. +# -+.                      
                            -..    -+ +--#..+-                       
                           +-.  ..-....... ++                        
                          -# + .-.         -++++.                    
                       .---+.+.--..    . . .+####-                   
                  ..--++-..- --- ....     ..    -###+.               
           .-++++##+++-.   .  .                  .+#######+-+#####+. 
        .+##++++--.        .           .---.         .-++++--+###### 
      .+#+-.                .                                     .# 
    .+#+.                    ..                                  -## 
   -#+.                       -.                           .-+--+#+. 
  -#-                          -                    ....        .-   
 .#-                      . ....      ..      -#########+++..   ..   
 +-                     . .   .      .+###++####-++--...-++++---.    
 #.                    ..           .+----+++-.                      
 #                             .    --                               
 #                     ..     .    .+                                
 #.                 ...  .    .    -+                                
 #-                   ...      .   .+                                
 -+                 ...        .    -+.                              
  +.            ....            .    .--.                            
  .+.               ...          ..    .---.                         
   -+-          .                  .     ..--.                       
    .++-....   .                    ..      .-+.                     
      .-+#. .                         .       .--.                   
        .+#-                           .        .+.                  
          +##+.                     ..            -+                 
           .+###+-..      ..++##########+---..     -+.               
              -+################+++--------------.. -+.              
                  .--+++++-.                  ..----. +.             
                                                  .-++ .             
                                                     .-.             
                                                                     
`

var unicorn5 string = `
                                  . .                
                                 . .               
                               .#...               
                           -. .#---                
                     ..   --+-#+-#.                
                    -.. ... ..- #.                 
                  .-#..--..    .++#+.              
              .--+----.-...     .++###-..          
       .-+++#+++-.  . .             -#####+-+####- 
     .+####+-.               ...       .------+++# 
   .+##-.            ..                         -# 
  .##.                ..                    ...+#+ 
 -#+                   .          .+######+--+#+.  
 #-                ....    .+++-++########+-....   
 #               .         ++--+++-                
 #                    .   .+                       
 #                    .   --                       
 #.                    .  .-                       
 +-         .          .   --.                     
 .+.          ..        ..  .---.                  
  .+-.                    .   ..--.                
   .---.                   ..    .--.              
      -#+                    .     .--             
       .###+-.       .-+####+-.      -+.           
         .+##################+-----.. .+.          
            .-++##++-..        ...----. -          
                                     .--..         
`

// Used by the deprecated Websocket API I need to nuke
// "encoding/json"
var db *Db = &Db{}

func StartServer(sets *common.ServerSettings) {
	// Where the presentation themes are. The template path is config, and a
	// theme that could only be found when the server was started in the right
	// directory would be a theme nobody could rely on.
	slides.SearchPath = sets.TemplatePath
	log.Printf("%s\n", unicorn4)
	log.Printf("[STARTING SERVER] orgs %s\n", versionLine())
	// Force config parsing right up front
	DefaultKeystore()
	Conf()
	// The ports actually served. The plugin manager copied them when the yaml
	// was read, before `-port` had its say, and a notification's link to worg
	// then pointed at a port nothing was listening on.
	if m := Conf().PlugManager; m != nil {
		m.Port, m.TLSPort = sets.Port, sets.TLSPort
	}
	if Conf().Server != nil && Conf().Server != sets {
		Conf().Server.Port, Conf().Server.TLSPort = sets.Port, sets.TLSPort
	}
	// The configured users, now that there is a configuration to read the
	// path out of - and then, unmissably, whether they are any good.
	LoadKeystore()
	WarnOnInsecureCredentials()
	// Settle the dnd module search path from the config before anything can
	// touch the ruleset library, so an export is never served off a library
	// that was built without the configured dndPaths on it.
	DndLibrary()
	LoadExtensions()
	GetDb().Watch()
	defer func() {
		GetDb().Close()
	}()
	//http.HandleFunc(orgs.Conf().ServePath, serveWs)
	//fileServer := http.FileServer(http.Dir("./web"))

	router := mux.NewRouter().StrictSlash(true)
	//router.HandleFunc(orgs.Conf().ServePath, serveWs)
	// move ws up, prevent '/*' from covering '/ws' in not testing mux, httprouter has this bug.
	RestApi(router)

	for i, path := range sets.OrgDirs {
		if i == 0 {
			if fpath, err := filepath.Abs(path); err == nil {
				fmt.Fprintf(os.Stderr, "PREFIX: %s\n", fpath)
				fs := http.FileServer(http.Dir(fpath))
				// The configured folders first and the copies built into the
				// binary behind them, so a server started anywhere has them.
				tpath, _ := filepath.Abs(Conf().TemplateImagesPath)
				fmt.Fprintf(os.Stderr, "TEMP PATH: %s\n", tpath)
				internalfs := http.FileServer(assets.Overlay(tpath, filepath.Join(assets.Dir("templates"), "html_styles", "images")))
				tfpath, _ := filepath.Abs(Conf().TemplateFontPath)
				internalfontfs := http.FileServer(assets.Overlay(tfpath, filepath.Join(assets.Dir("templates"), "fonts")))
				router.PathPrefix("/images/").Handler(http.StripPrefix("/images", fs))
				router.PathPrefix("/orgimages/").Handler(http.StripPrefix("/orgimages", internalfs))
				router.PathPrefix("orgimages/").Handler(http.StripPrefix("orgimages", internalfs))
				router.PathPrefix("/orgfonts/").Handler(http.StripPrefix("/orgfonts", internalfontfs))
				router.PathPrefix("orgfonts/").Handler(http.StripPrefix("orgfonts", internalfontfs))
			}
		}
	}
	// The old web pages and the font awesome fonts their stylesheet loads
	// from ../webfonts. A ./web or ./webfonts in the working directory wins.
	router.PathPrefix("/web/").Handler(http.StripPrefix("/web", http.FileServer(assets.Overlay("./web", assets.Dir("web")))))
	router.PathPrefix("/webfonts/").Handler(http.StripPrefix("/webfonts", http.FileServer(assets.Overlay("./webfonts", assets.Dir("webfonts")))))
	// Serve the embedded worg frontend
	webFS, _ := fs.Sub(worg.Content, ".")
	router.PathPrefix("/").Handler(http.FileServer(http.FS(webFS)))

	// http.Handle(Conf().WebServePath, http.StripPrefix(Conf().WebServePath, fileServer))
	// This is annoying, I can't seem to handle binding to anything other than /
	//http.Handle("/", fileServer)
	//http.HandleFunc("/orgs", portal)
	startPlugins(sets)
	// The transcription service, when the voice block asks orgs to run one.
	// Started here rather than in a plugin because it outlives a reload and is
	// not driven by the org files at all.
	StartWhisper()
	// LanguageTool for writing mode's checker, when languageTool: asks orgs
	// to run one (languagetool.go).
	StartLanguageTool()
	// The trigram index, so that searching the text of every file is not
	// reading the text of every file. Built in the background: the first
	// request is somebody typing into a search box, and a first keystroke that
	// costs a walk of the whole database is what this is here to stop.
	StartTrigramIndex()

	// Everything below serves handler rather than the router, so the
	// localhost check sits in front of the api, the files and worg alike.
	var handler http.Handler = router
	if sets.LocalhostOnly {
		handler = localhostOnly(router)
		fmt.Fprintf(os.Stderr, "LOCALHOST ONLY: requests from other machines are refused\n")
	}

	if sets.HttpsOnly && !sets.AllowHttps {
		log.Fatal("httpsOnly is set but allowHttps is not: there would be nothing to listen on. Set allowHttps: true (and a certificate - orgs cert makes one)")
	}
	go func() {
		if sets.HttpsOnly {
			fmt.Fprintf(os.Stderr, "HTTPS ONLY: no http listener on %d\n", sets.Port)
			return
		}
		var corsHandler http.Handler
		if sets.AccessControl == "*" {
			corsPolicy := cors.New(cors.Options{
				AllowedOrigins: []string{"*"},
				AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
				AllowedHeaders: []string{"*"},
			})
			corsHandler = corsPolicy.Handler(handler)
		} else {
			// Explicitly allow worg dev port over localhost
			corsPolicy := cors.New(cors.Options{
				AllowedOrigins:   []string{fmt.Sprintf("http://localhost:%d", sets.Port), "http://localhost:3000", "https://localhost:3000"},
				AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
				AllowedHeaders:   []string{"*"},
				AllowCredentials: true,
				//	// Enable Debugging for testing, consider disabling in production
				// Debug: true,
			})
			corsHandler = corsPolicy.Handler(handler)
		}
		//if orgs.Conf().AllowHttp {
		fmt.Fprintf(os.Stderr, "HTTP PORT: %d\n", sets.Port)
		//fmt.Fprintf(os.Stderr, "WEB: %s\n", orgs.Conf().WebServePath)
		//fmt.Fprintf(os.Stderr, "ORG: %s\n", orgs.Conf().ServePath)
		listenAndServe(sets.LocalhostOnly, sets.Port, "", "", corsHandler)
		//}
	}()

	// Allow https connections
	if sets.AllowHttps {
		var tlsCorsHandler http.Handler
		if sets.AccessControl == "*" {
			corsPolicy := cors.New(cors.Options{
				AllowedOrigins: []string{"*"},
				AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
				AllowedHeaders: []string{"*"},
			})
			tlsCorsHandler = corsPolicy.Handler(handler)
		} else {
			corsPolicy := cors.New(cors.Options{
				AllowedOrigins:   []string{sets.AccessControl},
				AllowCredentials: true,
				AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
				AllowedHeaders:   []string{"*"},
				Debug:            true,
			})
			tlsCorsHandler = corsPolicy.Handler(handler)
		}
		fmt.Fprintf(os.Stderr, "PORT: %d\n", sets.TLSPort)
		servercrt := sets.ServerCrt
		serverkey := sets.ServerKey
		listenAndServe(sets.LocalhostOnly, sets.TLSPort, servercrt, serverkey, tlsCorsHandler)
	} else {
		// The https listener is what used to hold this function open, so with
		// allowHttps off the process fell straight through both listeners and
		// exited - `orgs serve` on http alone started, printed that it was
		// listening, and was gone before anything could connect to it. The http
		// listener is in a goroutine and does not hold the process up on its
		// own.
		//
		// Waiting on a signal rather than on nothing, so the whisper child and
		// the plugins below still get stopped on a ctrl-c. StartServer is
		// normally ended by log.Fatal, which does not unwind, so this is the
		// only path on which those two lines run at all.
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
	}
	StopWhisper()
	StopLanguageTool()
	StopHarper()
	StopTrigramIndex()
	StopBabelSessions()
	stopPlugins(sets)
}

func startPlugins(sets *common.ServerSettings) {
	autoclockout.RegisterClockAccessor(Clock())
	for _, plug := range sets.Plugins {
		plug.Start(db)
	}
}

func stopPlugins(sets *common.ServerSettings) {
	for _, plug := range sets.Plugins {
		plug.Stop()
	}
}
