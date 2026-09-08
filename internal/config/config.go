package config

import (
	"bufio"
	"os"
	"strings"
)

func init() {
	loadEnvFile(".env")
}

func loadEnvFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if idx := strings.Index(line, "="); idx > 0 {
			k := strings.TrimSpace(line[:idx])
			v := strings.TrimSpace(line[idx+1:])
			if os.Getenv(k) == "" {
				_ = os.Setenv(k, v)
			}
		}
	}
}

type Config struct {
	Port           string
	AppURL         string
	AllowedOrigins []string

	TogetherAPIKey  string
	TogetherBaseURL string

	// Default model used when caller doesn't specify one.
	DefaultModel string

	// Hard ceiling on output tokens per request (safety net for cost).
	MaxOutputTokens int

	// Directory holding skill markdown files. Loaded on demand by handlers.
	SkillsDir string

	// Directory holding agent markdown files (frontmatter + identity body).
	AgentsDir string

	// Capture: persist every request/response to JSONL for later
	// finetune + analysis. R2 (or any S3-compatible) target is optional;
	// without it the JSONL stays local.
	CaptureEnabled bool
	CaptureDir     string
	R2Endpoint     string
	R2Bucket       string
	R2AccessKey    string
	R2SecretKey    string
	R2Region       string
	R2Prefix       string
	R2UploadEvery  string // human duration, e.g. "1h", "15m" — finalized files
	R2PartialEvery string // human duration, e.g. "10m"; "0" disables today's partial upload

	InternalSecret string

	// RequireAuth, when true, gates the chat + web endpoints on a valid
	// X-Internal-Secret (the gateway forwards it after authenticating the
	// user). Default false to avoid breaking the gateway path on deploys
	// where the gateway doesn't yet forward the secret to inference — flip
	// INFERENCE_REQUIRE_AUTH=true once that's confirmed. See SECURITY note
	// in main.go.
	RequireAuth bool
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// envAny returns the first non-empty env value among `keys`. Lets the
// config tolerate alias names without breaking existing deploys —
// Cloudflare's dashboard variously labels R2 credentials as
// "Access Key" / "Secret Key" or "API Key" / "API Secret", so the same
// pair shows up under both names in the wild.
func envAny(fallback string, keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n := 0
	for _, c := range v {
		if c < '0' || c > '9' {
			return fallback
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := parts[:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func Load() *Config {
	return &Config{
		Port:            env("PORT", "4300"),
		AppURL:          env("APP_URL", "http://localhost:4300"),
		AllowedOrigins:  append([]string{env("APP_URL", "http://localhost:4300"), "http://localhost:5173", "http://localhost:5174"}, splitCSV(env("ALLOWED_ORIGINS", ""))...),
		TogetherAPIKey:  env("TOGETHER_API_KEY", ""),
		TogetherBaseURL: env("TOGETHER_BASE_URL", "https://api.together.xyz/v1"),
		DefaultModel:    env("DEFAULT_MODEL", "Qwen/Qwen3-Coder-480B-A35B-Instruct-FP8"),
		MaxOutputTokens: envInt("MAX_OUTPUT_TOKENS", 4096),
		SkillsDir:       env("SKILLS_DIR", "./skills"),
		AgentsDir:       env("AGENTS_DIR", "./agents"),
		CaptureEnabled:  env("CAPTURE_ENABLED", "true") == "true",
		CaptureDir:      env("CAPTURE_DIR", "./logs/io"),
		R2Endpoint:      env("R2_ENDPOINT", ""),
		R2Bucket:        env("R2_BUCKET", ""),
		R2AccessKey:     envAny("", "R2_ACCESS_KEY", "R2_API_KEY", "R2_ACCESS_KEY_ID"),
		R2SecretKey:     envAny("", "R2_SECRET_KEY", "R2_API_SECRET", "R2_SECRET_ACCESS_KEY"),
		R2Region:        env("R2_REGION", "auto"),
		R2Prefix:        env("R2_PREFIX", "inference/"),
		R2UploadEvery:   env("R2_UPLOAD_EVERY", "1h"),
		R2PartialEvery:  env("R2_PARTIAL_EVERY", "10m"),
		InternalSecret:  env("INTERNAL_SHARED_SECRET", ""),
		RequireAuth:     env("INFERENCE_REQUIRE_AUTH", "") == "true",
	}
}
