package main

import (
	"fmt"
	"html/template"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/http/fcgi"
	"path/filepath"
	"time"

	"github.com/gorilla/mux"
	"github.com/justinas/alice"
	"github.com/justinas/nosurf"
)

const PORT = 8080

var (
	router *mux.Router
)

func PageHandler(name string) http.Handler {
	t := template.Must(template.New("").ParseFiles(
		"templates/_base.html", "templates/"+name+".html",
	))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TODO: remove this when things are stabilized
		t = template.Must(template.New("").ParseFiles(
			"templates/_base.html", "templates/"+name+".html",
		))
		log.Println("viewing " + name + ": " + r.RemoteAddr)
		if err := t.ExecuteTemplate(w, "_base.html", &TemplateData{r: r}); err != nil {
			panic(err)
		}
	})
}

func RSVPFormHandler() http.Handler {
	t := template.Must(template.New("").ParseFiles(
		"templates/_base.html", "templates/rsvp-submitted.html",
	))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var (
			fullName    = r.PostFormValue("FullName")
			attending   = r.PostFormValue("Attending") == "yes"
			plusOne     = r.PostFormValue("PlusOne") == "yes"
			plusOneName = r.PostFormValue("PlusOneName")
		)
		log.Printf("full name: %s\n", fullName)
		log.Printf("attending: %t\n", attending)
		log.Printf("plus one?: %t\n", plusOne)
		log.Printf("plus one full name: %s\n", plusOneName)

		cookie := fullName
		if plusOne {
			cookie += fmt.Sprintf(" (+%s)", plusOneName)
		}

		http.SetCookie(w, &http.Cookie{
			Name:  "rsvp",
			Value: cookie,
		})

		if err := t.ExecuteTemplate(w, "_base.html", &TemplateData{r: r}); err != nil {
			panic(err)
		}
	})
}

func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("panic: %s\n", r)
				http.Error(w, http.StatusText(500), 500)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type TemplateData struct {
	r *http.Request
}

func (d *TemplateData) CSRFToken() string {
	return nosurf.Token(d.r)
}

func (d *TemplateData) IsActive(names ...string) bool {
	for _, name := range names {
		if name == mux.CurrentRoute(d.r).GetName() {
			return true
		}
	}
	return false
}

func (d *TemplateData) Path(name string, pairs ...string) string {
	route := router.GetRoute(name)
	if route == nil {
		panic("no route found with name: " + name)
	}
	url, err := route.URL(pairs...)
	if err != nil {
		panic("error building url for path " + name + ": " + err.Error())
	}
	return url.Path
}

func (d *TemplateData) Photos() []string {
	photos, _ := filepath.Glob("static/pictures/*")
	// shuffle: https://stackoverflow.com/a/12267471/823762
	for i := range photos {
		j := rand.Intn(i + 1)
		photos[i], photos[j] = photos[j], photos[i]
	}
	return photos
}

func (d *TemplateData) DaysLeft() int {
	location, err := time.LoadLocation("America/Chicago")
	if err != nil {
		log.Printf("unable to load location: %s\n", err)
		return 0
	}
	weddingDate := time.Date(2018, 10, 5, 0, 0, 0, 0, location)
	return int(weddingDate.Sub(time.Now()) / (time.Hour * 24))
}

func (d *TemplateData) RSVPed() string {
	if cookie, err := d.r.Cookie("rsvp"); err == nil {
		return cookie.Value
	}
	return ""
}

func main() {
	rand.Seed(time.Now().UnixNano())

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", PORT))
	if err != nil {
		log.Fatal(err)
	}

	router = mux.NewRouter()
	router.Handle("/", PageHandler("index")).Methods("GET").Name("index")
	router.Handle("/accommodations", PageHandler("details_accommodations")).Methods("GET").Name("details-accommodations")
	router.Handle("/travel-and-transportation", PageHandler("details_travel_and_transportation")).Methods("GET").Name("details-travel-and-transportation")
	router.Handle("/things-to-do", PageHandler("details_things_to_do")).Methods("GET").Name("details-things-to-do")
	router.Handle("/photos", PageHandler("photos")).Methods("GET").Name("photos")
	router.Handle("/rsvp", PageHandler("rsvp")).Methods("GET").Name("rsvp")
	router.Handle("/rsvp", RSVPFormHandler()).Methods("POST")
	router.PathPrefix("/static/").Methods("GET").Handler(
		http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))),
	)

	chain := alice.New(
		Recover,
		nosurf.NewPure,
	).Then(router)

	log.Printf("listening on :%d\n", PORT)
	if err := fcgi.Serve(listener, chain); err != nil {
		log.Fatal(err)
	}
}
