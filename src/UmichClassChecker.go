package src

import (
	"io"
	"fmt"
	"bytes"
	"errors"
	"strings"
	"net/http"
	"io/ioutil"
	"html/template"
	"encoding/json"
	"encoding/base64"

	"appengine"
	"appengine/user"
	"appengine/mail"
	"appengine/urlfetch"
	"appengine/datastore"
)

func init() {
	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/addClassToTrack", addClassHandler)
	http.HandleFunc("/checkClasses", checkClassesHandler)
	http.HandleFunc("/getTermsAndSchools", getTermsAndSchoolsHandler)
	http.HandleFunc("/refreshAccessToken", refreshAccessTokenHandler)
}

type Class struct {
	UserEmail	string
	TermCode	string
	SchoolCode	string
	Subject		string
	ClassNumber	string
	SectionNumber	string
	Status		bool
}

type Term struct {
	TermCode	string
	TermDescr	string
}

type School struct {
	TermCode	string
	Code		string
	Name		string
}

type AuthInfo struct {
	AccessToken	string
	ConsumerKey	string
	ConsumerSecret	string
}

var baseUrl = "http://api-gw.it.umich.edu/Curriculum/SOC/v1/"

//Handling hitting the home page: Checking the user and loading the info

var templates = template.Must(template.ParseFiles("website/home.html"))

