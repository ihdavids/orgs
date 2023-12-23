package googlecal

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"

	"github.com/ihdavids/orgs/internal/app/orgs/plugs"
)

// GO HERE: https://console.cloud.google.com/
// Create credentials

// Retrieve a token, saves the token, then returns the generated client.
func getClient(config *oauth2.Config, tokFile string) *http.Client {
	// The file token.json stores the user's access and refresh tokens, and is
	// created automatically when the authorization flow completes for the first
	// time.
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	}
	return config.Client(context.Background(), tok)
}

// Request a token from the web, then returns the retrieved token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("Unable to read authorization code: %v", err)
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web: %v", err)
	}
	return tok
}

// Retrieves a token from a local file.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// Saves a token to a file path.
func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

type GoogleCalendar struct {
	Name        string
	Credentials string
	Token       string
	Output      string
	NumEvents   int64
}

func (self *GoogleCalendar) Unmarshal(unmarshal func(interface{}) error) error {
	return unmarshal(self)
}

func (self *GoogleCalendar) Update(db plugs.ODb) {
	fmt.Printf("Google Calendar Update...\n")

	ctx := context.Background()
	b, err := os.ReadFile(self.Credentials)
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v", err)
	}

	// If modifying these scopes, delete your previously saved token.json.
	config, err := google.ConfigFromJSON(b, calendar.CalendarReadonlyScope)
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}
	client := getClient(config, self.Token)

	srv, err := calendar.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		log.Fatalf("Unable to retrieve Calendar client: %v", err)
	}

	t := time.Now().Format(time.RFC3339)
	cals, _ := srv.CalendarList.List().Do()
	f, err := os.OpenFile(self.Output, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to create calendar file: %v", err)
	}
	defer f.Close()
	if cals != nil && cals.Items != nil {
		for _, cal := range cals.Items {
			if cal.Hidden {
				fmt.Printf("SKIPPING: %s\n", cal.Summary)
				continue
			}
			fmt.Fprintf(f, "* %-25s\t\t:Cal:\n", cal.Summary)
			events, err := srv.Events.List(cal.Id).
				ShowDeleted(false).
				SingleEvents(true).
				TimeMin(t).
				MaxResults(self.NumEvents).
				OrderBy("startTime").
				Do()
			if err != nil {
				log.Fatalf("Unable to retrieve next ten of the user's events: %v", err)
			}
			//fmt.Println("Upcoming events:")
			if len(events.Items) == 0 {
				fmt.Printf("No upcoming events found for calendar: %s\n", cal.Summary)
			} else {
				for _, item := range events.Items {
					date, err := time.Parse(time.RFC3339, item.Start.DateTime)
					var datestr string
					if err == nil {
						datestr = date.Format("2006-01-02 Mon 15:04")
					} else {
						datestr = item.Start.Date
					}
					fmt.Fprintf(f, "** TODO %v\n   <%s>\n   :PROPERTIES:\n", item.Summary, datestr)
					fmt.Fprintf(f, "     :CREATED_ON: %v\n", item.Created)
					fmt.Fprintf(f, "     :CREATED_BY: %v\n", item.Creator.DisplayName)
					fmt.Fprintf(f, "     :LINK:       [[%v][Link]]\n", item.HtmlLink)
					fmt.Fprintf(f, "     :ID:         %v\n", item.Id)
					fmt.Fprintf(f, "   :END:\n")
					fmt.Fprintf(f, "   %s\n", item.Description)
				}
			}
		}
	}

}

func (self *GoogleCalendar) Startup(freq int, manager *plugs.PluginManager, opts *plugs.PluginOpts) {
}

// init function is called at boot
func init() {
	plugs.AddPoller("googlecal", func() plugs.Poller {
		return &GoogleCalendar{Credentials: "credentials.json", Token: "token.json", Output: "cal.org", NumEvents: 30}
	})
}
