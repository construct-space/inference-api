package main

import (
	"crypto/subtle"
	"log"
	"net/http"
	"time"

	"construct/inference/internal/capture"
	"construct/inference/internal/config"
	"construct/inference/internal/handlers"
	"construct/inference/internal/middleware"
)

func main() {
	cfg := config.Load()
	handlers.Cfg = cfg

	// Capture: best-effort. If R2 creds are missing we still log to disk.
	if cfg.CaptureEnabled {
		var up *capture.R2Uploader
		if cfg.R2Endpoint != "" && cfg.R2Bucket != "" && cfg.R2AccessKey != "" {
			interval, err := time.ParseDuration(cfg.R2UploadEvery)
			if err != nil || interval <= 0 {
				interval = time.Hour
			}
			// Partial interval: parsed separately so "0" can disable it
			// explicitly. Negative parse error → default 10m. Setting
			// R2_PARTIAL_EVERY=0 keeps the legacy "wait until midnight"
			// behaviour for operators that prefer one upload/day.
			var partial time.Duration
			switch raw := cfg.R2PartialEvery; raw {
			case "0", "off", "disabled":
				partial = 0
			case "":
				partial = 10 * time.Minute
			default:
				if d, perr := time.ParseDuration(raw); perr == nil && d > 0 {
					partial = d
				} else {
					partial = 10 * time.Minute
				}
			}
			u, err := capture.NewR2Uploader(capture.R2Config{
				Endpoint:        cfg.R2Endpoint,
				AccessKey:       cfg.R2AccessKey,
				SecretKey:       cfg.R2SecretKey,
				Bucket:          cfg.R2Bucket,
				Region:          cfg.R2Region,
				Prefix:          cfg.R2Prefix,
				Interval:        interval,
				PartialInterval: partial,
				UseSSL:          true,
			})
			if err != nil {
				log.Printf("capture: R2 disabled: %v", err)
			} else {
				up = u
				if partial > 0 {
					log.Printf("capture: R2 uploader on (%s/%s, finalized every %s, partial every %s)", cfg.R2Bucket, cfg.R2Prefix, interval, partial)
				} else {
					log.Printf("capture: R2 uploader on (%s/%s, finalized every %s, partial uploads disabled)", cfg.R2Bucket, cfg.R2Prefix, interval)
				}
			}
		} else {
			log.Printf("capture: R2 creds not set, logging to disk only at %s", cfg.CaptureDir)
		}
		logger, err := capture.New(capture.Options{
			Enabled:  true,
			Dir:      cfg.CaptureDir,
			Uploader: up,
		})
		if err != nil {
			log.Fatalf("capture: %v", err)
		}
		handlers.Capture = logger
	} else {
		log.Printf("capture: disabled (CAPTURE_ENABLED=false)")
	}

	mux := http.NewServeMux()

	// SECURITY: the chat + web endpoints proxy to the platform's paid Together
	// key and offer a body-returning web fetch. They MUST NOT be open to the
	// public. `gate` requires the gateway-forwarded X-Internal-Secret when
	// cfg.RequireAuth is set. It defaults OFF so we don't break the gateway
	// path on a deploy where the gateway doesn't yet forward the secret to
	// inference — set INFERENCE_REQUIRE_AUTH=true once that's confirmed (and
	// ensure llm.lisaos.dev isn't reachable bypassing the gateway).
	gate := func(h http.HandlerFunc) http.HandlerFunc {
		if !cfg.RequireAuth {
			return h
		}
		return func(w http.ResponseWriter, r *http.Request) {
			if cfg.InternalSecret == "" ||
				subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Internal-Secret")), []byte(cfg.InternalSecret)) != 1 {
				handlers.WriteJSON(w, 401, map[string]any{"error": "unauthorized"})
				return
			}
			h(w, r)
		}
	}
	if !cfg.RequireAuth {
		log.Printf("WARNING: inference chat/web endpoints are UNAUTHENTICATED. Set INFERENCE_REQUIRE_AUTH=true once the gateway forwards X-Internal-Secret to close free-inference abuse.")
	}

	mux.HandleFunc("POST /api/chat", gate(handlers.Chat))
	mux.HandleFunc("POST /api/chat/stream", gate(handlers.ChatStream))
	mux.HandleFunc("GET /api/models", handlers.ListModels)

	// OpenAI-compatible aliases at the canonical /v1/* path. Brain (and any
	// other OpenAI SDK) hits `<base>/chat/completions` and `<base>/models`
	// — so the Construct gateway is reachable as just
	// `http://llm.lisaos.dev/v1` from any OpenAI-shaped client.
	// `/v1/chat/completions` always streams: brain always sends
	// `stream:true`, and Together-backed responses are stream-only at the
	// /chat/stream handler. Non-streaming callers should use /api/chat.
	mux.HandleFunc("POST /v1/chat/completions", gate(handlers.ChatStream))
	mux.HandleFunc("GET /v1/models", handlers.ListModels)
	mux.HandleFunc("GET /api/skills", handlers.ListSkills)
	mux.HandleFunc("GET /api/skills/{name}", handlers.GetSkill)
	mux.HandleFunc("GET /api/agents", handlers.ListAgents)
	mux.HandleFunc("GET /api/agents/{name}", handlers.GetAgent)
	mux.HandleFunc("GET /api/tools/builtin", handlers.BuiltinTools)
	mux.HandleFunc("POST /api/web/search", gate(handlers.WebSearch))
	mux.HandleFunc("POST /api/web/fetch", gate(handlers.WebFetch))

	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		handlers.WriteJSON(w, 200, map[string]any{"status": "ok"})
	})
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		handlers.WriteJSON(w, 200, map[string]any{"status": "ok"})
	})
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			handlers.WriteJSON(w, 200, map[string]any{"service": "inference-api", "status": "ok"})
			return
		}
		handlers.WriteJSON(w, 404, map[string]any{"error": "Not found"})
	})

	var handler http.Handler = mux
	handler = middleware.CORS(cfg)(handler)
	handler = middleware.SecurityHeaders(handler)
	handler = middleware.Logger(handler)

	log.Printf("Inference API running on :%s", cfg.Port)
	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      0,
		IdleTimeout:       120 * time.Second,
	}
	log.Fatal(server.ListenAndServe())
}
