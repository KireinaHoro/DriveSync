package remote

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/net/context"
	"google.golang.org/api/drive/v3"

	C "github.com/KireinaHoro/DriveSync/config"
	E "github.com/KireinaHoro/DriveSync/errors"
	U "github.com/KireinaHoro/DriveSync/utils"
)

// SyncDirectory accepts a path to recursively upload to Google Drive to the specified category,
// returning any error that happens in the process.
//
// It creates a ".sync_finished" mark file in the directory upon finishing, and will return
// an ErrorAlreadySynced directly if that mark is present.
func SyncDirectory(reader *bufio.Reader, srv *drive.Service, path, category string) error {
	// trim the trailing slash
	path = filepath.Clean(path)
	markFilePath := path + "/.sync_finished"
	// check if we have the mark file
	if _, err := os.Stat(markFilePath); !os.IsNotExist(err) {
		return E.ErrorAlreadySynced("folder already synced")
	}
	// parentIDs: key: path; value: parent ID
	parentIDs := make(map[string]string)
	var uploadWg sync.WaitGroup
	err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("Error occured while visiting path %s: %v", path, err)
			return err
		}
		if _, ok := C.IgnoreList[info.Name()]; ok {
			return nil
		}
		parentPath, _ := filepath.Split(path)
		// trim the trailing slash
		parentPath = filepath.Clean(parentPath)
		parentID, ok := parentIDs[parentPath]
		if !ok {
			//log.Println("cache miss: ", parentPath)
			// parent path not present; this is the root of folder to upload
			parentID, err = getUploadLocation(reader, srv, category)
			if err != nil {
				return err
			}
		}
		// prepare for worker goroutine
		ctx := context.Background()
		// routineID for logging
		routineID := fmt.Sprintf("%05x", rand.Uint32()%0xfffff)
		if info.IsDir() {
			// we won't be checking if folder exists (real filesystems won't have duplicate files)
			id := new(string)
			err := withRetry(U.CtxWithLoggerID(ctx, routineID), func() error {
				var err error
				*id, err = createDirectory(srv, info.Name(), parentID)
				return err
			}, retryIfNeeded)
			if err != nil {
				return err
			} else {
				// record parent entry
				parentIDs[path] = *id
				//log.Println("added parent map entry: ", path, id)
				if C.Verbose {
					log.Printf("Created directory '%s' (from %s) with ID %s", info.Name(), path, *id)
				}
			}
		} else {
			uploadWg.Add(1)
			go func() {
				defer uploadWg.Done()
				// we won't be checking if file exists (real filesystems won't have duplicate files)
				id := new(string)
				err := withRetry(U.CtxWithLoggerID(ctx, routineID), func() error {
					var err error
					*id, err = createFile(srv, path, parentID)
					return err
				}, retryIfNeeded)
				if err != nil {
					log.Fatalf("Unexpected error while uploading file '%s' (from %s): %v", info.Name(), path, err)
				}
				if C.Verbose {
					log.Printf("Uploaded file '%s' (from %s) with ID %s", info.Name(), path, *id)
				}
			}()
		}
		return nil
	})
	// wait for all goroutines to finish working
	uploadWg.Wait()
	if err != nil {
		return errors.New(fmt.Sprintf("failed to sync directory: %v", err))
	}
	// mark the folder as already synced
	_, err = os.Create(markFilePath)
	if err != nil {
		return E.ErrorSetMarkFailed(err.Error())
	}
	if C.Verbose {
		log.Printf("Sync completed for directory '%s' into category %s.", path, category)
	}
	return nil
}

// SyncFile accepts a path to upload to Google Drive to the specified category,
// returning any error that happens in the process.
//
// It creates a (".sync_finished-"+filepath.Base(path)) mark file in the directory containing
// the file, and will return an ErrorAlreadySynced directly if that mark is present.
func SyncFile(reader *bufio.Reader, srv *drive.Service, path, category string) error {
	// clean the path to avoid surprises
	path = filepath.Clean(path)
	parentPath, basename := filepath.Split(path)
	markFilePath := parentPath + ".sync_finished-" + basename
	// check if we have the mark file
	if _, err := os.Stat(markFilePath); !os.IsNotExist(err) {
		return E.ErrorAlreadySynced("file already synced")
	}
	parentID, err := getUploadLocation(reader, srv, category)
	if err != nil {
		return err
	}
	ctx := context.Background()
	routineID := fmt.Sprintf("%05x", rand.Uint32()%0xfffff)
	id := new(string)
	err = withRetry(U.CtxWithLoggerID(ctx, routineID), func() error {
		var err error
		*id, err = createFile(srv, path, parentID)
		return err
	}, retryIfNeeded)
	if err != nil {
		log.Fatalf("Unexpected error while uploading file '%s' (from %s): %v", basename, path, err)
	}
	if C.Verbose {
		log.Printf("Uploaded file '%s' (from %s) with ID %s", basename, path, *id)
	}
	_, err = os.Create(markFilePath)
	if err != nil {
		return E.ErrorSetMarkFailed(err.Error())
	}
	if C.Verbose {
		log.Printf("Sync completed for file '%s' into category %s.", path, category)
	}
	return nil
}
