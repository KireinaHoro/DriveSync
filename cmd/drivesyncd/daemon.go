package main

import (
	"log"
	"os"
	"runtime"

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
	// parse flag
	parseFlags()
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

	registerSignals()

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

	processCommand(ctx)

	// we're launched as a daemon
	lock, err = daemon.OpenLockFile(C.Config.Target+"/.drivesync-lock", 0644)
	defer lock.Remove()
	if err != nil {
		log.Fatalf("E: Failed to open lock file: %v", err)
	} else if err = lock.Lock(); err != nil {
		log.Fatalf("E: Failed to lock (maybe another daemon is running?): %v", err)
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
