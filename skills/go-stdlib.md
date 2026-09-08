# Go stdlib HTTP — Apoc backend patterns

Style mirrors Construct's own `api/*` services (oracle, accounts, source). Go 1.26+, stdlib `net/http` only. No Gin/Echo unless explicitly requested.

## Minimal service

```
my-api/
├── go.mod
├── main.go
├── Dockerfile
└── internal/
    ├── config/config.go
    ├── handlers/handlers.go
    └── middleware/middleware.go
```

## go.mod

```
module construct/myapi

go 1.26.2
```

## main.go

```go
package main

import (
    "log"
    "net/http"
    "time"

    "construct/myapi/internal/config"
    "construct/myapi/internal/handlers"
    "construct/myapi/internal/middleware"
)

func main() {
    cfg := config.Load()
    handlers.Cfg = cfg

    mux := http.NewServeMux()
    mux.HandleFunc("GET /api/things", handlers.ListThings)
    mux.HandleFunc("POST /api/things", handlers.CreateThing)
    mux.HandleFunc("GET /api/things/{id}", handlers.GetThing)
    mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
        handlers.WriteJSON(w, 200, map[string]any{"status": "ok"})
    })

    var handler http.Handler = mux
    handler = middleware.CORS(cfg)(handler)
    handler = middleware.SecurityHeaders(handler)
    handler = middleware.Logger(handler)

    log.Printf("API on :%s", cfg.Port)
    srv := &http.Server{
        Addr:              ":" + cfg.Port,
        Handler:           handler,
        ReadHeaderTimeout: 10 * time.Second,
        ReadTimeout:       30 * time.Second,
        WriteTimeout:      60 * time.Second,
        IdleTimeout:       120 * time.Second,
    }
    log.Fatal(srv.ListenAndServe())
}
```

## Handler

```go
package handlers

import (
    "encoding/json"
    "net/http"
)

func WriteJSON(w http.ResponseWriter, status int, data any) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    _ = json.NewEncoder(w).Encode(data)
}

func ListThings(w http.ResponseWriter, r *http.Request) {
    WriteJSON(w, 200, map[string]any{"things": []string{}})
}

func GetThing(w http.ResponseWriter, r *http.Request) {
    id := r.PathValue("id")
    WriteJSON(w, 200, map[string]any{"id": id})
}
```

## Dockerfile

```Dockerfile
FROM golang:1.26-alpine AS build
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o app .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /app/app .
ENV PORT=80
EXPOSE 80
CMD ["./app"]
```

## Patterns

- `http.ServeMux` with `"METHOD /path"` and `{id}` path params (Go 1.22+ stdlib router — don't reach for chi/gorilla).
- `r.PathValue("id")` for path params.
- Handlers take `(w, r)`, return nothing. Helpers like `WriteJSON` for response.
- Middleware composes outside `mux`: `handler = middleware.X(handler)`.
- Config from env via a `Load()` function. `.env` loader in `init()` for dev.
- One handler per file, grouped by resource (`things.go`, `users.go`).
- For DB: `gorm.io/gorm` with `postgres` driver in Construct services; raw `database/sql` if asked for "no ORM."

## Don't propose

- `gin`, `echo`, `chi`, `gorilla/mux` — stdlib `ServeMux` is the choice (since 1.22).
- `logrus`, `zap` — `log` from stdlib is fine until proven insufficient.
- Custom error types — return `error` and let callers wrap.
- Generics for what concrete types handle fine.
