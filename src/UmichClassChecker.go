package src

import (
	"fmt"
	"net/http"
	"html/template"

	"appengine"
    "appengine/user"
)

func init() {
    http.HandleFunc("/", homeHandler)
    http.HandleFunc("/addClassToTrack", addClassHandler)
}

//Incredibly secure...
var allowedUsers = [...]string{ "test@example.com",
							    "boztalay@umich.edu",
							    "cjspevak@umich.edu" } 

var templates = template.Must(template.ParseFiles("website/home.html"))

//Handling hitting the home page: Checking the user and loading the info

func homeHandler(w http.ResponseWriter, r *http.Request) {
    didBlockUser := checkTheUserAndBlockIfNecessary(w, r)
	if(didBlockUser) {
		return
	}
	
    err := templates.ExecuteTemplate(w, "home.html", nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func checkTheUserAndBlockIfNecessary(w http.ResponseWriter, r *http.Request) (bool) {
	context := appengine.NewContext(r)
    currentUser := user.Current(context)
    if currentUser == nil {
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
	for _, allowedUser := range allowedUsers {
		if userToCheck == allowedUser {
			return true;
		}
	}
	 
	return false;
}

//Handling entering something on the form

func addClassHandler(w http.ResponseWriter, r *http.Request) {
    didBlockUser := checkTheUserAndBlockIfNecessary(w, r)
	if(didBlockUser) {
		return
	}
	
	department := r.FormValue("Department")
	classNumber := r.FormValue("ClassNumber")
	sectionNumber := r.FormValue("SectionNumber")
	
	fmt.Fprintf(w, "%s, %s, %s", department, classNumber, sectionNumber)
}