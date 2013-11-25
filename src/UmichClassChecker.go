package src

import (
	"fmt"
	"errors"
	"strings"
	"net/http"
	"io/ioutil"
	"html/template"
	"encoding/json"

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

type TermStruct struct {
	TermCode	string
	TermDescr	string
}

type School struct {
	TermCode	string
	Code		string
	Name		string
}

//No, I'm not too worried about this. It'll get you one whole request per second.
var auth = "Bearer lcraDGG39rMxPj0WQ7gOw9sLg70a"
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

type TermsOverallResponse struct {
	OverallResponse TermsResponse `json:"getSOCTermsResponse"`
}

type TermsResponse struct {
	Term	[]TermStruct
}

func getTermsAndSchoolsHandler(w http.ResponseWriter, r *http.Request) {
	context := appengine.NewContext(r)

	termsUrl := baseUrl + "Terms"

	client := urlfetch.Client(context)
	request, err := http.NewRequest("GET", termsUrl, nil)
	request.Header.Add("Authorization", auth)
	request.Header.Add("Accept", "application/json")

	context.Infof("About to load url: %s", termsUrl)
	response, err := client.Do(request)

	if(err != nil) {
		context.Infof("Couldn't load terms!")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	body, err := ioutil.ReadAll(response.Body)
	response.Body.Close()
	if(err != nil) {
		context.Infof("Something went wrong processing the terms response")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	context.Infof("About to unmarshal: %s", string(body))
	var termsResponse TermsOverallResponse
	err = json.Unmarshal(body, &termsResponse);
	if(err != nil) {
		context.Infof("Couldn't unmarshal the terms response")
		context.Infof(err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	termsQuery := datastore.NewQuery("TermStruct").KeysOnly()
	termKeys, err := termsQuery.GetAll(context, nil)
	if(err != nil) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _,termKey := range termKeys {
		datastore.Delete(context, termKey)
	}

	for _,term := range termsResponse.OverallResponse.Term {
		context.Infof("Putting a term with code %s in the datastore", term.TermCode)
		datastore.Put(context, datastore.NewIncompleteKey(context, "TermStruct", nil), &term)
	}

	//For each term, request schools

	//For each school, store in datastore
}

//TODO change how this works
func generateApiUrl(termCode string, schoolCode string, subjectCode string, classNumber string, sectionNumber string) (string) {
	baseUrl := "http://api-gw.it.umich.edu/Curriculum/v1/Terms"

	if(termCode != "") {
		baseUrl += "/" + termCode

		if(schoolCode != "") {
			baseUrl += "/Schools/" + schoolCode

			if(subjectCode != "") {
				baseUrl += "/Subjects/" + subjectCode

				if(classNumber != "") {
					baseUrl += "/CatalogNbrs/" + classNumber

					if(sectionNumber != "") {
						baseUrl += "/Sections/" + sectionNumber
					} //Ready, set, go!
				} //Wheee!
			} //Whooooooo!
		} //Watch out for the rock at the bottom!
	} //Phew!

	return baseUrl
}
