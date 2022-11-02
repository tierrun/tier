package tier_test

import (
	"io"
	"log"
	"net/http"

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
		if err := c.SubscribeNow(r.Context(), org, plan); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})

	log.Fatal(http.ListenAndServe(":https", m))
}

func orgFromSession(r *http.Request) string {
	return "org:example"
}

func convert(temp string) string {
	return "80F"
}
