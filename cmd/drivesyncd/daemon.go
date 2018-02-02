package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"os/signal"
	"path/filepath"
	"runtime"
	"time"

	"github.com/radovskyb/watcher"
	"github.com/sevlyar/go-daemon"

	A "github.com/KireinaHoro/DriveSync/auth"
	C "github.com/KireinaHoro/DriveSync/config"
	E "github.com/KireinaHoro/DriveSync/errors"
	R "github.com/KireinaHoro/DriveSync/remote"
)

var targetPath string

func main() {
	srv := A.Authenticate()

	if runtime.GOOS == "darwin" {
		// set up proxy for run in restricted network environment
		proxyUrl, _ := url.Parse("http://127.0.0.1:8001")
		http.DefaultTransport = &http.Transport{Proxy: http.ProxyURL(proxyUrl)}
	}

	// temporary settings for developing
	C.Verbose = true
	C.CreateMissing = true
	C.Category = "Playground"

	// initialize context for forking into background
	ctx := &daemon.Context{
		PidFileName: filepath.Base(os.Args[0]) + ".pid",
		PidFilePerm: 0644,
		LogFileName: filepath.Base(os.Args[0]) + ".log",
		LogFilePerm: 0640,
		WorkDir:     "./",
		Umask:       027,
		Args:        os.Args,
	}

	d, err := ctx.Reborn()
	if err != nil {
		log.Fatalf("E: Unable to fork: %v", err)
	}
	if d != nil {
		return
	}
	defer ctx.Release()

	// we need to do this manually for old runtime
	if C.Verbose {
		log.Println("I: Procs usable:", runtime.NumCPU())
	}
	runtime.GOMAXPROCS(runtime.NumCPU())

	currentUser, err := user.Current()
	if err != nil {
		log.Fatalf("E: Failed to get current user: %v", err)
	}
	targetPath = filepath.Clean(currentUser.HomeDir + "/Documents")

	lock, err := daemon.OpenLockFile(targetPath + "/.drivesync-lock", 0644)
	if err != nil {
		log.Fatalf("E: Failed to open lock file: %v", err)
	} else if err = lock.Lock(); err != nil {
		log.Fatalf("E: Failed to lock (maybe another daemon is running?): %v", err)
	}
	// initialize watcher
	w := watcher.New()
	w.IgnoreHiddenFiles(true)

	done := make(chan struct{})
	go func() {
		defer close(done)

		for {
			select {
			case event := <-w.Event:
				err := R.Sync(nil, srv, event.Path, C.Category)
				if err != nil {
					if _, ok := err.(E.ErrorAlreadySynced); ok {
						log.Printf("I: Already synced: %q", event.Path)
					} else {
						log.Printf("W: Failed to sync %q: %v", event.Path, err)
					}
				}
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

	w.Add(targetPath)
	// we only care about new file events
	w.FilterOps(watcher.Create)

	log.Print("I: Daemon started.")

	f, err := os.Open(targetPath)
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
		itemPath := targetPath + "/" + v
		err := R.Sync(nil, srv, itemPath, C.Category)
		if err != nil {
			if _, ok := err.(E.ErrorAlreadySynced); ok {
				log.Printf("I: Already synced: %q", itemPath)
			} else {
				log.Printf("W: Failed to sync %q: %v", itemPath, err)
			}
		}
	}
	log.Println("I: Sync completed.")

	// start watching
	log.Printf("I: Starting watch of target %q...", targetPath)
	closed := make(chan struct{})

	// handle SIGINT and SIGKILL and close
	c := make(chan os.Signal)
	signal.Notify(c, os.Kill, os.Interrupt)
	go func() {
		<-c
		log.Println("I: Exit signal received. Closing handles...")
		err := lock.Remove()
		if err != nil {
			log.Fatalf("E: Failed to remove lock file: %v", err)
		}
		w.Close()
		<-done
		close(closed)
	}()

	if err := w.Start(100 * time.Millisecond); err != nil {
		log.Fatalf("E: Failed to start watcher: %s", err)
	}

	// run indefinitely before receiving signal to quit
	<-closed
}
