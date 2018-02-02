package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/pkg/errors"
	"github.com/radovskyb/watcher"
	"github.com/sevlyar/go-daemon"
	"google.golang.org/api/drive/v3"

	A "github.com/KireinaHoro/DriveSync/auth"
	C "github.com/KireinaHoro/DriveSync/config"
)

var (
	targetPath string
	w          *watcher.Watcher
	lock       *daemon.LockFile
	srv        *drive.Service
	done       chan struct{}
)

func main() {
	srv = A.Authenticate()

	if runtime.GOOS == "darwin" {
		// set up proxy for run in restricted network environment
		proxyUrl, _ := url.Parse("http://127.0.0.1:8001")
		http.DefaultTransport = &http.Transport{Proxy: http.ProxyURL(proxyUrl)}
	}

	// temporary settings for developing
	C.Verbose = true
	C.CreateMissing = true
	C.Category = "Playground"

	currentUser, err := user.Current()
	if err != nil {
		log.Fatalf("E: Failed to get current user: %v", err)
	}
	targetPath = filepath.Clean(currentUser.HomeDir + "/Documents")

	lock, err = daemon.OpenLockFile(targetPath+"/.drivesync-lock", 0644)
	if err != nil {
		log.Fatalf("E: Failed to open lock file: %v", err)
	} else if err = lock.Lock(); err != nil {
		log.Fatalf("E: Failed to lock (maybe another daemon is running?): %v", err)
	}

	// register handlers for signals
	daemon.AddCommand(nil, syscall.SIGQUIT, termHandler)
	daemon.AddCommand(nil, syscall.SIGTERM, termHandler)
	daemon.AddCommand(nil, syscall.SIGHUP, reloadHandler)

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

	log.Println("----------------------------")
	// we need to do this manually for old runtime
	if C.Verbose {
		log.Println("I: Procs usable:", runtime.NumCPU())
	}
	runtime.GOMAXPROCS(runtime.NumCPU())

	// run indefinitely before receiving signal to quit
	done = make(chan struct{})
	go worker()

	err = daemon.ServeSignals()
	if err != nil {
		log.Printf("E: %v", err)
	}
	log.Println(" I: Daemon terminated.")
}

func termHandler(sig os.Signal) error {
	logMessage := fmt.Sprintf("I: Received signal: %v.", sig.String())
	if sig == syscall.SIGQUIT {
		logMessage += " Closing handles..."
	} else {
		logMessage += " Exiting now..."
	}
	log.Print(logMessage)
	err := lock.Remove()
	if err != nil {
		return errors.New(fmt.Sprintf("failed to remove lock file: %v", err))
	}
	w.Close()
	// wait for things to be completed
	if sig == syscall.SIGQUIT {
		<-done
	}
	return daemon.ErrStop
}

func reloadHandler(sig os.Signal) error {
	log.Printf("I: Received signal: %v.", sig.String())
	return nil
}
