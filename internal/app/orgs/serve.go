package orgs

import (
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"path/filepath"

	"github.com/gorilla/mux"
	"github.com/ihdavids/orgs/internal/app/orgs/plugs/autoclockout"
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
	log.Printf("%s\n", unicorn4)
	log.Printf("[STARTING SERVER]\n")
	// Force config parsing right up front
	DefaultKeystore()
	Conf()
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
				fmt.Printf("PREFIX: %s\n", fpath)
				fs := http.FileServer(http.Dir(fpath))
				tpath, _ := filepath.Abs(Conf().TemplateImagesPath)
				fmt.Printf("TEMP PATH: %s\n", tpath)
				internalfs := http.FileServer(http.Dir(tpath))
				tfpath, _ := filepath.Abs(Conf().TemplateFontPath)
				internalfontfs := http.FileServer(http.Dir(tfpath))
				router.PathPrefix("/images/").Handler(http.StripPrefix("/images", fs))
				router.PathPrefix("/orgimages/").Handler(http.StripPrefix("/orgimages", internalfs))
				router.PathPrefix("orgimages/").Handler(http.StripPrefix("orgimages", internalfs))
				router.PathPrefix("/orgfonts/").Handler(http.StripPrefix("/orgfonts", internalfontfs))
				router.PathPrefix("orgfonts/").Handler(http.StripPrefix("orgfonts", internalfontfs))
			}
		}
	}
	// Serve the embedded worg frontend
	webFS, _ := fs.Sub(worg.Content, ".")
	router.PathPrefix("/").Handler(http.FileServer(http.FS(webFS)))

	// http.Handle(Conf().WebServePath, http.StripPrefix(Conf().WebServePath, fileServer))
	// This is annoying, I can't seem to handle binding to anything other than /
	//http.Handle("/", fileServer)
	//http.HandleFunc("/orgs", portal)
	startPlugins(sets)

	// Allow http connections but only from localhost
	go func() {
		var corsHandler http.Handler
		if sets.AccessControl == "*" {
			corsPolicy := cors.New(cors.Options{
				AllowedOrigins:   []string{"*"},
				AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
				AllowedHeaders:   []string{"*"},
			})
			corsHandler = corsPolicy.Handler(router)
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
			corsHandler = corsPolicy.Handler(router)
		}
		//if orgs.Conf().AllowHttp {
		fmt.Printf("HTTP PORT: %d\n", sets.Port)
		//fmt.Printf("WEB: %s\n", orgs.Conf().WebServePath)
		//fmt.Printf("ORG: %s\n", orgs.Conf().ServePath)
		err := http.ListenAndServe(fmt.Sprint(":", sets.Port), corsHandler)
		if err != nil {
			log.Fatal("ListenAndServe: ", err)
		}
		//}
	}()

	// Allow https connections
	if sets.AllowHttps {
		var tlsCorsHandler http.Handler
		if sets.AccessControl == "*" {
			corsPolicy := cors.New(cors.Options{
				AllowedOrigins:   []string{"*"},
				AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
				AllowedHeaders:   []string{"*"},
			})
			tlsCorsHandler = corsPolicy.Handler(router)
		} else {
			corsPolicy := cors.New(cors.Options{
				AllowedOrigins:   []string{sets.AccessControl},
				AllowCredentials: true,
				AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
				AllowedHeaders:   []string{"*"},
				Debug: true,
			})
			tlsCorsHandler = corsPolicy.Handler(router)
		}
		fmt.Printf("PORT: %d\n", sets.TLSPort)
		servercrt := sets.ServerCrt
		serverkey := sets.ServerKey
		err := http.ListenAndServeTLS(fmt.Sprint(":", sets.TLSPort), servercrt, serverkey, tlsCorsHandler)
		if err != nil {
			log.Fatal("ListenAndServeTLS: ", err)
		}
	}
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
