package main

import (
	"fmt"
	"net"
	"net/http"

	"tier.run/api"
)

func serve(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "listening on %s\n", ln.Addr())

	h := api.New(tc())
	h.Logf = vlogf
	return http.Serve(ln, h)
}
