package jira

// -----------------------------------------------------------
/* SDOC: Pollers

* Jira

	TODO More documentation on this module

	#+BEGIN_SRC yaml
    - name: "jira"
      endpoint: "your endpoint https://go-jira.atlassian.net"
      user: "The user operating on jira"
      queries: "List of JiraSearch objects"
	#+END_SRC

EDOC */
// -----------------------------------------------------------
/* SDOC: Updaters

* Jira

	TODO More documentation on this module

	#+BEGIN_SRC yaml
    - name: "jira"
      endpoint: "your endpoint https://go-jira.atlassian.net"
      user: "The user operating on jira"
      queries: "List of JiraSearch objects"
	#+END_SRC

EDOC */

import (
	"encoding/base64"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/coryb/oreo"
	"github.com/flosch/pongo2/v5"
	"github.com/go-jira/jira"
	"github.com/go-jira/jira/jiradata"
	"github.com/ihdavids/go-org/org"
	"github.com/ihdavids/orgs/internal/common"
	"gopkg.in/op/go-logging.v1"
)

type oreoLogger struct {
	logger *logging.Logger
}

func (ol *oreoLogger) Printf(format string, args ...interface{}) {
	ol.logger.Debugf(format, args...)
}

type JiraSearch struct {
	jira.SearchOptions `yaml:",inline"`
	Filename           string `yaml:"file"`
	Template           string `yaml:"template"`
}

type JiraSync struct {
	Name string
	// Endpoint is the URL for the Jira service.  Something like: https://go-jira.atlassian.net
	Endpoint string       `yaml:"endpoint,omitempty"`
	User     string       `yaml:"user"`
	Queries  []JiraSearch `yaml:"queries"`
	// AuthenticationMethod is the method we use to authenticate with the jira serivce.
	// Possible values are "api-token", "bearer-token" or "session".
	// The default is "api-token" when the service endpoint ends with "atlassian.net", otherwise it "session".  Session authentication
	// will promt for user password and use the /auth/1/session-login endpoint.
	AuthenticationMethod string `yaml:"authentication-method"`
	// This is required should be your full login name
	Login string

	HaveStarted    bool
	HaveMarshalled bool
	//Credentials string
	//Token       string
	//Output      string
	//NumEvents   int64
	hclient *oreo.Client
	out     *logging.Logger
	pm      *common.PluginManager
}

// curl -D- -u username:password -X POST --data '{"fields":{"project":{"key": "PROJECTKEY"},"summary": "REST ye merry gentlemen.","description": "Creating of an issue using project keys and issue type names using the REST API","issuetype": {"name": "Bug"}}}' -H "Content-Type: application/json" https://mycompanyname.atlassian.net/rest/api/2/issue/

func (self *JiraSync) Unmarshal(unmarshal func(interface{}) error) error {
	if !self.HaveMarshalled {
		self.HaveMarshalled = true
		return unmarshal(self)
	}
	return nil
}

func (self *JiraSync) AuthMethod() string {
	if strings.Contains(self.Endpoint, ".atlassian.net") /*&& o.AuthenticationMethod.Source == "default"*/ {
		return "api-token"
	}
	return self.AuthenticationMethod
}

func (self *JiraSync) AuthMethodIsToken() bool {
	return self.AuthMethod() == "api-token" || self.AuthMethod() == "bearer-token"
}

func (self *JiraSync) GetPass(manager *common.PluginManager) string {
	return manager.GetPass("orgs-jira", "keyring-nonfatal")
}

func (self *JiraSync) GetLogin() string {
	return self.Login
}

