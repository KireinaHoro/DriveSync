package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/radovskyb/watcher"

	C "github.com/KireinaHoro/DriveSync/config"
	E "github.com/KireinaHoro/DriveSync/errors"
	R "github.com/KireinaHoro/DriveSync/remote"
)

func worker() {
	conf := C.Config.Get()
	// initialize watcher
	w = watcher.New()
	w.IgnoreHiddenFiles(true)

	go func() {
		defer close(done)

		for {
			select {
			case event := <-w.Event:
				go func() {
					err := R.SyncWithGuess(nil, srv, event.Path, C.NoGuessing)
					if err != nil {
						if _, ok := err.(E.ErrorAlreadySynced); ok {
							log.Printf("I: Already synced: %q", event.Path)
						} else {
							log.Printf("W: Failed to sync %q: %v", event.Path, err)
						}
					}
				}()
			case err := <-w.Error:
				if err == watcher.ErrWatchedFileDeleted {
					fmt.Println(err)
					continue
				}
				log.Fatalf("E: Error occurred while watching: %v", err)
			case <-w.Closed:
				return
			}
		}
	}()

	w.Add(conf.Target)
	// we only care about new file events
	w.FilterOps(watcher.Create)

	log.Print("I: Daemon started.")

	f, err := os.Open(conf.Target)
	if err != nil {
		log.Fatalf("E: Failed to open target: %v", err)
	}
	if fi, err := f.Stat(); err != nil {
		log.Fatalf("E: Failed to stat target: %v", err)
	} else if !fi.IsDir() {
		log.Fatalf("E: Target %q is not a directory", fi.Name())
	}

	// sync the target first
	log.Println("I: Syncing files/folders...")
	children, err := f.Readdirnames(-1)
	for _, v := range children {
		itemPath := conf.Target + "/" + v
		go func() {
			log.Printf("I: Syncing %q...", itemPath)
			err := R.SyncWithGuess(nil, srv, itemPath, C.NoGuessing)
			if err != nil {
				if _, ok := err.(E.ErrorAlreadySynced); ok {
					log.Printf("I: Already synced: %q", itemPath)
				} else {
					log.Printf("W: Failed to sync %q: %v", itemPath, err)
				}
			}
		}()
	}
	log.Println("I: Initial scan completed.")

	// start watching
	log.Printf("I: Starting watch of target %q...", conf.Target)

	interval, _ := time.ParseDuration(conf.ScanInterval)
	if err := w.Start(interval); err != nil {
		log.Fatalf("E: Failed to start watcher: %s", err)
	}
}
