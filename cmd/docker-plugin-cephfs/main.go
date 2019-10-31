package main

import (
	"docker-plugin-cephfs/pkg/volume"
	"github.com/boltdb/bolt"
	plugin "github.com/docker/go-plugins-helpers/volume"
	"log"
)

const socket = "cephfs"

func main() {
	// Open the my.db data file in your current directory.
	// It will be created if it doesn't exist.
	db, err := bolt.Open(socket+".db", 0600, nil)
	if err != nil {
		log.Fatalf("Could not open the database: %s", err)
	}

	driver := volume.NewDriver(db)
	handler := plugin.NewHandler(driver)
	log.Fatal(handler.ServeUnix(socket, 1))
}
