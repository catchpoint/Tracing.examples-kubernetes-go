package main

import (
	"context"
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

	"github.com/uptrace/opentelemetry-go-extra/otelsql"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
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

	fmt.Println("Tracer initialized")

	return tp
}

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

func getTracerConfig() map[string]interface{} {
	serviceName := os.Getenv("OTEL_SERVICE_NAME")
	if serviceName == "" {
		serviceName = "text-analyzer-service-otel"
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

func DBConnection(r *http.Request) *sql.DB {
	db, err := otelsql.Open("sqlite3", "file::memory:?cache=shared",
		otelsql.WithAttributes(semconv.DBSystemSqlite),
		otelsql.WithDBName("text-analyze-service-db"),
		otelsql.WithTracerProvider(otel.GetTracerProvider()))

	if err != nil {
		panic(err)
	}

	db.ExecContext(r.Context(), "CREATE TABLE IF NOT EXISTS texts (id INTEGER PRIMARY KEY, text TEXT)")

	return db
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
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	fmt.Println("Tracer initialized")

	analyze := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otel.GetTextMapPropagator().Inject(r.Context(), propagation.HeaderCarrier(r.Header))
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

		otel.GetTextMapPropagator().Inject(r.Context(), propagation.HeaderCarrier(r.Header))

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
		otel.GetTextMapPropagator().Inject(r.Context(), propagation.HeaderCarrier(r.Header))

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

	analyzeHandler := NewTraceparentHandler(otelhttp.NewHandler(analyze, "analyze", otelhttp.WithTracerProvider(tp), otelhttp.WithServerName("analyze")))
	randomHandler := NewTraceparentHandler(otelhttp.NewHandler(random, "random", otelhttp.WithTracerProvider(tp), otelhttp.WithServerName("random")))
	writeHandler := NewTraceparentHandler(otelhttp.NewHandler(write, "write", otelhttp.WithTracerProvider(tp), otelhttp.WithServerName("write")))

	http.Handle("/analyze", analyzeHandler)
	http.Handle("/random", randomHandler)
	http.Handle("/write", writeHandler)

	port := os.Getenv("TEXT_ANALYZE_SERVICE_PORT")
	if port == "" {
		port = "3000"
	}

	fmt.Println("Listening on http://localhost:" + port)
	log.Fatal(http.ListenAndServe(":"+port, nil))

}

type TraceparentHandler struct {
	next  http.Handler
	props propagation.TextMapPropagator
}

func NewTraceparentHandler(next http.Handler) *TraceparentHandler {
	return &TraceparentHandler{
		next:  next,
		props: otel.GetTextMapPropagator(),
	}
}

func (h *TraceparentHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	h.props.Inject(req.Context(), propagation.HeaderCarrier(w.Header()))
	h.next.ServeHTTP(w, req)
}
