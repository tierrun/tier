//go:generate go build -buildmode=plugin -o funcs.so .
//go:generate go run blake.io/pages/cmd/pages -rm -p funcs.so

package main

import (
	"html/template"

	"github.com/tailscale/hujson"
	"tier.run/pricing"
)

var (
	Data  any
	Funcs = map[string]any{
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
	}
)
