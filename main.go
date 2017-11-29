package main

import (
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/http/fcgi"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/securecookie"
	"github.com/justinas/alice"
	"github.com/justinas/nosurf"
	"golang.org/x/crypto/bcrypt"
)

const (
	Port           = 8080
	AuthCookieName = "auth"
)

var (
	router *mux.Router
	sc     = securecookie.New(securecookie.GenerateRandomKey(64), nil)
)

func accessGranted(r *http.Request) bool {
	if mux.CurrentRoute(r).GetName() == "login" {
		return true
	}

	cookie, err := r.Cookie(AuthCookieName)
	if err != nil {
		return false
	}

	var value string
	if err = sc.Decode(AuthCookieName, cookie.Value, &value); err != nil {
		return false
	}

	return value == "authenticated"
}

func mustLogin(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:   AuthCookieName,
		MaxAge: -1,
	})
	path, err := router.Get("login").URL()
	if err != nil {
		panic(err)
	}
	v := url.Values{}
	v.Add("redirect", r.URL.Path)
	http.Redirect(w, r, path.String()+"?"+v.Encode(), http.StatusFound)
}

func PageHandler(name string) http.Handler {
	t := template.Must(template.New("").ParseFiles(
		"templates/_base.html", "templates/"+name+".html",
	))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !accessGranted(r) {
			mustLogin(w, r)
			return
		}

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

func LoginHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hash, err := ioutil.ReadFile("password")
		if err != nil {
			panic(err)
		}
		password := []byte(r.PostFormValue("Password"))
		if err := bcrypt.CompareHashAndPassword(hash, password); err != nil {
			http.Redirect(w, r, r.Referer(), http.StatusFound)
			return
		}

		encoded, err := sc.Encode(AuthCookieName, "authenticated")
		if err != nil {
			panic(err)
		}
		http.SetCookie(w, &http.Cookie{
			Name:  AuthCookieName,
			Value: encoded,
		})
		http.Redirect(w, r, r.FormValue("redirect"), http.StatusFound)
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

func (d *TemplateData) GoogleAPIKey() string {
	return os.Getenv("GOOGLE_API_KEY")
}

func main() {
	rand.Seed(time.Now().UnixNano())

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", Port))
	if err != nil {
		log.Fatal(err)
	}

	router = mux.NewRouter()
	router.Handle("/", PageHandler("index")).Methods("GET").Name("index")

	for _, p := range []string{
		"login",
		"about-us",
		"the-wedding",
		"chicago",
		"photos",
		"rsvp",
	} {
		router.Handle("/"+p, PageHandler(p)).Methods("GET").Name(p)
	}
	router.Handle("/rsvp", RSVPFormHandler()).Methods("POST")
	router.Handle("/login", LoginHandler()).Methods("POST")
	router.PathPrefix("/static/").Methods("GET").Handler(
		http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))),
	)

	chain := alice.New(
		Recover,
		nosurf.NewPure,
	).Then(router)

	log.Printf("listening on :%d\n", Port)
	if err := fcgi.Serve(listener, chain); err != nil {
		log.Fatal(err)
	}
}
