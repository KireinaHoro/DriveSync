package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
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

	flag.StringVar(&C.Config.ArchiveRootName, "root", "archive", "name of the archive root")
	flag.StringVar(&C.Config.DefaultCategory, "category", "Uncategorized", "destination category")
	flag.BoolVar(&C.Config.ForceRecheck, "recheck", true, "force file checksum recheck")
	flag.BoolVar(&C.Interactive, "interactive", false, "work interactively")
	flag.BoolVar(&C.Config.Verbose, "verbose", true, "verbose output")
	flag.BoolVar(&C.Config.CreateMissing, "create-missing", false, "create category if not exist")

	flag.Parse()

	C.Target = flag.Arg(0)
}

func main() {
	// process default configurations
	err := C.ReadConfig()
	if err != nil {
		log.Fatalf("Failed to read config: %v", err)
	}

	// process commandline flags
	initFlags()

	reader := bufio.NewReader(os.Stdin)

	if C.Config.UseProxy {
		C.ProxySetup()
	}

	// we need to do this manually for old runtime
	if C.Config.Verbose {
		fmt.Println("Procs usable:", runtime.NumCPU())
	}
	runtime.GOMAXPROCS(runtime.NumCPU())

	// authenticate to Google Drive server to get *drive.Service
	srv := A.Authenticate()

	var info os.FileInfo

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
		C.Config.DefaultCategory, err = reader.ReadString('\n')
		C.Config.DefaultCategory = strings.TrimRight(C.Config.DefaultCategory, "\n")
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
		err = R.SyncDirectory(reader, srv, C.Target, C.Config.DefaultCategory)
	} else {
		fmt.Printf("Syncing file '%s'...\n", C.Target)
		err = R.SyncFile(reader, srv, C.Target, C.Config.DefaultCategory)
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
