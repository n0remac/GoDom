package main

import (
	"log"
	"net/http"
	"os"

	"github.com/n0remac/GoDom/admin"
	"github.com/n0remac/GoDom/auth"
	"github.com/n0remac/GoDom/database"
	. "github.com/n0remac/GoDom/websocket"
)

const (
	webPort = ":8080"
)

func main() {
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

	handled, message, err := admin.HandleCLI(store, os.Args[1:], os.Args[0])
	if err != nil {
		log.Fatal(err)
	}
	if message != "" {
		log.Print(message)
	}
	if handled {
		return
	}

	// Create a new HTTP server
	mux := http.NewServeMux()
	imageHandler, err := ds.ImageHandler("/images/")
	if err != nil {
		log.Fatalf("failed to configure image handler: %v", err)
	}
	mux.Handle("/images/", imageHandler)

	// create global registry
	globalRegistry := NewCommandRegistry()
	mux.HandleFunc("/ws/hub", CreateWebsocket(globalRegistry))

	// Apps
	Home(mux, globalRegistry, ds)

	authApp := auth.AuthWithStores(mux, globalRegistry, store, store)
	admin.Mount(mux, authApp)

	warning, err := admin.MissingAdminWarning(store, os.Args[0])
	if err != nil {
		log.Printf("warning: unable to check admin configuration: %v", err)
	} else if warning != "" {
		log.Print(warning)
	}

	go WsHub.Run()
	log.Printf("Starting server on %s", webPort)
	log.Fatal(http.ListenAndServe(webPort, mux))
}
