package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"text/template"
	"time"

	"github.com/go-co-op/gocron"
	_ "github.com/mattn/go-sqlite3"
	"github.com/uptrace/opentelemetry-go-extra/otelsql"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/contrib/propagators/b3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

var templates = template.Must(template.ParseGlob("views/**/*"))
var traceContext propagation.TraceContext

func DBConnection(r *http.Request) *sql.DB {
	DatabasePath := "database.db"

	db, err := otelsql.Open("sqlite3", DatabasePath,
		otelsql.WithAttributes(semconv.DBSystemSqlite),
		otelsql.WithDBName("blog-service-db"),
		otelsql.WithTracerProvider(otel.GetTracerProvider()))

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

func initSdtout() *sdktrace.TracerProvider {

	// Create stdout exporter to be able to retrieve
	// the collected spans.
	exporter, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		log.Fatal(err)
	}

	// Create a new trace provider instance.
	tp := sdktrace.NewTracerProvider(
		// Always be sure to batch in production.
		sdktrace.WithBatcher(exporter),
	)

	// Set the TracerProvider as global default to ensure all traces from
	// the same process are sent to the same backend.
	otel.SetTracerProvider(tp)

	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			b3.New(),
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	fmt.Println("Tracer initialized")

	return tp
}

func getTracerConfig() map[string]interface{} {
	serviceName := os.Getenv("OTEL_SERVICE_NAME")
	if serviceName == "" {
		serviceName = "blog-service-otel"
	}

	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		log.Fatal("OTEL_EXPORTER_OTLP_ENDPOINT environment variable not set")
	}

	headerString := os.Getenv("OTEL_EXPORTER_OTLP_HEADERS")

	headers := map[string]string{}

	if headerString != "" {
		for _, header := range strings.Split(headerString, ",") {
			parts := strings.Split(header, "=")
			if len(parts) != 2 {
				log.Fatalf("invalid header %q", header)
			}
			headers[parts[0]] = parts[1]
		}
	}

	return map[string]interface{}{
		"OTEL_EXPORTER_OTLP_ENDPOINT": endpoint,
		"OTEL_EXPORTER_OTLP_HEADERS":  headers,
		"OTEL_SERVICE_NAME":           serviceName,
	}
}

func main() {
	env := getTracerConfig()

	ctx := context.Background()

	url := env["OTEL_EXPORTER_OTLP_ENDPOINT"].(string)
	headers := env["OTEL_EXPORTER_OTLP_HEADERS"].(map[string]string)

	fmt.Println(url)
	fmt.Println(headers)
	fmt.Println(env["OTEL_SERVICE_NAME"].(string))
	client := otlptracegrpc.NewClient(
		otlptracegrpc.WithEndpoint(url),
		otlptracegrpc.WithHeaders(headers),
	)

	exp, err := otlptrace.New(ctx, client)
	if err != nil {
		log.Fatalf("failed to create exporter: %v", err)
	}

	tp := trace.NewTracerProvider(
		trace.WithSampler(trace.AlwaysSample()),
		trace.WithBatcher(exp),
		trace.WithResource(
			resource.NewWithAttributes(
				semconv.SchemaURL,
				semconv.ServiceNameKey.String(env["OTEL_SERVICE_NAME"].(string)),
				attribute.String("service.type", "go"),
			),
		),
	)

	defer func() {
		if err := tp.Shutdown(ctx); err != nil {
			log.Printf("error shutting down tracer provider: %v", err)
		}
		if err := exp.Shutdown(ctx); err != nil {
			log.Printf("error shutting down exporter: %v", err)
		}
	}()

	otel.SetTracerProvider(tp)

	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.Baggage{},
			propagation.TraceContext{},
		),
	)

	fmt.Println("Tracer initialized")

	runCron()

	mux := http.NewServeMux()

	mux.Handle("/", otelhttp.NewHandler(http.HandlerFunc(Home), "Home", otelhttp.WithTracerProvider(tp), otelhttp.WithServerName("home")))
	mux.Handle("/show", otelhttp.NewHandler(http.HandlerFunc(Show), "Show", otelhttp.WithTracerProvider(tp), otelhttp.WithServerName("show")))
	mux.Handle("/create", otelhttp.NewHandler(http.HandlerFunc(Create), "Create", otelhttp.WithTracerProvider(tp), otelhttp.WithServerName("create")))
	mux.Handle("/store", otelhttp.NewHandler(http.HandlerFunc(Store), "Store", otelhttp.WithTracerProvider(tp), otelhttp.WithServerName("store")))
	mux.Handle("/edit", otelhttp.NewHandler(http.HandlerFunc(Edit), "Edit", otelhttp.WithTracerProvider(tp), otelhttp.WithServerName("edit")))
	mux.Handle("/update", otelhttp.NewHandler(http.HandlerFunc(Update), "Update", otelhttp.WithTracerProvider(tp), otelhttp.WithServerName("update")))
	mux.Handle("/delete", otelhttp.NewHandler(http.HandlerFunc(Delete), "Delete", otelhttp.WithTracerProvider(tp), otelhttp.WithServerName("delete")))

	mux.Handle("/generate", otelhttp.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status, err := GetRandomText(*r)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}

		w.Write([]byte(status))

	}), "Random", otelhttp.WithTracerProvider(tp), otelhttp.WithServerName("generate")))

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

func GetAnalyzerURL() string {
	if os.Getenv("ANALYZER_URL") == "" {
		return "http://localhost:3000"
	}

	return os.Getenv("ANALYZER_URL")
}

func AnalyzeText(r http.Request, str string) (string, error) {
	url := GetAnalyzerURL() + "/analyze?text=" + url.QueryEscape(str)

	requestClone := r.Clone(r.Context())
	resp, err := otelhttp.Get(requestClone.Context(), url)
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

	requestClone := r.Clone(r.Context())
	resp, err := otelhttp.Get(requestClone.Context(), url)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()

	return resp.Status, nil
}

func GetRandomText(r http.Request) (string, error) {
	url := GetAnalyzerURL() + "/random"

	requestClone := r.Clone(r.Context())
	resp, err := otelhttp.Get(requestClone.Context(), url)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()

	body := make([]byte, 100)
	resp.Body.Read(body)

	return string(body), nil
}
