//go:generate go build -buildmode=plugin -o funcs.so .
//go:generate go run blake.io/pages/cmd/pages -rm -p funcs.so

package main

import (
	"bytes"
	"html/template"
	"time"

	"blake.io/pages"
	"github.com/tailscale/hujson"
	"tier.run/pricing"
)

type tick struct{}

var (
	Data  any
	Funcs = template.FuncMap{
		// model is a template function that validates, formats, and
		// returns a pricing model json.
		"model": func(s string) (template.HTML, error) {
			_, err := pricing.Unmarshal([]byte(s))
			if err != nil {
				return "", err
			}
			b, err := hujson.Format([]byte(s))
			if err != nil {
				return "", err
			}
			return template.HTML(b), nil
		},

		// hujson is a template function that formats HuJSON.
		"hujson": func(s string) (template.HTML, error) {
			b, err := hujson.Format([]byte(s))
			if err != nil {
				return "", err
			}
			return template.HTML(b), nil
		},

		// now is time.Now
		"now": time.Now,

		// For use like {{ range loop 100 }} tick {{ end }} which can be
		// used for generating n t
		"loop": func(n int) []tick {
			return make([]tick, n)
		},

		// markdown converts the markdown in s to HTML.
		//
		// NOTE: This is a hack but works for now. Pages will have an
		// official way to render markdown for traits in layouts.
		"markdown": func(b []byte) (template.HTML, error) {
			var buf bytes.Buffer
			if err := pages.DefaultMarkdown(b, &buf); err != nil {
				return "", err
			}
			return template.HTML(buf.String()), nil
		},
	}
)
