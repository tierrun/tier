package stroke

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"time"

	"tier.run/stripe"
)

// Backfiller returns a function that can be used to backfill the account pool,
// or nil if the current process is the backfiller.
func Backfiller(c *stripe.Client) func() {
	child := os.Getenv("_TIER_BACKGROUND_ACCOUNT_BACKFILL_CHILD") != ""
	if child {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		backfillCache(ctx, c)
		return nil
	} else {
		return func() {
			// TODO(bmizerany): update after duration
			if !touchAfter(cachePath(c, "touch"), 0) {
				return
			}
			exe, err := os.Executable()
			if err != nil {
				panic(err)
			}
			_, err = os.StartProcess(exe, []string{exe}, &os.ProcAttr{
				Env: append(os.Environ(), "_TIER_BACKGROUND_ACCOUNT_BACKFILL_CHILD=1"),
			})
			if err != nil {
				panic(err)
			}
		}
	}
}

// NextAccount returns the next available account from the pool, or the empty
// string if none are available. Any account taken from the pool is immediately
// marked as used before returning. Backfiller processes will replace used
// accounts with new ones.
func NextAccount(c *stripe.Client) string {
	dir := cachePath(c, "q")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		panic(err)
	}
	matches, err := fs.Glob(os.DirFS(dir), "acct_*")
	if err != nil {
		panic(err)
	}
	for _, id := range matches {
		name := filepath.Join(dir, id)
		tomb := filepath.Join(filepath.Dir(name), "tomb_"+id)
		if err := os.Rename(name, tomb); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			panic(err)
		}
		return id
	}
	// no accounts available, leave a tomb so we know to create one when
	// backfilling
	touchAfter(filepath.Join(dir, "tomb_"+time.Now().Format(time.RFC3339Nano)), 0)
	return ""
}

func backfillCache(ctx context.Context, c *stripe.Client) {
	dir := cachePath(c, "q")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		panic(err)
	}
	tombs, err := fs.Glob(os.DirFS(dir), "tomb_*")
	if err != nil {
		panic(err)
	}
	for _, tomb := range tombs {
		name := filepath.Join(dir, tomb)
		err := os.Remove(name)
		if err != nil {
			// lost the race to a peer process
			log.Printf("failed to remove tomb %q: %v", tomb, err)
			continue
		}
		a, err := stripe.CreateAccount(ctx, c)
		if err != nil {
			log.Println("failed to create account:", err)
			continue
		}
		name = filepath.Join(dir, a.ID)
		if err := os.WriteFile(name, nil, 0o644); err != nil {
			log.Printf("failed to write account %s: %v", name, err)
			continue
		}
		log.Println("created account", name)
	}
	if len(tombs) == 0 {
		log.Println("no accounts to backfill")
	}
}

func touchAfter(path string, d time.Duration) bool {
	if time.Since(lastTouched(path)) > d {
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			panic(err)
		}
		return true
	}
	return false
}

func lastTouched(path string) time.Time {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return time.Unix(0, 0)
	}
	if err != nil {
		panic(err)
	}
	return info.ModTime()
}

func sum(c *stripe.Client) string {
	h := sha256.New()
	io.WriteString(h, c.APIKey)
	return hex.EncodeToString(h.Sum(nil))
}

func cachePath(c *stripe.Client, tail string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	return filepath.Join(home, ".cache", "stroke", sum(c), "accounts", tail)
}
