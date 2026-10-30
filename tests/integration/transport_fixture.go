//go:build integration

package main

import (
	"context"
	"log"
	"net"
	"net/http"
)

func main() {
	var c net.ListenConfig
	c.SetMultipathTCP(true)
	l, e := c.Listen(context.Background(), "tcp", ":18080")
	if e != nil {
		log.Fatal(e)
	}
	log.Fatal(http.Serve(l, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("healthy")) })))
}
