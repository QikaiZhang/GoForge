package project

// File contents for a freshly scaffolded project. These are plain
// string constants in this stage; the template stage replaces this
// file with templates under templates/project/.

const goModContent = `module {{NAME}}

go 1.22
`

const mainGoContent = `// Command server runs the {{NAME}} HTTP service.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	port := envOr("PORT", "8080")

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	// registerRoutes is the seam where feature handlers get wired in.
	registerRoutes(mux)

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("{{NAME}} listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
}

// registerRoutes wires feature handlers into the mux. It starts empty;
// goforge generate appends handler wiring here.
func registerRoutes(mux *http.ServeMux) {}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
`

const modelGoContent = `// Package model holds the domain types shared by all layers.
package model

// User is the sample domain type that ships with the scaffold.
// Replace it with the types of your own domain.
type User struct {
	ID   string
	Name string
}
`

const configYamlContent = `# Runtime configuration of the generated service.
# The sample server reads PORT from the environment; keep this file in
# sync with your deployment tooling.
server:
  port: 8080
`

const gitignoreContent = `/bin/
/dist/
*.test
.DS_Store
`

const readmeContent = `# {{NAME}}

A Go service scaffolded by [goforge](https://github.com/QikaiZhang/GoForge).

## Run

` + "`" + `bash
go run ./cmd/server
curl localhost:8080/healthz
` + "`" + `

## Layout

` + "`" + `text
cmd/server/          entry point
internal/handler/    HTTP layer
internal/service/    business rules
internal/repository/ data access
internal/model/      domain types
configs/             runtime configuration
` + "`" + `
`
