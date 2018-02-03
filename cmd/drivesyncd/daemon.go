package main

import (
	"fmt"
	"log"
	"os"
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
	w    *watcher.Watcher
	lock *daemon.LockFile
	srv  *drive.Service
	done chan struct{}
)

func main() {
	// read config
	err := C.ReadConfig()
	if err != nil {
		log.Fatalf("E: Failed to read config: %v", err)
	}
	if C.Config.Target == "" {
		log.Fatal(`E: Please set field "target" in configuration file.`)
	}

	srv = A.Authenticate()

	if C.Config.UseProxy {
		C.ProxySetup()
	}

	lock, err = daemon.OpenLockFile(C.Config.Target+"/.drivesync-lock", 0644)
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
		PidFileName: C.Config.PidFile,
		PidFilePerm: 0644,
		LogFileName: C.Config.LogFile,
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
	if C.Config.Verbose {
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
	log.Println("I: Daemon terminated.")
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
