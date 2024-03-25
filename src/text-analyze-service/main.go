package main

import (
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

func GetRandomInt() int {
	val, err := rand.Int(rand.Reader, big.NewInt(300))
	if err != nil {
		panic(err)
	}

	return int(val.Int64())
}

var banned = []string{
	"random",
	"service",
	"analyze",
	"write",
	"health",
	"metrics",
	"favicon.ico",
	"robots.txt",
	"trace",
}

func AnalyzeText(text string) (string, error) {

	fmt.Println("Analyzing text: " + text)
	if len(text) == 0 {
		return "", errors.New("empty text")
	}

	if len(text) > 150 {
		return "", errors.New("too long text")
	}

	for _, b := range banned {
		if strings.Contains(text, b) {
			return "", errors.New("banned word")
		}
	}

	return text, nil
}

func GetRandomText() (string, error) {

	alpha := "abcdefghijklmnopqrstuvwxyz ABCDEFGHIJKLMNOPQRSTUVWXYZ"

	var text string

	for i := 0; i < GetRandomInt(); i++ {
		text += string(alpha[GetRandomInt()%len(alpha)])
	}

	return text, nil
}

func DBConnection(r *http.Request) *sql.DB {
	db, err := sql.Open("sqlite3", "file::memory:?cache=shared")
	if err != nil {
		panic(err)
	}
	db.ExecContext(r.Context(), "CREATE TABLE IF NOT EXISTS texts (id INTEGER PRIMARY KEY, text TEXT)")

	return db
}

func main() {
	analyze := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		PrintHeaders(r)
		text := r.URL.Query().Get("text")
		analyzedText, err := AnalyzeText(text)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}

		w.Write([]byte(analyzedText))
	})

	random := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		PrintHeaders(r)
		text, err := DBConnection(r).QueryContext(r.Context(), "SELECT text FROM texts ORDER BY RANDOM() LIMIT 1")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}

		if !text.Next() {
			randomText, err := GetRandomText()
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(err.Error()))
				return
			}

			w.Write([]byte(randomText))
			return
		}

		var textString string
		err = text.Scan(&textString)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}

		w.Write([]byte(textString))
	})

	write := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		PrintHeaders(r)
		text := r.URL.Query().Get("text")
		key := r.URL.Query().Get("key")

		fmt.Println("text: " + text + " will be written to db")

		if key != os.Getenv("SECRET_KEY") {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("invalid key"))
			return
		}

		db := DBConnection(r)
		defer db.Close()

		_, err := db.ExecContext(r.Context(), "INSERT INTO texts (text) VALUES (?)", text)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}

		w.Write([]byte("ok"))
	})

	http.Handle("/analyze", analyze)
	http.Handle("/random", random)
	http.Handle("/write", write)

	port := os.Getenv("TEXT_ANALYZE_SERVICE_PORT")
	if port == "" {
		port = "3000"
	}

	fmt.Println("Listening on http://localhost:" + port)
	log.Fatal(http.ListenAndServe(":"+port, nil))

}

func PrintHeaders(r *http.Request) {
	for name, values := range r.Header {
		for _, value := range values {
			fmt.Println(name, value)
		}
	}
}
