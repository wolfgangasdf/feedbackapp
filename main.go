package main

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"

	auth "github.com/abbot/go-http-auth"
	"github.com/sethvargo/go-limiter/httplimit"
	"github.com/sethvargo/go-limiter/memorystore"
	qrcode "github.com/skip2/go-qrcode"
	"github.com/sym01/htmlsanitizer"
	"github.com/tidwall/buntdb"
)

var tplSubmitted *template.Template
var tplSubmit *template.Template
var tplAdmin *template.Template

const passwordlen = 8
const feedbackmaxlen = 10000

var db *buntdb.DB

type TagData struct {
	Tag string
}

type Config struct {
	Port int `json:"port"`
}

func getConfig() *Config {
	conf := &Config{Port: 8000}
	b, err := os.ReadFile("feedbackapp-settings.json")
	if err != nil {
		log.Println("Can't read config: " + err.Error())
	} else {
		if err = json.Unmarshal(b, conf); err != nil {
			log.Fatal("Can't parse config: " + err.Error())
		}
	}
	return conf
}

// serve files from go-bindata, print but ignore errors.
func serveStaticFile(w http.ResponseWriter, path string) {
	fmt.Println("serveFileAsset: " + path)
	mimeType := mime.TypeByExtension(filepath.Ext(path))
	b, err := Asset(path)
	if err != nil {
		log.Println(err)
	} else {
		w.Header().Set("Content-Type", mimeType)
		w.Write(b)
	}
}

func index(w http.ResponseWriter, r *http.Request) {
	fmt.Println("index: url=" + r.URL.Path)
	if r.URL.Path == "/" {
		w.WriteHeader(http.StatusNotFound)
		return
	} else {
		serveStaticFile(w, r.URL.Path[1:])
	}
}

func checkTag(tag string) error {
	if len(tag) > 100 {
		return errors.New("tag too long")
	}
	for _, c := range tag {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			return errors.New("wrong char in tag: " + tag)
		}
	}
	return nil
}

// last part of url is tag - it makes sure that it is safe
func getTagFromURL(url string) (tag string, err error) {
	lidx := strings.LastIndex(url, "/")
	tag = url[lidx+1:]
	if err := checkTag(tag); err != nil {
		return "", err
	}
	// check if tag exists
	err = db.View(func(tx *buntdb.Tx) error {
		if _, err := tx.Get("tag:" + tag + ":password"); err != nil {
			return errors.New("tag doesn't exist: " + tag)
		}
		return nil
	})

	return tag, err
}

