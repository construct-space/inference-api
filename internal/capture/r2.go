package capture

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// R2Uploader ships day-files to Cloudflare R2 (or any S3-compatible
// endpoint). Two ladders:
//
//  1. **Finalized uploads** — every `Interval` (default 1h), upload
//     any day-files strictly before today that haven't been uploaded
//     yet. Each finalized upload drops an `<file>.uploaded` marker so
//     restarts don't re-ship. After a successful finalized upload of
//     day D, the matching `<prefix>/<D>.jsonl.partial` key in R2 is
//     deleted — the full file supersedes it.
//
//  2. **Partial uploads** — every `PartialInterval` (default 10m,
//     `0` to disable), upload today's in-progress file under
//     `<prefix>/<today>.jsonl.partial`. No marker, just overwritten
//     each tick. Without this, today's data only lands in R2 after
//     midnight + the next finalized tick — meaning a host loss
//     before midnight loses up to 24h of activity. With the partial
//     ladder, max loss drops to PartialInterval.
type R2Uploader struct {
	Client          *minio.Client
	Bucket          string
	Prefix          string
	Interval        time.Duration
	PartialInterval time.Duration // 0 disables partial uploads
}

type R2Config struct {
	Endpoint        string // e.g. <account-id>.r2.cloudflarestorage.com
	AccessKey       string
	SecretKey       string
	Bucket          string
	Region          string        // R2 uses "auto"
	Prefix          string        // e.g. "inference/" — leading folder in bucket
	Interval        time.Duration // how often to scan + upload finalized files (default 1h)
	PartialInterval time.Duration // how often to upload today's partial (default 10m; 0 disables)
	UseSSL          bool          // default true; R2 requires HTTPS
}

func NewR2Uploader(cfg R2Config) (*R2Uploader, error) {
	if cfg.Endpoint == "" || cfg.Bucket == "" || cfg.AccessKey == "" || cfg.SecretKey == "" {
		return nil, errors.New("r2: endpoint, bucket, access key, and secret key are required")
	}
	region := cfg.Region
	if region == "" {
		region = "auto"
	}
	interval := cfg.Interval
	if interval <= 0 {
		interval = time.Hour
	}
	// Partial interval: explicit 0 disables; negative or unset → 10m.
	partial := cfg.PartialInterval
	if partial < 0 {
		partial = 10 * time.Minute
	}
	cli, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL || cfg.Endpoint != "",
		Region: region,
	})
	if err != nil {
		return nil, fmt.Errorf("r2: %w", err)
	}
	return &R2Uploader{
		Client:          cli,
		Bucket:          cfg.Bucket,
		Prefix:          cfg.Prefix,
		Interval:        interval,
		PartialInterval: partial,
	}, nil
}

// run scans the local log dir on Interval (finalized uploads) and
// PartialInterval (today's in-progress file). Both ladders run in the
// same goroutine via a select so we don't need locking — minio-go
// already serialises HTTP per call internally and FPutObject opens
// the file fresh each time.
func (u *R2Uploader) run(dir string) {
	if u == nil {
		return
	}
	// One immediate pass on startup so a restart after a crash flushes
	// yesterday's file right away. Partial pass too — covers the case
	// where the previous container died mid-day with unshipped writes.
	u.uploadAll(dir)
	u.uploadPartial(dir)

	finalT := time.NewTicker(u.Interval)
	defer finalT.Stop()

	var partialC <-chan time.Time
	if u.PartialInterval > 0 {
		partialT := time.NewTicker(u.PartialInterval)
		defer partialT.Stop()
		partialC = partialT.C
	}

	for {
		select {
		case <-finalT.C:
			u.uploadAll(dir)
		case <-partialC:
			u.uploadPartial(dir)
		}
	}
}

func (u *R2Uploader) uploadAll(dir string) {
	files, err := ListDayFiles(dir)
	if err != nil {
		log.Printf("r2: list day files: %v", err)
		return
	}
	today := CurrentDay()
	for _, path := range files {
		day := filepath.Base(path)
		day = day[:len(day)-len(".jsonl")]
		if day >= today {
			continue // skip current (and impossibly future) day
		}
		marker := path + ".uploaded"
		if _, err := os.Stat(marker); err == nil {
			continue // already uploaded
		}
		if err := u.uploadFile(path); err != nil {
			log.Printf("r2: upload %s: %v", path, err)
			continue
		}
		if err := os.WriteFile(marker, []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o644); err != nil {
			log.Printf("r2: write marker %s: %v", marker, err)
		}
		log.Printf("r2: uploaded %s -> %s/%s", filepath.Base(path), u.Bucket, u.objectKey(filepath.Base(path)))

		// The full finalized object supersedes any in-progress
		// partial we shipped for this day. Best-effort delete; if it
		// fails (key missing, network blip) the stale partial just
		// sits in R2 — not a correctness issue.
		u.deletePartial(day)
	}
}

// uploadPartial ships today's in-progress day-file under a `.partial`
// suffix. Idempotent — each tick overwrites the previous partial in
// R2. No marker file, so restarts re-upload from the latest disk
// content. Open file handles in the capture logger are safe to read
// concurrently: each Append is one syscall (O_APPEND), so a parallel
// FPutObject sees a well-formed JSONL up to whatever was flushed when
// the read began.
func (u *R2Uploader) uploadPartial(dir string) {
	today := CurrentDay()
	path := filepath.Join(dir, today+".jsonl")
	info, err := os.Stat(path)
	if err != nil {
		// File may not exist yet (no traffic today) — that's fine.
		return
	}
	if info.Size() == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	key := u.objectKey(today + ".jsonl.partial")
	if _, err := u.Client.FPutObject(ctx, u.Bucket, key, path, minio.PutObjectOptions{
		ContentType: "application/x-ndjson",
	}); err != nil {
		log.Printf("r2: partial upload %s: %v", filepath.Base(path), err)
		return
	}
	log.Printf("r2: partial uploaded %s (%d bytes) -> %s/%s", filepath.Base(path), info.Size(), u.Bucket, key)
}

func (u *R2Uploader) deletePartial(day string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	key := u.objectKey(day + ".jsonl.partial")
	if err := u.Client.RemoveObject(ctx, u.Bucket, key, minio.RemoveObjectOptions{}); err != nil {
		// Don't error-log a missing-key result — most days won't have
		// a partial (uploadPartial only runs when PartialInterval > 0,
		// and even then only writes if the file exists). Anything
		// genuinely worth noting will surface via the next tick.
		return
	}
	log.Printf("r2: cleared partial for %s (superseded by finalized upload)", day)
}

func (u *R2Uploader) uploadFile(path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	key := u.objectKey(filepath.Base(path))
	_, err := u.Client.FPutObject(ctx, u.Bucket, key, path, minio.PutObjectOptions{
		ContentType: "application/x-ndjson",
	})
	return err
}

func (u *R2Uploader) objectKey(name string) string {
	if u.Prefix == "" {
		return name
	}
	p := u.Prefix
	if p[len(p)-1] != '/' {
		p += "/"
	}
	return p + name
}
