package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	A "github.com/KireinaHoro/DriveSync/auth"
	C "github.com/KireinaHoro/DriveSync/config"
	E "github.com/KireinaHoro/DriveSync/errors"
	R "github.com/KireinaHoro/DriveSync/remote"
)

// initFlags initializes the command-line arguments.
func initFlags() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "\nUsage: %s [options] ( <target> || -interactive )\n\n",
			filepath.Base(os.Args[0]))
		flag.PrintDefaults()
	}

	flag.StringVar(&C.ArchiveRootName, "root", "archive", "name of the archive root")
	flag.StringVar(&C.Category, "category", "Uncategorized", "destination category")
	flag.BoolVar(&C.ForceRecheck, "recheck", true, "force file checksum recheck")
	flag.BoolVar(&C.Interactive, "interactive", false, "work interactively")
	flag.BoolVar(&C.Verbose, "verbose", false, "verbose output")
	flag.BoolVar(&C.CreateMissing, "create-missing", false, "create category if not exist")

	flag.Parse()

	C.Target = flag.Arg(0)
}

func main() {
	initFlags()

	reader := bufio.NewReader(os.Stdin)

	if runtime.GOOS == "darwin" {
		// set up proxy for run in restricted network environment
		proxyUrl, _ := url.Parse("http://127.0.0.1:8001")
		http.DefaultTransport = &http.Transport{Proxy: http.ProxyURL(proxyUrl)}
	}

	// we need to do this manually for old runtime
	if C.Verbose {
		fmt.Println("Procs usable:", runtime.NumCPU())
	}
	runtime.GOMAXPROCS(runtime.NumCPU())

	// authenticate to Google Drive server to get *drive.Service
	srv := A.Authenticate()

	var (
		info os.FileInfo
		err  error
	)

	if C.Interactive {
		fmt.Print("Enter target to sync, in absolute path: ")
		C.Target, err = reader.ReadString('\n')
		C.Target = strings.TrimRight(C.Target, "\n")
		if err != nil {
			log.Fatalf("Failed to scan: %v", err)
		}
		info, err = os.Stat(C.Target)
		if err != nil {
			log.Fatalf("Failed to stat target '%s': %v", C.Target, err)
		}
		fmt.Print("Enter desired category: ")
		C.Category, err = reader.ReadString('\n')
		C.Category = strings.TrimRight(C.Category, "\n")
		if err != nil {
			log.Fatalf("Failed to scan: %v", err)
		}
	} else {
		if C.Target == "" {
			fmt.Fprintln(os.Stderr, "Please specify target properly.")
			flag.Usage()
			os.Exit(1)
		}
		if C.Target[0] != '/' {
			pwd, err := os.Getwd()
			if err != nil {
				log.Fatalf("Failed to get current working directory: %v", err)
			}
			C.Target = filepath.Clean(pwd + "/" + C.Target)
		}
		info, err = os.Stat(C.Target)
		if err != nil {
			log.Fatalf("Failed to stat target '%s': %v", C.Target, err)
		}
	}
	if info.IsDir() {
		fmt.Printf("Syncing directory '%s'...\n", C.Target)
		err = R.SyncDirectory(reader, srv, C.Target, C.Category)
	} else {
		fmt.Printf("Syncing file '%s'...\n", C.Target)
		err = R.SyncFile(reader, srv, C.Target, C.Category)
	}
	if err != nil {
		if _, ok := err.(E.ErrorSetMarkFailed); ok {
			log.Printf("Sync succeeded, yet failed to set sync mark: %v", err)
		} else {
			log.Fatalf("Failed to sync '%s': %v", C.Target, err)
		}
	}
	fmt.Println("Sync succeeded.")
}
