package main

import (
	"database/sql"
	"encoding/base64"
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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/securecookie"
	"github.com/justinas/alice"
	"github.com/justinas/nosurf"
	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

const (
	Port            = 8080
	AuthCookieName  = "auth"
	AlertCookieName = "alerts"

	CookieKey = "O0lSuL+o85YmkrPSdcqmG/yFnehNVkt8s4IkbAy8WcH+5/kS7jxxqk09mmoxhMOZ1tcnldS5MQxSXMM4q60+RA=="
)

var (
	db     *sql.DB
	router *mux.Router
	sc     *securecookie.SecureCookie
	hits   = struct {
		sync.RWMutex
		n int
	}{}
)

func accessGranted(r *http.Request) bool {
	if mux.CurrentRoute(r).GetName() == "login" {
		return true
	}

	value, err := Cookie(r, AuthCookieName)
	return err == nil && value == "authenticated"
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

		Hit()

		// TODO: remove this when things are stabilized
		t = template.Must(template.New("").ParseFiles(
			"templates/_base.html", "templates/"+name+".html",
		))

		data := TemplateData{r: r}
		if alert, err := r.Cookie(AlertCookieName); err == nil {
			parts := strings.Split(alert.Value, "=")
			data.Alert = &Alert{Type: parts[0], Message: parts[1]}
			http.SetCookie(w, &http.Cookie{
				Name:   AlertCookieName,
				MaxAge: -1,
			})
		}

		log.Println("viewing " + name + ": " + r.RemoteAddr)
		if err := t.ExecuteTemplate(w, "_base.html", &data); err != nil {
			panic(err)
		}
	})
}

func RSVPFormHandler() http.Handler {
	t := template.Must(template.New("").ParseFiles(
		"templates/_base.html", "templates/rsvp-submitted.html",
	))
	errt := template.Must(template.New("").ParseFiles(
		"templates/_base.html", "templates/rsvp-error.html",
	))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var (
			fullName    = r.PostFormValue("FullName")
			attending   = r.PostFormValue("Attending") == "yes"
			plusOne     = r.PostFormValue("PlusOne") == "yes"
			plusOneName = r.PostFormValue("PlusOneName")
		)

		if _, err := db.Exec("insert into rsvp (full_name, attending, plus_one, plus_one_full_name) values ($1, $2, $3, $4)", fullName, attending, plusOne, plusOneName); err != nil {
			if rerr := errt.ExecuteTemplate(w, "_base.html", &struct {
				TemplateData
				Err error
			}{
				TemplateData: TemplateData{r: r},
				Err:          err,
			}); rerr != nil {
				panic(rerr)
			}
		} else {
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
		}
	})
}

func LoginHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hash, err := ioutil.ReadFile("password")
		if err != nil {
			panic(err)
		}
		password := []byte(strings.ToLower(r.PostFormValue("Password")))
		if err := bcrypt.CompareHashAndPassword(hash, password); err != nil {
			http.SetCookie(w, &http.Cookie{
				Name:  AlertCookieName,
				Value: "danger=Invalid password, please try again.",
			})
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

func Cookie(r *http.Request, name string) (string, error) {
	cookie, err := r.Cookie(name)
	if err != nil {
		return "", err
	}

	var value string
	if err = sc.Decode(name, cookie.Value, &value); err != nil {
		return "", err
	}

	return value, nil
}

func SetCookie(w http.ResponseWriter, name, value string) {
}

type TemplateData struct {
	r     *http.Request
	Alert *Alert
}

type Alert struct {
	Type, Message string
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

func Hit() {
	hits.Lock()
	defer hits.Unlock()

	hits.n++
	if err := ioutil.WriteFile("hits", []byte(strconv.Itoa(hits.n)+"\n"), 0644); err != nil {
		log.Println("error writing hits: " + err.Error())
	}
}

func HitsHandler(w http.ResponseWriter, r *http.Request) {
	hits.RLock()
	defer hits.RUnlock()
	w.Write([]byte(strconv.Itoa(hits.n) + "\n"))
}

func main() {
	rand.Seed(time.Now().UnixNano())
	key, err := base64.StdEncoding.DecodeString(CookieKey)
	if err != nil {
		log.Fatal(err)
	}
	sc = securecookie.New(key, nil)

	if b, err := ioutil.ReadFile("hits"); err != nil {
		log.Println("failed to read hits file: " + err.Error())
	} else {
		s := strings.TrimSpace(string(b))
		if hits.n, err = strconv.Atoi(s); err != nil {
			log.Println("failed to read hits file: " + err.Error())
		} else {
			log.Printf("current hits: %d", hits.n)
		}
	}

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", Port))
	if err != nil {
		log.Fatal(err)
	}

	db, err = sql.Open("postgres", "user=damien dbname=damien host=/tmp sslmode=disable")
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
		"accommodations",
		"photos",
		"rsvp",
		"about-website",
	} {
		router.Handle("/"+p, PageHandler(p)).Methods("GET").Name(p)
	}
	router.Handle("/rsvp", RSVPFormHandler()).Methods("POST")
	router.Handle("/login", LoginHandler()).Methods("POST")
	router.PathPrefix("/static/").Methods("GET").Handler(
		http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))),
	)
	router.HandleFunc("/hits", HitsHandler).Methods("GET")

	chain := alice.New(
		Recover,
		nosurf.NewPure,
	).Then(router)

	log.Printf("listening on :%d\n", Port)
	if err := fcgi.Serve(listener, chain); err != nil {
		log.Fatal(err)
	}
}