func (self *JiraSync) register(o *oreo.Client, manager *common.PluginManager) *oreo.Client {
	self.GetPass(manager)
	o = o.WithPreCallback(func(req *http.Request) (*http.Request, error) {
		if self.AuthMethod() == "api-token" {
			// need to set basic auth header with user@domain:api-token
			token := self.GetPass(manager)
			authHeader := fmt.Sprintf("Basic %s", base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", self.GetLogin(), token))))
			req.Header.Add("Authorization", authHeader)
		} else if self.AuthMethod() == "bearer-token" {
			token := self.GetPass(manager)
			authHeader := fmt.Sprintf("Bearer %s", token)
			req.Header.Add("Authorization", authHeader)
		}
		return req, nil
	})

	o = o.WithPostCallback(func(req *http.Request, resp *http.Response) (*http.Response, error) {
		if self.AuthMethod() == "session" {
			authUser := resp.Header.Get("X-Ausername")
			if authUser == "" || authUser == "anonymous" {
				/*
					// preserve the --quiet value, we need to temporarily disable it so
					// the normal login output is surpressed
					defer func(quiet bool) {
						self.Quiet.Value = quiet
					}(globals.Quiet.Value)
					globals.Quiet.Value = true
				*/
				// we are not logged in, so force login now by running the "login" command
				//app.Parse([]string{"login"})

				// rerun the original request
				return o.Do(req)
			}
		} else if self.AuthMethodIsToken() && resp.StatusCode == 401 {
			//self.SetPass("")
			return o.Do(req)
		}
		return resp, nil
	})
	return o
}

func getProp(sec *org.Section, name string, defVal string) string {
	if p, ok := sec.Headline.Properties.Get(name); ok {
		return p
	}
	return defVal
}

func defaultIssueType(o *oreo.Client, endpoint string, project, issuetype *string) error {
	if project == nil || *project == "" {
		return fmt.Errorf("Project undefined, please use --project argument or set the `project` config property")
	}
	if issuetype != nil && *issuetype != "" {
		return nil
	}
	projectMeta, err := jira.GetIssueCreateMetaProject(o, endpoint, *project)
	if err != nil {
		return err
	}

	issueTypes := map[string]bool{}

	for _, issuetype := range projectMeta.IssueTypes {
		issueTypes[issuetype.Name] = true
	}

	//  prefer "Bug" type
	if _, ok := issueTypes["Bug"]; ok {
		*issuetype = "Bug"
		return nil
	}
	// next best default it "Task"
	if _, ok := issueTypes["Task"]; ok {
		*issuetype = "Task"
		return nil
	}

	return fmt.Errorf("Unable to find default issueType of Bug or Task, please set --issuetype argument or set the `issuetype` config property")
}

func fixUserField(ua jira.HttpClient, endpoint string, userField map[string]interface{}) error {
	if _, ok := userField["accountId"].(string); ok {
		// this field is already GDPR ready
		return nil
	}

	queryName, ok := userField["displayName"].(string)
	if !ok {
		queryName, ok = userField["emailAddress"].(string)
		if !ok {
			// no fields to search on, skip user lookup
			return nil
		}
	}
	users, err := jira.UserSearch(ua, endpoint, &jira.UserSearchOptions{
		// Query field will search users displayName and emailAddress
		Query: queryName,
	})
	if err != nil {
		return err
	}
	if len(users) != 1 {
		return fmt.Errorf("Found %d accounts for users with query %q", len(users), queryName)
	}
	userField["accountId"] = users[0].AccountID
	return nil
}

func fixGDPRUserFields(ua jira.HttpClient, endpoint string, meta jiradata.FieldMetaMap, fields map[string]interface{}) error {
	for fieldName, fieldMeta := range meta {
		// check to see if meta-field is in fields data, otherwise skip
		if _, ok := fields[fieldName]; !ok {
			continue
		}
		if fieldMeta.Schema.Type == "user" {
			userField, ok := fields[fieldName].(map[string]interface{})
			if !ok {
				// for some reason the field seems to be the wrong type in the data
				// even though the schema is a "user"
				continue
			}
			err := fixUserField(ua, endpoint, userField)
			if err != nil {
				return err
			}
			fields[fieldName] = userField
		}
		if fieldMeta.Schema.Type == "array" && fieldMeta.Schema.Items == "user" {
			listUserField, ok := fields[fieldName].([]interface{})
			if !ok {
				// for some reason the field seems to be the wrong type in the data
				// even though the schema is a list of "user"
				continue
			}
			for i, userFieldItem := range listUserField {
				userField, ok := userFieldItem.(map[string]interface{})
				if !ok {
					// for some reason the field seems to be the wrong type in the data
					// even though the schema is a "user"
					continue
				}
				err := fixUserField(ua, endpoint, userField)
				if err != nil {
					return err
				}
				listUserField[i] = userField
			}
			fields[fieldName] = listUserField
		}
	}
	return nil
}

func (self *JiraSync) CreateJira(db common.ODb, target *common.Target) (common.ResultMsg, error) {
	res := common.ResultMsg{}
	res.Ok = false
	res.Msg = "Unknown error, did not create JIRA"
	if !self.HaveStarted {
		fmt.Printf("ERROR: Attempt to create jira when plugin has not started! ABORT\n")
		return res, fmt.Errorf("attempt to create jira when plugin has not started")
	}
	self.out.Infof("CreateJira Called\n")
	if self.Endpoint != "" {
		self.out.Infof("  JIRA: Endpoint is valid...\n")
		_, sec := db.GetFromTarget(target, false)
		if sec != nil {
			self.out.Infof("  JIRA: Found org target...\n")
			o := self.hclient
			issueType := getProp(sec, "ISSUETYPE", "Story")
			effort := getProp(sec, "EFFORT", "")
			project := getProp(sec, "PROJECTKEY", "CLI")
			priority := getProp(sec, "PRIORITY", "")
			labels := getProp(sec, "LABELS", "")
			assigned := getProp(sec, "ASSIGNED", "")
			devFacing := getProp(sec, "DEVFACING", "")
			id := getProp(sec, "CUSTOM_ID", "")
			if id != "" {
				self.out.Infof("  [%s] Is already a jira\n", id)
			}
			body := common.GetSectionBody(sec)
			/*
				id := getProp(sec, "CUSTOM_ID", "")
				assigned := getProp(sec, "ASSIGNED", "")
				priority := getProp(sec, "PRIORITY", "")
				labels := getProp(sec, "LABELS", "")
				status := getProp(sec, "STATUS", "")
			*/
			/*
			   :CUSTOM_ID: {{ i.Key }}
			   :CREATED:   {{ i.Fields.created | age }}
			   :UPDATED:   {{ i.Fields.updated | age }}
			   :ASSIGNED:  {{i.Fields.assignee.displayName | ljust:25}} [{{i.Fields.assignee.emailAddress}}][{{i.Fields.assignee.accountId}}]
			   :REPORTER:  {{i.Fields.reporter.displayName | ljust:25}} [{{i.Fields.reporter.emailAddress}}][{{i.Fields.reporter.accountId}}]
			   :PRIORITY:  {{i.Fields.priority.name | ljust:25}} [{{i.Fields.priority.id}}]
			   :ISSUETYPE: {{i.Fields.issuetype.name | ljust:25}} [{{i.Fields.issuetype.id}}]
			   :SPRINT:    {{i.Fields.sprints}}
			   :LABELS:    {%for lbl in i.Fields.labels%}{{lbl}} {%endfor%}
			   :STATUS:    {{i.Fields.status.name}}
			   :LINK:      [[{{endpoint}}/browse/{{i.Key}}][{{i.Key}}]]
			*/
			/*
				serverInfo, err := jira.ServerInfo(o, self.Endpoint)
				if err != nil {
					return err
				}
				jiraDeploymentType := strings.ToLower(serverInfo.DeploymentType)
			*/
			type templateInput struct {
				Meta      *jiradata.IssueType `yaml:"meta" json:"meta"`
				Overrides map[string]string   `yaml:"overrides" json:"overrides"`
			}

			if err := defaultIssueType(o, self.Endpoint, &project, &issueType); err != nil {
				fmt.Printf("  Failed querying default IssueType: %s...\n", err.Error())
				res.Msg = "Failed querying default issue type for issue"
				return res, err
			}
			createMeta, err := jira.GetIssueCreateMetaIssueType(o, self.Endpoint, project, issueType)
			if err != nil {
				fmt.Printf("  Failed generating meta: %s...\n", err.Error())
				res.Msg = "Failed generating metadata for issue"
				return res, err
			}
			issueUpdate := jiradata.IssueUpdate{}
			issueUpdate.Fields = make(map[string]interface{})
			issueUpdate.Fields["project"] = map[string]string{"key": project}
			issueUpdate.Fields["summary"] = common.GetSectionTitle(sec)
			issueUpdate.Fields["issuetype"] = map[string]string{"name": issueType}
			if assigned != "" {
				a_map := map[string]interface{}{}
				if strings.Contains(assigned, "[") {
					as := strings.Split(assigned, "[")
					for _, a := range as {
						a = strings.TrimSpace(strings.Replace(a, "]", "", -1))
						if strings.Contains("@", a) {
							a_map["emailAddress"] = a
						} else {
							a_map["displayName"] = strings.TrimSpace(a)
						}
					}
				} else {
					if strings.Contains("@", assigned) {
						a := strings.TrimSpace(strings.Replace(assigned, "]", "", -1))
						a_map["emailAddress"] = strings.TrimSpace(a)
					} else {
						a_map["displayName"] = strings.TrimSpace(assigned)
					}
				}
				if len(a_map) > 0 {
					// It is REALLY important that this be an interface map as we check that type later
					issueUpdate.Fields["assignee"] = a_map
				}
			}
			if priority != "" {
				issueUpdate.Fields["priority"] = map[string]string{"name": priority}
			}
			if body != "" {
				issueUpdate.Fields["description"] = body
			}
			if labels != "" {
				issueUpdate.Fields["labels"] = strings.Split(strings.TrimSpace(labels), " ")
			}
			if effort != "" {
				eff := common.ParseWorkDuration(effort)
				if eff != nil {
					seconds := eff.Mins
					issueUpdate.Fields["timetracking"] = map[string]interface{}{"originalEstimate": seconds}
				}
			}
			if devFacing == "" {
				issueUpdate.Fields["customfield_10131"] = []map[string]interface{}{}
				//issueUpdate.Fields["customfield_10131"] = nil
			} else {
				issueUpdate.Fields["customfield_10131"] = []map[string]interface{}{{"value": "Developer Facing"}}
			}

			//issueUpdate.Fields["login"] = self.Login
			//issueUpdate.Fields["reporter"] = map[string]string{"emailAddress": self.Login}

			self.out.Infof("TRYING TO CREATE ISSUE: %s\n", issueUpdate.Fields["summary"])
			self.out.Infof("- Type: [%s]\n", issueType)
			self.out.Infof("- Project: [%s]\n", project)
			self.out.Infof("- Login: [%s]\n", self.Login)
			self.out.Infof("- Assigned: [%s]\n", assigned)
			self.out.Infof("- Labels: [%s]\n", labels)
			self.out.Infof("- description: [%s]\n", body)

			var issueResp *jiradata.IssueCreateResponse
			// This looks up userID for any users in the metadata, it's expensive but necessary
			err = fixGDPRUserFields(o, self.Endpoint, createMeta.Fields, issueUpdate.Fields)
			if err != nil {
				self.out.Errorf("ERROR: Failed to gdpr: %s\n", err.Error())
				res.Msg = "Failed field massage before issue creation"
				return res, err
			}
			if id == "" {
				issueResp, err = jira.CreateIssue(o, self.Endpoint, &issueUpdate)
				// We have to MANUALY update again because this is a creation flag and we want it OFF
				if devFacing == "" && err == nil && issueResp != nil {
					err = jira.EditIssue(o, self.Endpoint, issueResp.Key, &issueUpdate)
				}
			} else {
				err = jira.EditIssue(o, self.Endpoint, id, &issueUpdate)
			}
			if err != nil {
				self.out.Errorf("ERROR: failed to create issue: %s\n", err.Error())
				res.Msg = "Failed creating or editing issue"
				return res, err
			}
			self.out.Infof("GOT TO END...\n")

			if issueResp != nil {
				browseLink := jira.URLJoin(self.Endpoint, "browse", issueResp.Key)
				res.Msg = browseLink
				self.out.Infof("OK %s %s\n", issueResp.Key, browseLink)
			} else {
				if id != "" {
					browseLink := jira.URLJoin(self.Endpoint, "browse", id)
					res.Msg = browseLink
					self.out.Infof("OK %s %s\n", id, browseLink)
				} else {
					if err == nil {
						res.Msg = "Issue response was nil, no information available."
					} else {
						res.Msg = fmt.Sprintf("Issue response was nil, Errinfo: %s", err.Error())
					}
				}
			}
		}
	}
	self.DoQuery(db)
	res.Ok = true
	return res, nil
}

func (self *JiraSync) DoQuery(db common.ODb) {
	// Simple list of issues
	// listTemp := "{% for i in data.Issues %}{{ i.Key | add: \":\" | ljust:12 }}{{ i.Fields.summary }}\n{% endfor %}"

	//self.QueryFields = "assignee,created,priority,reporter,status,summary,updated,issuetype"
	// DONE: Add age
	// DONE: Fix status
	// DONE: Add Comments
	// DONE: Add decription
	// TODO: Add testing instructions
	// TODO: Add FastFlags
	// TODO: Add Sprint and Epic

	if self.Endpoint != "" {
		for _, query := range self.Queries {

			if query.QueryFields == "" {
				query.QueryFields = "assignee,created,priority,reporter,status,summary,updated,issuetype,comment,description,Sprint,labels,project,customfield_10131,customfield_10020,timeoriginalestimate"
			}
			if query.Sort == "" {
				query.Sort = "priority asc, key"
			}
			if self.Endpoint == "" {
				self.out.Error("JIRA: No endpoint, cannot sync!")
			}
			if query.Query == "" {
				// Get My Tasks!
				query.Query = "resolution = unresolved and assignee=currentuser() and project = CLI ORDER BY priority asc, created"
			}
			template := query.Template
			if template == "" {
				template = "jiradefault.tpl"
			}
			data, err := jira.Search(self.hclient, self.Endpoint, &query, jira.WithAutoPagination())
			if err == nil {
				if data != nil {
					fmt.Printf("DATA: %d\n", data.Total)
					self.out.Debugf("JIRA GO: %d\n", data.Total)
				} else {
					self.out.Debugf("JIRA Query is nil?\n")
				}
				ctx := make(map[string]interface{})
				ctx["data"] = data
				ctx["query"] = query.Query
				ctx["fields"] = query.QueryFields
				ctx["sort"] = query.Sort
				ctx["endpoint"] = self.Endpoint
				res := ""
				if data != nil {
					res = self.pm.Tempo.RenderTemplate(template, ctx)
				} else {
					fmt.Printf("ERROR: No data returned from JIRA, abort render")
				}
				//jiracli.RunTemplate("list", data, nil)
				//fmt.Printf("RESULTS: \n%s\n", res)
				os.WriteFile(query.Filename, []byte(res), os.ModePerm)
			} else {
				self.out.Errorf("JIRA: GOT ERROR [%v]\n", err)
			}

		}
	} else {
		self.out.Error("JIRA: No endpoint, cannot sync!")
	}

	//fmt.Printf("TEMPLATE: %s\n", listTemp)

	/*
			:ASSIGNED:  {% for j in i.Fields.assignee %} {{j}}{%endfor%}
		   :ASSIGNED:  {{ i.Fields.assignee }}
		   :ISSUETYPE: {{ i.Fields.issuetype }}
		   :PRIORITY:  {{ i.Fields.priority }}
		   :REPORTER:  {{ i.Fields.reporter }}
	*/
}

func (self *JiraSync) Update(db common.ODb) {
	fmt.Printf("Jira Sync Update...\n")
	self.DoQuery(db)
	/*
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
	*/
}

func toOrgStatus(in *pongo2.Value, param *pongo2.Value) (out *pongo2.Value, errOut *pongo2.Error) {
	if in != nil {
		s := in.String()
		if s != "" {
			switch s {
			case "Open",
				"Submitted",
				"In Progress":
				return pongo2.AsValue("TODO"), nil
			case "Code Complete",
				"Closed":
				return pongo2.AsValue("DONE"), nil
			}
			return in, nil
		}
	}
	return pongo2.AsValue(""), nil
}

func toOrgEffort(in *pongo2.Value, param *pongo2.Value) (out *pongo2.Value, errOut *pongo2.Error) {
	if in != nil && in.IsFloat() {
		v := in.Float()
		out := ""
		// Weeks
		if v > 60.0*60.0*8.0*5.0 {
			weeks := int(v / (60.0 * 60.0 * 8.0 * 5.0))
			out += fmt.Sprintf("%dw", weeks)
			v = math.Mod(v, 5.0)
		}
		if v > 60.0*60.0*8.0 {
			days := int(v / (60.0 * 60.0 * 8.0))
			out += fmt.Sprintf("%dd", days)
			v = math.Mod(v, 8.0)
		}
		if v > 60.0*60.0 {
			hours := int(v / (60.0 * 60.0))
			out += fmt.Sprintf("%dh", hours)
			v = math.Mod(v, 60.0)
		}
		if v > 60.0 {
			minutes := int(v / (60.0))
			out += fmt.Sprintf("%dmin", minutes)
			v = math.Mod(v, 60.0)
		} else {
			seconds := int(v)
			if seconds > 0 {
				out += fmt.Sprintf("%ds", seconds)
			}
		}
		if out != "" {
			return pongo2.AsValue(out), nil
		}
	}
	return pongo2.AsValue(""), nil
}

func (self *JiraSync) Startup(freq int, manager *common.PluginManager, popt *common.PluginOpts) {
	if !self.HaveStarted {
		self.HaveStarted = true
		pongo2.RegisterFilter("orgStatus", toOrgStatus)
		pongo2.RegisterFilter("orgEffort", toOrgEffort)
		self.out = manager.Out
		self.pm = manager
		if self.hclient == nil {
			self.hclient = oreo.New().WithCookieFile(filepath.Join(manager.HomeDir, "cookies.js")).WithLogger(&oreoLogger{manager.Out})
			self.hclient = self.register(self.hclient, manager)
		}
		// TODO: Remove this eventually
		self.DoQuery(nil)
	}
}

func (self *JiraSync) UpdateTarget(db common.ODb, target *common.Target, manager *common.PluginManager) (common.ResultMsg, error) {
	return self.CreateJira(db, target)
}

// init function is called at boot
var jiraSync *JiraSync

func init() {
	if jiraSync == nil {
		jiraSync = &JiraSync{User: os.Getenv("JIRA_USER"), AuthenticationMethod: "api-token"}
	}
	common.AddPoller("jira", func() common.Poller {
		return jiraSync
	})
	common.AddUpdater("jira", func() common.Updater {
		return jiraSync
	})
}