func handleTag(w http.ResponseWriter, r *http.Request) {
	fmt.Println("handleTag: url=" + r.URL.Path)
	tag, err := getTagFromURL(r.URL.Path)
	if err != nil {
		fmt.Println("error: ", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	tplSubmit.Execute(w, TagData{Tag: tag})
}

func handleSubmit(w http.ResponseWriter, r *http.Request) {
	fmt.Println("handleSubmit: url=" + r.URL.Path)
	tag, err := getTagFromURL(r.URL.Path)
	if err != nil {
		fmt.Println("error: ", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	r.ParseForm()
	submittext := r.FormValue("submitText")
	if len(submittext) > feedbackmaxlen {
		submittext = submittext[:feedbackmaxlen]
	}
	s := htmlsanitizer.NewHTMLSanitizer()
	s.AllowList = nil
	textsane, _ := s.SanitizeString(submittext)
	// for i := 0; i < 20; i++ {
	if err = db.Update(func(tx *buntdb.Tx) error {
		postcountkey := "tag:" + tag + ":postcount"
		pcs, err := tx.Get(postcountkey)
		if err != nil {
			pcs = "0"
		}
		pc := atoi(pcs) + 1
		tx.Set(fmt.Sprintf("tag:%s:post:%04d", tag, pc), "["+time.Now().Format("2006-01-02 15:04:05")+"] "+textsane, nil)
		tx.Set(postcountkey, strconv.Itoa(pc), nil)
		return nil
	}); err != nil {
		log.Fatal(err)
	}
	// }

	tplSubmitted.Execute(w, TagData{Tag: tag})
}

type Feedback struct {
	Feedback     []string `json:"feedback"`
	NewLastIndex int      `json:"newlastindex"`
}

func atoi(s string) int {
	value, _ := strconv.Atoi(s)
	return value
}

func handleFeedbackGetafter(w http.ResponseWriter, r *http.Request) {
	// fmt.Println("handleFeedbackGetafter: url=" + r.URL.RawQuery)
	tag := r.URL.Query().Get("t")
	password := r.URL.Query().Get("p")
	afterIdx := atoi(r.URL.Query().Get("i"))
	msg := Feedback{}
	if err := db.View(func(tx *buntdb.Tx) error {
		if err := checkTag(tag); err != nil {
			return err
		}
		pck := "tag:" + tag + ":password"
		truepassword, err := tx.Get(pck)
		if err != nil {
			truepassword = ""
		}
		if truepassword != password {
			return errors.New("wrong pass: " + truepassword + "/" + password)
		}
		err = tx.AscendKeys("tag:"+tag+":post:*", func(key, val string) bool {
			if idx := atoi(key[strings.LastIndex(key, ":")+1:]); idx > afterIdx {
				msg.Feedback = append(msg.Feedback, val)
				msg.NewLastIndex = idx
			}
			return true
		})
		return err
	}); err != nil {
		fmt.Println("error: ", err)
		w.WriteHeader(http.StatusBadRequest)
	}
	s, _ := json.Marshal(msg)
	w.Write(s)
}

type AdminTag struct {
	Name     string
	Password string
}
type AdminData struct {
	Tags []AdminTag
}

func handleAdmin(w http.ResponseWriter, ar *auth.AuthenticatedRequest) {
	r := ar.Request
	fmt.Println("handleAdmin: url=" + r.URL.Path)
	ad := AdminData{}
	db.View(func(tx *buntdb.Tx) error {
		tx.AscendKeys("tag:*:password", func(key, val string) bool {
			fmt.Printf("admin: %s = %s\n", key, val)
			ss := strings.Split(key, ":")
			ad.Tags = append(ad.Tags, AdminTag{Name: ss[1], Password: val})
			return true
		})
		return nil
	})
	tplAdmin.Execute(w, ad)
}

func handleQrx(w http.ResponseWriter, ar *auth.AuthenticatedRequest) {
	r := ar.Request
	fmt.Println("handleQrx: url=" + r.URL.Path)
	link := r.URL.Query().Get("link")
	fmt.Println("link=" + link)
	var png []byte
	png, _ = qrcode.Encode(link, qrcode.Medium, 256)
	w.Write(png)
}

func getNewPassword(n int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyz")
	b := make([]rune, n)
	for i := range b {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			log.Fatal(err)
		}
		b[i] = letters[num.Int64()]
	}
	return string(b)
}

func handleAdminAddTag(w http.ResponseWriter, ar *auth.AuthenticatedRequest) {
	r := ar.Request
	fmt.Println("handleAdminAddTag: url=" + r.URL.Path)
	r.ParseForm()
	newtag := r.FormValue("newTag")
	if err := checkTag(newtag); err != nil {
		w.Write([]byte("tag wrong: " + err.Error()))
		return
	}
	password := getNewPassword(passwordlen)
	db.Update(func(tx *buntdb.Tx) error {
		tx.Set("tag:"+newtag+":password", password, nil)
		return nil
	})
	http.Redirect(w, &r, "/admin", http.StatusSeeOther)
}

func handleAdminRmTag(w http.ResponseWriter, ar *auth.AuthenticatedRequest) {
	r := ar.Request
	fmt.Println("handleAdminRmTag: url=" + r.URL.Path)
	tag, err := getTagFromURL(r.URL.Path)
	if err != nil {
		fmt.Println("error: ", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := db.Update(func(tx *buntdb.Tx) error {
		var delkeys []string
		tx.AscendKeys("tag:"+tag+":post:*", func(k, v string) bool {
			fmt.Println("add to delkeys: ", k)
			delkeys = append(delkeys, k)
			return true
		})
		for _, k := range delkeys {
			if _, err = tx.Delete(k); err != nil {
				return err
			}
		}
		if _, err = tx.Delete("tag:" + tag + ":password"); err != nil {
			return err
		}
		tx.Delete("tag:" + tag + ":postcount")
		return err
	}); err != nil {
		fmt.Println("error: ", err)
	}
	db.Shrink()
	http.Redirect(w, &r, "/admin", http.StatusSeeOther)
}

func main() {
	tplSubmitted = template.Must(template.New("submitted").Parse(string(MustAsset("submitted.html"))))
	tplSubmit = template.Must(template.New("submit").Parse(string(MustAsset("submit.html"))))
	tplAdmin = template.Must(template.New("admin").Parse(string(MustAsset("admin.html"))))

	conf := getConfig()

	authenticator := auth.NewBasicAuthenticator("feedbackapp", auth.HtpasswdFileProvider("feedbackapp-logins.htpasswd"))

	// db
	var err error
	db, err = buntdb.Open("feedbackapp.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	var config buntdb.Config
	if err := db.ReadConfig(&config); err != nil {
		log.Fatal(err)
	}
	config.SyncPolicy = buntdb.Always
	if err := db.SetConfig(config); err != nil {
		log.Fatal(err)
	}

	// rate limiter
	store, err := memorystore.New(&memorystore.Config{
		Tokens:   5,           // Number of tokens allowed per interval.
		Interval: time.Minute, // Interval until tokens reset.
	})
	if err != nil {
		log.Fatal(err)
	}
	middleware, err := httplimit.NewMiddleware(store, httplimit.IPKeyFunc())
	if err != nil {
		log.Fatal(err)
	}

	mux1 := http.NewServeMux()
	mux1.HandleFunc("/", index)
	mux1.HandleFunc("/t/", handleTag)
	mux1.Handle("/submit/", middleware.Handle(http.HandlerFunc(handleSubmit))) // rate limited!
	mux1.HandleFunc("/feedback/getafter", handleFeedbackGetafter)
	mux1.HandleFunc("/admin", authenticator.Wrap(handleAdmin))
	mux1.HandleFunc("/admin/add", authenticator.Wrap(handleAdminAddTag))
	mux1.HandleFunc("/admin/rm/", authenticator.Wrap(handleAdminRmTag))
	mux1.HandleFunc("/qrx", authenticator.Wrap(handleQrx))

	log.Printf("Starting server on :%d...", conf.Port)

	http.ListenAndServe(fmt.Sprintf(":%d", conf.Port), mux1)
}
