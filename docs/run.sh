#!/bin/sh
# TODO: fix the need to navigate to /public in pages.
echo "http://localhost:6060/public/"
go build -buildmode=plugin -o funcs.so .
go run blake.io/pages/cmd/pages@latest -http=:6060 -p funcs.so
