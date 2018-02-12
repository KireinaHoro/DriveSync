package remote

import (
	"bufio"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"

	C "github.com/KireinaHoro/DriveSync/config"
	E "github.com/KireinaHoro/DriveSync/errors"
	U "github.com/KireinaHoro/DriveSync/utils"
)

// yesNoResponse prompts the user to make a choice, returning a boolean.
func yesNoResponse(reader *bufio.Reader, prompt string) bool {
	if !C.Interactive {
		return false
	}
	fmt.Print(prompt + " [Y/n]: ")
	for {
		response, scanErr := reader.ReadString('\n')
		if scanErr != nil {
			log.Fatalf("failed to scan response: %v", scanErr)
		}
		switch strings.ToUpper(strings.TrimRight(response, "\n")) {
		case "":
			fallthrough
		case "Y":
			return true
		case "N":
			return false
		default:
			fmt.Printf("Sorry, response '%v' not understood. ", response)
		}
	}
}

// getLeafFromParent resolves the ID of the requested leaf in given folder ID.
func getLeafFromParent(srv *drive.Service, leafName, parentID string, wantFolder bool) (string, error) {
	var q []string
	q = append(q, fmt.Sprintf("('%s' in parents)", parentID))
	q = append(q, fmt.Sprintf("name='%s'", leafName))
	if wantFolder {
		q = append(q, fmt.Sprintf("mimeType='%s'", C.DriveFolderType))
	} else {
		q = append(q, fmt.Sprintf("mimeType!='%s'", C.DriveFolderType))
	}
	q = append(q, "trashed=false")
	ansList, err := srv.Files.List().Q(strings.Join(q, "and")).Fields("files(id)").Do()
	if err != nil {
		return "", errors.New(fmt.Sprintf("failed to fetch ID of '%s': %v", leafName, err))
	} else if len(ansList.Files) == 0 {
		return "", E.ErrorNotFound(fmt.Sprintf("error: no '%s' in '%s'", leafName, parentID))
	} else if len(ansList.Files) > 1 {
		// return an E.ErrorMultipleResults
		var ret []string
		for _, f := range ansList.Files {
			ret = append(ret, f.Id)
		}
		return "", E.ErrorMultipleResults(ret)
	}
	return ansList.Files[0].Id, nil
}

// getUploadLocation resolves the folder ID of the given category.
func getUploadLocation(reader *bufio.Reader, srv *drive.Service, category string) (string, error) {
	conf := C.Config.Get()
	var err error
	// get the archive root
	if C.ArchiveRootID == "" {
		C.ArchiveRootID, err = getLeafFromParent(srv, conf.ArchiveRootName, "root", true)
		if err != nil {
			if _, ok := err.(E.ErrorNotFound); conf.CreateMissing || (ok &&
				yesNoResponse(reader, "Archive root not found; create it now?")) {
				C.ArchiveRootID, err = createDirectory(srv, conf.ArchiveRootName, "root")
				if err != nil {
					return "", errors.New(fmt.Sprintf("failed to create archive root '%s': %v",
						conf.ArchiveRootName, err))
				}
				if conf.Verbose {
					log.Printf("Created %q.", conf.ArchiveRootName)
				}
			} else {
				return "", errors.New(fmt.Sprintf("failed to retrieve archive root '%s': %v",
					conf.ArchiveRootName, err))
			}
		}
	}
	// get the desired category
	categoryID, ok := C.CategoryIDs.Get(category)
	if !ok {
		categoryID, err = getLeafFromParent(srv, category, C.ArchiveRootID, true)
		if err != nil {
			if _, ok := err.(E.ErrorNotFound); conf.CreateMissing || (ok &&
				yesNoResponse(reader, fmt.Sprintf("Category '%s' not found; create it now?", category))) {
				categoryID, err = createDirectory(srv, category, C.ArchiveRootID)
				if err != nil {
					return "", errors.New(fmt.Sprintf("failed to create category '%s': %v",
						category, err))
				}
				if conf.Verbose {
					log.Printf("Created %q.", category)
				}
			} else {
				return "", errors.New(fmt.Sprintf("failed to retrieve category '%s': %v",
					category, err))
			}
		}
		C.CategoryIDs.Set(category, categoryID)
	}
	//fmt.Printf("Category folder ID: %s\n", categoryID)
	return categoryID, nil
}

// createDirectory creates the directory with name leafName inside directory
// with ID of parentID, returning the ID of the created folder.
//
// Note: the caller shall check if the directory with leafName exists.
// Failing to do so will result in duplicate directories.
func createDirectory(srv *drive.Service, leafName, parentID string) (string, error) {
	createInfo := &drive.File{
		Name:        leafName,
		Description: leafName,
		MimeType:    C.DriveFolderType,
		Parents:     []string{parentID},
	}
	info, err := srv.Files.Create(createInfo).Fields("id").Do()
	if err != nil {
		//return "", errors.New(fmt.Sprintf("failed to create on Drive server: %v", err))
		return "", err
	} else {
		return info.Id, nil
	}
}

// createDirectoryWithCheck checks if the directory with given name exists in given parentID.
// If such folder exists, it will return the ID of the existing folder; otherwise a new one
// will be created.
//
// This function is to eliminate the problem of duplicate files on remote.
func createDirectoryWithCheck(srv *drive.Service, leafName, parentID string) (string, error) {
	fileID, err := getLeafFromParent(srv, leafName, parentID, true)
	if err != nil {
		if _, ok := err.(E.ErrorNotFound); ok {
			return createDirectory(srv, leafName, parentID)
		} else {
			return "", err
		}
	}
	return fileID, nil
}

