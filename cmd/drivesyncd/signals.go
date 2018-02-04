package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"syscall"

	"github.com/sevlyar/go-daemon"
)

type signalInfo struct {
	description string
	signal      os.Signal
	handler     func(signal os.Signal) error
}

var (
	// the accepted signal option from commandline
	signal *string
	// the list of available signals
	signals = map[string]signalInfo{
		"quit": {
			"graceful shutdown",
			syscall.SIGQUIT,
			termHandler,
		},
		"stop": {
			"immediate shutdown",
			syscall.SIGTERM,
			termHandler,
		},
		"reload": {
			"reload configuration file",
			syscall.SIGHUP,
			reloadHandler,
		},
	}
)

// parseDescriptions initializes the flag descriptions.
func parseFlags() {
	usage := "send signal to the daemon running"
	for k, v := range signals {
		usage += "\n\t\t" + k + "\t- " + v.description
	}
	signal = flag.String("s", "", usage)
	flag.Parse()
}

// registerSignals registers handlers for signals.
func registerSignals() {
	for k, v := range signals {
		daemon.AddCommand(daemon.StringFlag(signal, k), v.signal, v.handler)
	}
}

// processCommand looks up if the signal given on commandline matches a known one.
// If commandline arguments provided, the daemon will be started;
// otherwise the commandline option will be processed.
func processCommand(ctx *daemon.Context) {
	if len(daemon.ActiveFlags()) > 0 {
		d, err := ctx.Search()
		if err != nil {
			log.Fatalf("E: Unable find the daemon: %v", err)
		}
		err = daemon.SendCommands(d)
		if err != nil {
			log.Fatalf("E: Unable to send signal to the daemon: %v", err)
		}
		os.Exit(0)
	} else if *signal != "" {
		// an unknown signal
		log.Fatalf("E: Unknown signal: %s", *signal)
	}

}

// termHandler handles termination situations, shuts down the program immediately
// or waits for clean-ups based on the signal received.
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

// reloadHandler handles configuration file reload event.
func reloadHandler(sig os.Signal) error {
	// TODO proper implementation
	log.Printf("I: Received signal: %v.", sig.String())
	return nil
}
