package stripe_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"sync"
	"testing"

	"tier.run/stripe"
	"tier.run/stripe/stroke"
)

func TestDedup(t *testing.T) {
	if os.Getenv("CI") == "" {
		t.Skip("skipping test in local environment")
	}

	ctx := context.Background()
	c := stroke.WithAccount(t, stroke.Client(t))

	create := func(newEmail func() string) {
		t.Helper()
		_, err := stripe.Dedup(ctx, "testkey", t.Logf, func(f stripe.Form) (err error) {
			email := "x@x.com"
			if newEmail != nil {
				email = newEmail()
			}
			t.Logf("email: %s", email)
			f.Set("email", email)
			defer func() {
				t.Logf("err: %v", err)
			}()
			return c.Do(ctx, "POST", "/v1/customers", f, nil)
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	assault := func(email func() string) {
		var g sync.WaitGroup
		const numWorkers = 3
		const numRequestPerWorker = 10
		for i := 0; i < numWorkers; i++ {
			g.Add(1)
			go func() {
				defer g.Done()
				for i := 0; i < numRequestPerWorker; i++ {
					create(email)
				}
			}()
		}
		g.Wait()
	}

	check := func() {
		t.Helper()
		var f stripe.Form
		cc, err := stripe.Slurp[stripe.JustID](ctx, c, "GET", "/v1/customers", f)
		if err != nil {
			t.Fatal(err)
		}
		if len(cc) != 1 {
			t.Errorf("len(cc) = %d; want %d", len(cc), 1)
		}
	}

	create(nil)
	create(nil)
	check()
	assault(nil)
	check()
	assault(func() string {
		return fmt.Sprintf("x@%s.com", randomString())
	})
	check()
}

func randomString() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
