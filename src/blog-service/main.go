package main

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"text/template"
	"time"

	"github.com/go-co-op/gocron"
	_ "github.com/mattn/go-sqlite3"
)

var templates = template.Must(template.ParseGlob("views/**/*"))

func DBConnection(r *http.Request) *sql.DB {
	DatabasePath := "database.db"

	db, err := sql.Open("sqlite3", DatabasePath)

	if err != nil {
		panic(err)
	}

	return db
}

func oneHourAgo() string {
	log.Println("Deleting old posts")
	return time.Now().Add(-1 * time.Hour).Format("2006-01-02 15:04:05")
}

func deleteOldPosts() {
	db := DBConnection(nil)
	defer db.Close()

	stmt, _ := db.Prepare("DELETE FROM posts WHERE created_at < ?")
	stmt.Exec(oneHourAgo())

}

func runCron() {
	scheduler := gocron.NewScheduler(time.UTC)
	scheduler.Every(5).Minute().Do(deleteOldPosts)
	scheduler.StartAsync()
}

func throwRandomError() error {
	rand := rand.Intn(10)
	if rand < 3 {
		panic(GetRandomError())
	}
	return nil
}

func main() {
	runCron()

	mux := http.NewServeMux()

	mux.Handle("/", http.HandlerFunc(Home))
	mux.Handle("/show", http.HandlerFunc(Show))
	mux.Handle("/create", http.HandlerFunc(Create))
	mux.Handle("/store", http.HandlerFunc(Store))
	mux.Handle("/edit", http.HandlerFunc(Edit))
	mux.Handle("/update", http.HandlerFunc(Update))
	mux.Handle("/delete", http.HandlerFunc(Delete))

	mux.Handle("/generate", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status, err := GetRandomText(*r)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}

		w.Write([]byte(status))

	}))

	port := os.Getenv("PORT")
	if port == "" {
		port = "5000"
	}

	log.Println("Listen on the port http://localhost:" + port)

	log.Fatal(http.ListenAndServe(":"+port, mux))

}

type Post struct {
	Id        int
	Title     string
	Content   string
	Author    string
	CreatedAt string
}

