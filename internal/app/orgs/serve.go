package orgs

import (
	"fmt"
	"log"
	"net/http"
	"path/filepath"

	"github.com/gorilla/mux"
	"github.com/rs/cors"
)

// Used by the deprecated Websocket API I need to nuke
// "encoding/json"
var db *Db = &Db{}

func StartServer() {
	fmt.Printf("EXEC SERVE\n")
	// Force config parsing right up front
	DefaultKeystore()
	Conf()
	GetDb().Watch()
	defer func() {
		GetDb().Close()
	}()
	fmt.Println("STARTING SERVER")
	//http.HandleFunc(orgs.Conf().ServePath, serveWs)
	//fileServer := http.FileServer(http.Dir("./web"))

	router := mux.NewRouter().StrictSlash(true)
	//router.HandleFunc(orgs.Conf().ServePath, serveWs)
	// move ws up, prevent '/*' from covering '/ws' in not testing mux, httprouter has this bug.
	RestApi(router)

	for i, path := range Conf().OrgDirs {
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
	// END ROUTING TABLE PathPrefix("/") match '/*' request
	// This needs to be replaced by an org embedded mechanism so it's built in to orgs
	router.PathPrefix("/").Handler(http.FileServer(http.Dir("./web")))

	// http.Handle(Conf().WebServePath, http.StripPrefix(Conf().WebServePath, fileServer))
	// This is annoying, I can't seem to handle binding to anything other than /
	//http.Handle("/", fileServer)
	//http.HandleFunc("/orgs", portal)
	startPlugins()

	// Allow http connections but only from localhost
	go func() {
		corsHandler := cors.Default().Handler(router)
		if Conf().AccessControl != "*" {
			corsPolicy := cors.New(cors.Options{
				AllowedOrigins:   []string{fmt.Sprintf("http://localhost:%d", Conf().Port)},
				AllowCredentials: true,
				//	// Enable Debugging for testing, consider disabling in production
				//	Debug: true,
			})
			corsHandler = corsPolicy.Handler(corsHandler)
		}
		//if orgs.Conf().AllowHttp {
		fmt.Printf("HTTP PORT: %d\n", Conf().Port)
		//fmt.Printf("WEB: %s\n", orgs.Conf().WebServePath)
		//fmt.Printf("ORG: %s\n", orgs.Conf().ServePath)
		err := http.ListenAndServe(fmt.Sprint(":", Conf().Port), corsHandler)
		if err != nil {
			log.Fatal("ListenAndServe: ", err)
		}
		//}
	}()

	corsHandler := cors.Default().Handler(router)
	if Conf().AccessControl != "*" {
		corsPolicy := cors.New(cors.Options{
			AllowedOrigins:   []string{Conf().AccessControl},
			AllowCredentials: true,
			//	// Enable Debugging for testing, consider disabling in production
			//	Debug: true,
		})
		corsHandler = corsPolicy.Handler(corsHandler)
	}
	// Allow https connections
	if Conf().AllowHttps {
		fmt.Printf("PORT: %d\n", Conf().TLSPort)
		//fmt.Printf("WEB: %s\n", orgs.Conf().WebServePath)
		servercrt := Conf().ServerCrt
		serverkey := Conf().ServerKey
		err := http.ListenAndServeTLS(fmt.Sprint(":", Conf().TLSPort), servercrt, serverkey, corsHandler)
		if err != nil {
			log.Fatal("ListenAndServeTLS: ", err)
		}
	}
	stopPlugins()
}

func startPlugins() {
	for _, plug := range Conf().Plugins {
		plug.Start(db)
	}
}

func stopPlugins() {
	for _, plug := range Conf().Plugins {
		plug.Stop()
	}
}
