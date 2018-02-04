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

	conf := C.Config.Get()

	flag.StringVar(&conf.ArchiveRootName, "root", conf.ArchiveRootName, "name of the archive root")
	flag.StringVar(&conf.DefaultCategory, "category", conf.DefaultCategory, "destination category")
	flag.BoolVar(&conf.ForceRecheck, "recheck", conf.ForceRecheck, "force file checksum recheck")
	flag.BoolVar(&C.Interactive, "interactive", false, "work interactively")
	flag.BoolVar(&conf.Verbose, "verbose", conf.Verbose, "verbose output")
	flag.BoolVar(&conf.CreateMissing, "create-missing", conf.CreateMissing, "create category if not exist")

	flag.Parse()

	C.Target = flag.Arg(0)
	C.Config.Set(conf)
}

func main() {
	// process default configurations
	err := C.ReadConfig(false)
	if err != nil {
		log.Fatalf("Failed to read config: %v", err)
	}

	conf := C.Config.Get()

	// process commandline flags
	initFlags()

	reader := bufio.NewReader(os.Stdin)

	// we need to do this manually for old runtime
	if conf.Verbose {
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
		conf.DefaultCategory, err = reader.ReadString('\n')
		conf.DefaultCategory = strings.TrimRight(conf.DefaultCategory, "\n")
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
		err = R.SyncDirectory(reader, srv, C.Target, conf.DefaultCategory)
	} else {
		fmt.Printf("Syncing file '%s'...\n", C.Target)
		err = R.SyncFile(reader, srv, C.Target, conf.DefaultCategory)
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
