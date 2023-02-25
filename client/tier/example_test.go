package tier_test

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"tier.run/client/tier"
)

func ExampleClient() {
	c := &tier.Client{}

	m := http.NewServeMux()
	m.HandleFunc("/convert", func(w http.ResponseWriter, r *http.Request) {
		org := orgFromSession(r)
		ans := c.Can(r.Context(), org, "feature:convert")
		if ans.OK() {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		defer ans.Report()
		temp := convert(r.FormValue("temp"))
		io.WriteString(w, temp)
	})

	m.HandleFunc("/subscribe", func(w http.ResponseWriter, r *http.Request) {
		org := orgFromSession(r)
		plan := r.FormValue("plan")
		if err := c.Subscribe(r.Context(), org, plan); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	})

	log.Fatal(http.ListenAndServe(":https", m))
}

func ExampleClient_Can_basic() {
	c := &tier.Client{}

	// Check if the user can convert a temperature.
	if c.Can(context.Background(), "org:example", "feature:convert").OK() {
		// The user can convert a temperature.
	} else {
		// The user cannot convert a temperature.
	}
}

func ExampleClient_Can_report() {
	c := &tier.Client{}
	ans := c.Can(context.Background(), "org:example", "feature:convert")
	if !ans.OK() {
		// The user cannot convert a temperature.
		return
	}
	defer ans.Report() // report consumption after the conversion
	fmt.Println(convert(readInput()))
}

func ExampleClient_WithClock_testClocks() {
	c, err := tier.FromEnv()
	if err != nil {
		panic(err)
	}

	now := time.Now()

	ctx, err := c.WithClock(context.Background(), "testName", now)
	if err != nil {
		panic(err)
	}

	// Use ctx with other Client methods

	// This creates the customer and subscription using the clock.
	_ = c.Subscribe(ctx, "org:example", "plan:free@0")

	// Advance the clock by 24 hours, and then report usage.
	_ = c.Advance(ctx, now.Add(24*time.Hour))

	_ = c.Report(ctx, "org:example", "feature:bandwidth", 1000)
}

func orgFromSession(r *http.Request) string {
	return "org:example"
}

func convert(temp string) string {
	return "80F"
}

func readInput() string {
	return "30C"
}
