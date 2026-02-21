package main

import (
	"log"
	"net/http"
	"os"

	"github.com/n0remac/GoDom/auth"
	"github.com/n0remac/GoDom/database"
	. "github.com/n0remac/GoDom/websocket"
)

const (
	webPort = ":8080"
)

func main() {
	// Create a new HTTP server
	mux := http.NewServeMux()
	// create global registry
	globalRegistry := NewCommandRegistry()
	mux.HandleFunc("/ws/hub", CreateWebsocket(globalRegistry))

	// Apps
	Home(mux, globalRegistry)

	// Setup database-backed auth
	if err := os.MkdirAll("data", 0755); err != nil {
		log.Fatalf("failed to create data dir: %v", err)
	}
	ds, err := database.NewSQLiteStoreFromDSN("data/godom.sqlite")
	if err != nil {
		log.Fatalf("failed to open db: %v", err)
	}
	defer ds.Close()
	store := auth.NewSQLiteStore(ds)
	auth.AuthWithStores(mux, globalRegistry, store, store)

	go WsHub.Run()
	log.Printf("Starting server on %s", webPort)
	log.Fatal(http.ListenAndServe(webPort, mux))
}
