package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/exp/slices"
	"tier.run/stripe"
)

func createAccount(ctx context.Context) (a stripe.Account, err error) {
	path, err := cachePath("switch", "a")
	if err != nil {
		return stripe.Account{}, err
	}
	free, err := fs.Glob(os.DirFS(path), "acct_*")
	if err != nil {
		return stripe.Account{}, err
	}
	defer func() {
		if err == nil {
			appendBackgroundTasks("preallocateAccount")
		}
	}()
	vlogf("createAccount: free accounts: %v", free)
	for _, f := range free {
		if err := os.Remove(filepath.Join(path, f)); err != nil {
			// someone beat us, try another
			continue
		}
		a := stripe.Account{ID: f}
		return a, nil
	}
	a, err = createSwitchAccount(ctx, cc().Stripe)
	if err != nil {
		return stripe.Account{}, err
	}
	return a, nil
}

func preallocateAccount() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sc := cc().Stripe.CloneAs("") // use root account
	if sc.Live() {
		// be paranoid
		vlogf("preallocateAccount: skipping in live mode")
		return nil
	}

	a, err := createSwitchAccount(ctx, sc)
	if err != nil {
		return err
	}
	cp, err := cachePath("switch", "a")
	if err != nil {
		return err
	}
	path := filepath.Join(cp, a.ID)
	return os.WriteFile(path, nil, 0600)
}

func createSwitchAccount(ctx context.Context, sc *stripe.Client) (stripe.Account, error) {
	return stripe.CreateAccount(ctx, sc, &stripe.AccountParams{
		BusinessName: randomString("tier.switch."),
		Meta: stripe.Meta{
			"tier.account": "switch",
		},
	})
}

var cleanAccountTypes = []string{"switch", "stroke"}

func cleanAccounts(maxAge time.Duration) error {
	// Cleanup
	// * accounts older than maxAge
	// * accounts marked with tier.account == switch

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute) // this Stripe API is super slow.
	defer cancel()

	path, err := cachePath("switch", "a")
	if err != nil {
		return err
	}

	if cc().Live() {
		return fmt.Errorf("refusing to clean switch accounts in live mode")
	}

	sc := cc().Stripe.CloneAs("") // use root account
	return stripe.CleanAccounts(ctx, sc, func(a stripe.Account) bool {
		vlogf("considering %s for GC; meta=%v", a.ID, a.Meta)
		if !slices.Contains(cleanAccountTypes, a.Meta.Get("tier.account")) {
			return false
		}
		age := time.Since(a.Created)
		if age < maxAge {
			vlogf("account %s is %s old, keeping", a.ID, age)
			return false
		}
		fmt.Fprintln(stdout, a.ID)
		vlogf("account %s is %s old, garbage collecting", a.ID, age)
		// remove from cache or preallocated accounts to avoid using
		_ = os.Remove(filepath.Join(path, a.ID))
		return true
	})
}

func cachePath(parts ...string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(append([]string{home, ".cache", "tier"}, parts...)...)
	if err := os.MkdirAll(path, 0755); err != nil {
		return "", err
	}
	return path, nil
}

// randomString returns a random 16 byte hexencoded string with the given
// prefix.
func randomString(prefix string) string {
	s := make([]byte, 16)
	if _, err := rand.Read(s); err != nil {
		panic(err)
	}
	return prefix + hex.EncodeToString(s)
}