// createFile creates the file with path leafPath inside directory
// with ID of parentID, uploads the contents of the file, and
// returns the ID of the created file. If C.Config.ForceRecheck is true, it checks if the MD5
// sums of remote and local matches.
//
// Note: the caller shall check if the file with leafName exists.
// Failing to do so will result in duplicate files.
func createFile(srv *drive.Service, leafPath, parentID string) (string, error) {
	conf := C.Config.Get()
	leafName := filepath.Base(leafPath)
	uploadFile, err := os.Open(leafPath)
	if err != nil {
		return "", errors.New(fmt.Sprintf("failed to open file '%s': %v", leafPath, err))
	}
	createInfo := &drive.File{
		Name:        leafName,
		Description: leafName,
		MimeType:    mime.TypeByExtension(filepath.Ext(leafName)),
		Parents:     []string{parentID},
	}
	//info, err := srv.Files.Create(createInfo).Media(uploadFile).Fields("id, md5Checksum").Do()
	// we don't need the md5Checksum field if C.Config.ForceRecheck != true
	intermediateCall := srv.Files.Create(createInfo).Media(uploadFile)
	if conf.ForceRecheck {
		intermediateCall = intermediateCall.Fields("id, md5Checksum")
	} else {
		intermediateCall = intermediateCall.Fields("id")
	}
	retVal, retErr := make(chan *drive.File), make(chan error)
	go func() {
		info, err := intermediateCall.Do()
		retErr <- err
		retVal <- info
	}()
	var info *drive.File
	if conf.ForceRecheck {
		// calculate the MD5 hash of the file
		f, err := os.Open(leafPath)
		if err != nil {
			return "", errors.New(fmt.Sprintf("failed to open file for checksum: %v", err))
		}
		defer f.Close()

		h := md5.New()
		if _, err := io.Copy(h, f); err != nil {
			return "", errors.New(fmt.Sprintf("failed to calculate md5Checksum: %v", err))
		}

		realSum := hex.EncodeToString(h.Sum(nil))
		err = <-retErr
		if err != nil {
			return "", err
		}
		info = <-retVal
		if sum := info.Md5Checksum; sum != realSum {
			return "", E.ErrorChecksumMismatch(fmt.Sprintf(
				"md5Checksum mismatch: remote %s, local %s", sum, realSum))
		}
		//log.Printf("file '%s' has identical remote/local md5Checksum", leafPath)
	} else {
		err := <-retErr
		if err != nil {
			return "", err
		}
		info = <-retVal
	}
	return info.Id, nil
}

// createFileWithCheck checks if the file with given name exists in given parentID.
// If such file exists, it will delete the existing file so that situation of duplicate files
// won't occur.
//
// This function is to eliminate the problem of duplicate files on remote.
func createFileWithCheck(srv *drive.Service, leafPath, parentID string) (string, error) {
	leafName := filepath.Base(leafPath)
	fileID, err := getLeafFromParent(srv, leafName, parentID, false)
	if err == nil {
		err := srv.Files.Delete(fileID).Do()
		if err != nil {
			if C.Verbose {
				// non-critical; log the failure and continue
				log.Printf("Failed to remove existing file %q with ID %q: %v", leafName, fileID, err)
			}
		} else {
			if C.Verbose {
				log.Printf("Removed existing file %q with ID %q.", leafName, fileID)
			}
		}
	} else if err, ok := err.(E.ErrorMultipleResults); ok {
		// multiple results; we should delete all of them
		for _, f := range err {
			e := srv.Files.Delete(f).Do()
			if e != nil {
				if C.Verbose {
					// non-critical; log the failure and continue
					log.Printf("Failed to remove existing file %q with ID %q: %v", leafName, f, err)
				}
			} else {
				if C.Verbose {
					log.Printf("Removed existing file %q with ID %q.", leafName, f)
				}
			}
		}
	}
	return createFile(srv, leafPath, parentID)
}

// withRetry executes fn with retry upon failure in an exponential-backoff manner,
// if the error returned by fn satisfies shouldRetry.
func withRetry(ctx context.Context, fn func() error, shouldRetry func(error) bool) error {
	conf := C.Config.Get()
	l := U.GetLogger(ctx)
	err := fn()
	if shouldRetry(err) {
		if conf.Verbose {
			l.Printf("Need to retry due to: %v", err)
		}
		err = retry(ctx, fn, shouldRetry, C.RetryStartingRate, C.RetryRatio)
	}
	if err != nil {
		err = errors.New(fmt.Sprintf("[Job #%s] retry failed: %v", l, err))
	}
	return err
}

// retry increases retry timeout by factor ratio every time until success, or if an error not
// worth trying occurs.
func retry(ctx context.Context, fn func() error, shouldRetry func(error) bool, currentRate, ratio int) error {
	conf := C.Config.Get()
	l := U.GetLogger(ctx)
	if conf.Verbose {
		l.Printf("Waiting %d second(s) before retrying...", currentRate)
	}
	time.Sleep(time.Duration(currentRate) * time.Second)
	err := fn()
	if shouldRetry(err) {
		err = retry(ctx, fn, shouldRetry, currentRate*ratio, ratio)
	}
	return err
}

// retryIfNeeded takes an error, returning true if it's worth retrying.
func retryIfNeeded(err error) bool {
	if err != nil {
		if realErr, ok := err.(*googleapi.Error); ok {
			// retry on rate limit and all server-side errors
			if realErr.Code == 403 && strings.Contains(
				strings.ToLower(realErr.Message), "rate limit") {
				return true
			} else if 499 < realErr.Code && realErr.Code < 600 {
				return true
			}
		} else if _, ok := err.(E.ErrorChecksumMismatch); ok {
			// retry on checksum mismatch
			return true
		} else if _, ok := err.(net.Error); ok {
			// retry on network problem
			return true
		}
	}
	return false
}
