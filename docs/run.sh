#!/bin/sh
echo "http://localhost:6060/"
go build -buildmode=plugin -o funcs.so .
go run blake.io/pages/cmd/pages -http=:6060 -p funcs.so -v