// Functions
func Home(w http.ResponseWriter, r *http.Request) {
	DBConnection := DBConnection(r)
	posts, err := DBConnection.QueryContext(r.Context(), "SELECT * FROM posts")
	if err != nil {
		panic(err.Error())
	}

	post := Post{}
	postArrays := []Post{}

	for posts.Next() {
		var id int
		var title, content, author, created_at string
		err = posts.Scan(&id, &title, &content, &author, &created_at)
		if err != nil {
			panic(err.Error())
		}
		post.Id = id
		post.Title = title
		if len(content) > 450 {
			post.Content = content[:450] + "..."
		} else {
			post.Content = content
		}

		post.Author = author
		post.CreatedAt = FormatDate(created_at)

		postArrays = append(postArrays, post)
	}
	templates.ExecuteTemplate(w, "home", postArrays)
}
func Show(w http.ResponseWriter, r *http.Request) {

	id := r.URL.Query().Get("id")

	DBConnection := DBConnection(r)
	result, err := DBConnection.QueryContext(r.Context(), "SELECT * FROM posts WHERE id = "+id+" LIMIT 1; ")
	if err != nil {
		panic(err.Error())
	}
	post := Post{}

	for result.Next() {
		var id int
		var title, content, author, created_at string

		err = result.Scan(&id, &title, &content, &author, &created_at)
		if err != nil {
			panic(err.Error())
		}

		post.Id = id
		post.Title = title
		post.Content = content
		post.Author = author
		post.CreatedAt = created_at

	}
	defer result.Close()

	templates.ExecuteTemplate(w, "show", post)
}
func Create(w http.ResponseWriter, r *http.Request) {
	throwRandomError()
	templates.ExecuteTemplate(w, "create", nil)
}
func Store(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		title := r.FormValue("title")
		content := r.FormValue("content")
		author := r.FormValue("author")
		checked := r.FormValue("write-to-analyzer")

		_, err := AnalyzeText(*r, content)

		if err != nil {
			panic(err.Error())
		}

		if checked == "on" {
			_, err := WriteText(*r, title)
			if err != nil {
				panic(err.Error())
			}
		}

		if title == "" || content == "" || author == "" {
			http.Redirect(w, r, "/create", 301)
		}

		DBConnection := DBConnection(r)
		insert, err := DBConnection.PrepareContext(r.Context(), "INSERT INTO posts (title, content, author) VALUES (?, ?, ?)")
		if err != nil {
			panic(err.Error())
		}
		insert.Exec(title, content, author)
		defer insert.Close()

	}
	http.Redirect(w, r, "/", 301)
}
func Edit(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	DBConnection := DBConnection(r)
	result, err := DBConnection.QueryContext(r.Context(), "SELECT * FROM posts WHERE id = "+id+" LIMIT 1; ")
	if err != nil {
		panic(err.Error())
	}
	post := Post{}

	for result.Next() {
		var id int
		var title, content, author, created_at string

		err = result.Scan(&id, &title, &content, &author, &created_at)
		if err != nil {
			panic(err.Error())
		}

		post.Id = id
		post.Title = title
		post.Content = content
		post.Author = author
		post.CreatedAt = created_at

	}
	defer result.Close()

	templates.ExecuteTemplate(w, "edit", post)
}
func Update(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")

	fmt.Println(id)

	if r.Method == "POST" {

		title := r.FormValue("title")
		content := r.FormValue("content")
		author := r.FormValue("author")

		if title == "" || content == "" || author == "" {
			http.Redirect(w, r, "/edit?id="+id, 301)
		}
		DBConnection := DBConnection(r)
		update, err := DBConnection.PrepareContext(r.Context(), "UPDATE posts SET title = ?, content = ?, author = ? WHERE id = ?;")
		if err != nil {
			panic(err.Error())
		}
		update.Exec(title, content, author, id)
		defer update.Close()

	}
	http.Redirect(w, r, "/show?id="+id, 301)
}
func Delete(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	DBConnection := DBConnection(r)
	delete, err := DBConnection.PrepareContext(r.Context(), "DELETE FROM posts WHERE id = ?;")
	if err != nil {
		panic(err.Error())
	}
	delete.Exec(id)
	defer delete.Close()

	http.Redirect(w, r, "/", 301)
}

// Function to format date on this format Month day, year
func FormatDate(date string) string {
	layout := "2006-01-02 15:04:05"
	t, _ := time.Parse(layout, date)
	return t.Format("January 2, 2006")
}

func GetServerError() error {
	return errors.New("Server is down!")
}

func GetDatabaseError() error {
	return errors.New("Database is down!")
}

func GetMemoryOverflowError() error {
	return errors.New("Memory is full!")
}

func GetNetworkError() error {
	return errors.New("Network is not available!")
}

func GetDiskError() error {
	return errors.New("Disk is full!")
}

func GetRandomError() error {
	rand := rand.Intn(5)
	switch rand {
	case 0:
		return GetServerError()
	case 1:
		return GetDatabaseError()
	case 2:
		return GetMemoryOverflowError()
	case 3:
		return GetNetworkError()
	case 4:
		return GetDiskError()
	default:
		return GetServerError()
	}
}

func GetAnalyzerURL() string {
	if os.Getenv("ANALYZER_URL") == "" {
		return "http://localhost:3000"
	}

	return os.Getenv("ANALYZER_URL")
}

func AnalyzeText(r http.Request, str string) (string, error) {
	url := GetAnalyzerURL() + "/analyze?text=" + url.QueryEscape(str)

	resp, err := http.Get(url)

	if err != nil {
		return "", err
	}

	defer resp.Body.Close()

	body := make([]byte, 100)
	resp.Body.Read(body)

	return string(body), nil
}

func WriteText(r http.Request, str string) (string, error) {
	url := GetAnalyzerURL() + "/write?key=" + os.Getenv("SECRET_KEY") + "&text=" + url.QueryEscape(str)

	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()

	return resp.Status, nil
}

func GetRandomText(r http.Request) (string, error) {
	url := GetAnalyzerURL() + "/random"

	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()

	body := make([]byte, 100)
	resp.Body.Read(body)

	return string(body), nil
}
