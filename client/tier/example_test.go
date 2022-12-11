package tier_test

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"

	"tier.run/client/tier"
)

func ExampleClient() {
	c := tier.NewTierSidecarClient("http://localhost:8080")

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
	c := tier.NewTierSidecarClient("http://localhost:8080")

	// Check if the user can convert a temperature.
	if c.Can(context.Background(), "org:example", "feature:convert").OK() {
		// The user can convert a temperature.
	} else {
		// The user cannot convert a temperature.
	}
}

func ExampleClient_Can_report() {
	c := tier.NewTierSidecarClient("http://localhost:8080")
	ans := c.Can(context.Background(), "org:example", "feature:convert")
	if !ans.OK() {
		// The user cannot convert a temperature.
		return
	}
	defer ans.Report() // report consumption after the conversion
	fmt.Println(convert(readInput()))
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