type ClassTableRowInflater struct {
	Term		string
	Subject		string
	ClassNumber	string
	SectionNumber	string
	StatusColor	string
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	didBlockUser := checkTheUserAndBlockIfNecessary(w, r)
	if(didBlockUser) {
		return
	}

	context := appengine.NewContext(r)
	currentUser := user.Current(context)
	classesQuery := datastore.NewQuery("Class").Filter("UserEmail =", currentUser.Email)

	var classes []Class
	_, err := classesQuery.GetAll(context, &classes)
	if(err != nil) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	classRowInflaters := make([]ClassTableRowInflater, len(classes))

	for i, class := range classes {
		statusColor := "red"
		if(class.Status) {
			statusColor = "green"
		}

		classRowInflaters[i] = ClassTableRowInflater {
							Subject: class.Subject,
							ClassNumber: class.ClassNumber,
							SectionNumber: class.SectionNumber,
							StatusColor: statusColor,
					}
	}

	err = templates.ExecuteTemplate(w, "home.html", classRowInflaters)
	if(err != nil) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func checkTheUserAndBlockIfNecessary(w http.ResponseWriter, r *http.Request) (bool) {
	context := appengine.NewContext(r)
	currentUser := user.Current(context)
	if(currentUser == nil) {
		url, _ := user.LoginURL(context, "/")
		fmt.Fprintf(w, "<a href=\"%s\">Sign in or register</a>", url)
		return true
	} else if !isUserAllowed(currentUser.Email) {
		fmt.Fprintf(w, "You're not authorized to use this app.")
		return true
	}

	return false
}

func isUserAllowed(userToCheck string) (bool) {
	//Allowing all users for now
	return true;
}

func buildCourseGuideUrl(classToCheck Class) (string) {
	return "http://www.lsa.umich.edu/cg/cg_sections.aspx?content=1960" + classToCheck.Subject + classToCheck.ClassNumber + classToCheck.SectionNumber + "&termArray=f_13_1960"
}

//Handling entering something on the form

func addClassHandler(w http.ResponseWriter, r *http.Request) {
	didBlockUser := checkTheUserAndBlockIfNecessary(w, r)
	if(didBlockUser) {
		return
	}

	context := appengine.NewContext(r)
	currentUser := user.Current(context)

	subject := strings.ToUpper(r.FormValue("Subject"))
	classNumber := r.FormValue("ClassNumber")
	sectionNumber := r.FormValue("SectionNumber")

	classToCheck :=  Class {
				UserEmail: currentUser.Email,
				Subject: subject,
				ClassNumber: classNumber,
				SectionNumber: sectionNumber,
				Status: false,
			 }

	pageBody, err := loadCourseGuidePageAndCheckValidity(context, classToCheck)
	if(err == nil) {
		classStatus := getStatusOfClassFromPageBody(classToCheck, pageBody)
		classToCheck.Status = classStatus
		_, err := datastore.Put(context, datastore.NewIncompleteKey(context, "Class", nil), &classToCheck)
		if(err != nil) {
			fmt.Fprintf(w, "There was a problem storing your class.")
			return
		} else {
			http.Redirect(w, r, "/", http.StatusFound)
		}
	} else {
		fmt.Fprintf(w, "Couldn't find that class in the course guide.")
	}
}

//Checking up on the classes

func checkClassesHandler(w http.ResponseWriter, r *http.Request) {
	context := appengine.NewContext(r)
	classesQuery := datastore.NewQuery("Class")

	var classes []Class
	classKeys, err := classesQuery.GetAll(context, &classes)
	if(err != nil) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for i, class := range classes {
		pageBody, err := loadCourseGuidePageAndCheckValidity(context, class)
		if(err == nil) {
			fmt.Fprint(w, "Page body retrieved for: " + class.Subject + " " + class.ClassNumber + " " + class.SectionNumber + " - ")

			classStatus := getStatusOfClassFromPageBody(class, pageBody)
			fmt.Fprint(w, "Status: ", classStatus)

			if(classStatus != class.Status) {
				fmt.Fprint(w, " - Status changed, notifying " + class.UserEmail + "\n")
				sendEmailNotificationAboutStatusChange(context, class, classStatus)
			} else {
				fmt.Fprint(w, " - Status hasn't changed\n")
			}
			class.Status = classStatus
			datastore.Put(context, classKeys[i], &class)
		} else {
			fmt.Fprint(w, "Error loading the page for a class: " + err.Error() + "\n")
		}
	}
}

func loadCourseGuidePageAndCheckValidity(context appengine.Context, class Class) (string, error) {
	courseGuideUrl := buildCourseGuideUrl(class)

	client := urlfetch.Client(context)
	response, err := client.Get(courseGuideUrl)

	if(err != nil) {
		return "", err
	}
	body, err := ioutil.ReadAll(response.Body)
	response.Body.Close()
	if(err != nil) {
		return "", err
	}

	bodyString := string(body)

	if(strings.Contains(bodyString, "Section information is currently not available")) {
		return "", errors.New("Class doesn't exist")
	}

	return bodyString, nil
}

//Yeah, this is kind of messy and fragile, but getting third party HTML parsing libraries to work with AppEngine was too much
func getStatusOfClassFromPageBody(class Class, pageBody string) (bool) {
	indexOfSectionRow := strings.Index(pageBody, "<table border=1 cellspacing=0 cellpadding=3><tr><td><b>" + class.SectionNumber + "<br>")
	pageBodyAfterRowStart := pageBody[indexOfSectionRow:len(pageBody)]

	indexOfStatusSpan := strings.Index(pageBodyAfterRowStart, "<span")
	pageBodyAfterSpanStart := pageBodyAfterRowStart[indexOfStatusSpan:len(pageBodyAfterRowStart)]

	indexOfSpanTagClose := strings.Index(pageBodyAfterSpanStart, ">")
	indexOfSpanCloseTagOpen := strings.Index(pageBodyAfterSpanStart, "</")
	statusString := pageBodyAfterSpanStart[indexOfSpanTagClose + 1:indexOfSpanCloseTagOpen]

	return (statusString == "Open")
}

func sendEmailNotificationAboutStatusChange(context appengine.Context, class Class, newStatus bool) {
	var statusMessage string
	if(newStatus) {
		statusMessage = " opened up! Register as soon as you can!"
	} else {
		statusMessage = " filled up! Crap. Sorry."
	}

	msg := &mail.Message {
				Sender:		"Umich Class Checker <umclasschecker@gmail.com>",
				To:		 []string{class.UserEmail},
				Subject:	"Umich Class Status Change",
				Body:		"Hey!\n\n" +
						"The Umich Class Checker noticed that " + class.Subject + " " + class.ClassNumber + ", section " + class.SectionNumber + statusMessage + "\n\n" +
						"Have a good one!",
		   }

	mail.Send(context, msg)
}

//Getting the latest information on terms and schools

func getTermsAndSchoolsHandler(w http.ResponseWriter, r *http.Request) {
	context := appengine.NewContext(r)

	err := getAndStoreTerms(context)
	if(err != nil) {
		context.Infof("Failed to load and store the terms")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	//For each term, request schools

	//For each school, store in datastore
}

type TermsOverallResponse struct {
	OverallResponse TermsResponse `json:"getSOCTermsResponse"`
}

type TermsResponse struct {
	Terms	[]Term `json:"Term"`
}

func getAndStoreTerms(context appengine.Context) (err error) {
	responseBody, err := runApiRequest(context, "Terms")
	if(err != nil) {
		context.Infof("Failed loading the terms!")
		context.Infof(err.Error())
		return err
	}

	context.Infof("About to unmarshal: %s", string(responseBody))
	var termsResponse TermsOverallResponse
	err = json.Unmarshal(responseBody, &termsResponse);
	if(err != nil) {
		context.Infof("Couldn't unmarshal the terms response")
		context.Infof(err.Error())
		return err
	}

	termsQuery := datastore.NewQuery("Term").KeysOnly()
	termKeys, err := termsQuery.GetAll(context, nil)
	if(err != nil) {
		context.Infof("There was a problem loading the existing terms from the datastore")
		context.Infof(err.Error())
		return err
	}
	for _,termKey := range termKeys {
		datastore.Delete(context, termKey)
	}

	for _,term := range termsResponse.OverallResponse.Terms {
		datastore.Put(context, datastore.NewIncompleteKey(context, "Term", nil), &term)
	}

	return nil
}

//API stuff

func runApiRequest(context appengine.Context, path string) (body []byte, err error) {
/*	requestUrl := baseUrl + path

	client := urlfetch.Client(context)
	request, err := http.NewRequest("GET", requestUrl, nil)
	request.Header.Add("Authorization", auth)
	request.Header.Add("Accept", "application/json")

	context.Infof("About to run request at %s", requestUrl)
	response, err := client.Do(request)

	if(err != nil) {
		context.Infof("Request failed!")
		return nil, err
	}

	body, err = ioutil.ReadAll(response.Body)
	response.Body.Close()

	if(err != nil) {
		context.Infof("Couldn't read the response body!")
		return nil, err
	}

	return body, nil */
	return nil, nil
}

type RefreshAccessTokenResponse struct {
	AccessToken	string `json:"access_token"`
}

type WeirdCloser struct {
	io.Reader
}
func (WeirdCloser) Close() error { return nil }

func refreshAccessTokenHandler(w http.ResponseWriter, r *http.Request) {
	context := appengine.NewContext(r)

	authInfoQuery := datastore.NewQuery("AuthInfo")
	var authInfos []AuthInfo
	authInfoKeys, err := authInfoQuery.GetAll(context, &authInfos)
	if(err != nil) {
		context.Infof("Failed to load the auth info")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if(len(authInfos) < 1) {
		//If there isn't any auth info in the datastore,
		//make a blank one so I can fill it in manually later
		blankAuthInfo := AuthInfo { AccessToken:	"blank",
					    ConsumerKey:	"blank",
					    ConsumerSecret:	"blank",
					}
		datastore.Put(context, datastore.NewIncompleteKey(context, "AuthInfo", nil), &blankAuthInfo)
		return
	}

	authInfo := authInfos[0]

	unencodedBasicAuthString := authInfo.ConsumerKey + ":" + authInfo.ConsumerSecret
	encodedBasicAuth := base64.StdEncoding.EncodeToString([]byte(unencodedBasicAuthString))
	encodedBasicAuth = "Basic " + encodedBasicAuth

	client := urlfetch.Client(context)
	request, err := http.NewRequest("POST", "https://api-km.it.umich.edu/token", nil)
	request.Header.Add("Authorization", encodedBasicAuth)
	request.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	request.Body = WeirdCloser{bytes.NewBufferString("grant_type=client_credentials&scope=PRODUCTION")}

	response, err := client.Do(request)

	if(err != nil) {
		context.Infof("Request to get a new access token failed!")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	body, err := ioutil.ReadAll(response.Body)
	response.Body.Close()

	if(err != nil) {
		context.Infof("Couldn't read the response body!")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	context.Infof("About to unmarshal: %s", string(body))
	var refreshAccessTokenResponse RefreshAccessTokenResponse
	err = json.Unmarshal(body, &refreshAccessTokenResponse)
	if(err != nil) {
		context.Infof("Couldn't unmarshal the response!")
		context.Infof(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	authInfo.AccessToken = refreshAccessTokenResponse.AccessToken
	datastore.Put(context, authInfoKeys[0], &authInfo)

	context.Infof("Successfully refreshed the access token!")
}
