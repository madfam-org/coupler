package main

import (
	"log"
	"net/http"
	"os"

	"github.com/madfam-org/coupler/apps/gateway/internal/auth"
	"github.com/madfam-org/coupler/apps/gateway/internal/registry"
	"github.com/madfam-org/coupler/apps/gateway/internal/server"
)

func main() {
	connectorsDir := server.ConnectorsDir()
	reg, err := registry.LoadFromDir(connectorsDir)
	if err != nil {
		log.Fatalf("load registry: %v", err)
	}
	log.Printf("coupler-gateway: loaded %d tools from %s", len(reg.List()), connectorsDir)

	addr := ":8787"
	if v := os.Getenv("PORT"); v != "" {
		addr = ":" + v
	}

	srv := server.New(reg, auth.NewVerifier(server.AuthRequired()))
	log.Printf("coupler-gateway listening on %s (auth_required=%v)", addr, server.AuthRequired())
	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		log.Fatal(err)
	}
}
