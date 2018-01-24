package main

import (
	"bufio"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
)

var (
	ignoreList = map[string]struct{}{
		".DS_Store":      {},
		".localized":     {},
		".idea":          {},
		".sync_finished": {},
	}
	reader        = bufio.NewReader(os.Stdin)
	archiveRootID string
	categoryIDs   = &SafeMap{v: make(map[string]string)}
)

const (
	archiveRootName   = "archive"
	driveFolderType   = "application/vnd.google-apps.folder"
	forceRecheck      = true
	retryRatio        = 2
	retryStartingRate = 1
)

// SafeMap is a concurrent-safe map[string]string
type SafeMap struct {
	v map[string]string
	m sync.Mutex
}

func (r *SafeMap) Get(key string) (value string, ok bool) {
	r.m.Lock()
	defer r.m.Unlock()
	value, ok = r.v[key]
	return
}

func (r *SafeMap) Set(key, value string) {
	r.m.Lock()
	defer r.m.Unlock()
	r.v[key] = value
}

type ErrorNotFound string

func (r ErrorNotFound) Error() string {
	return string(r)
}

type ErrorAlreadySynced string

func (r ErrorAlreadySynced) Error() string {
	return string(r)
}

type ErrorChecksumMismatch string

func (r ErrorChecksumMismatch) Error() string {
	return string(r)
}

type logger string

const loggerID = "logger_id"

func (l logger) Printf(format string, v ...interface{}) {
	log.Printf("[Job #%s] %s", l, fmt.Sprintf(format, v...))
}

func (l logger) Println(v ...interface{}) {
	log.Printf("[Job #%s] %s", l, fmt.Sprintln(v...))
}

// ctxWithLoggerID generates a new context with loggerID for logging.
func ctxWithLoggerID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, loggerID, id)
}

// getLogger extracts logger from context.
func getLogger(ctx context.Context) logger {
	return logger(ctx.Value(loggerID).(string))
}

// getClient uses a Context and Config to retrieve a Token
// then generate a Client. It returns the generated Client.
func getClient(ctx context.Context, config *oauth2.Config) *http.Client {
	cacheFile, err := tokenCacheFile()
	if err != nil {
		log.Fatalf("Unable to get path to cached credential file: %v", err)
	}
	tok, err := tokenFromFile(cacheFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(cacheFile, tok)
	}
	return config.Client(ctx, tok)
}

// getTokenFromWeb uses Config to request a Token.
// It returns the retrieved Token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var code string
	if _, err := fmt.Scan(&code); err != nil {
		log.Fatalf("Unable to read authorization code: %v", err)
	}

	tok, err := config.Exchange(context.Background(), code)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web: %v", err)
	}
	return tok
}

// tokenCacheFile generates credential file path/filename.
// It returns the generated credential path/filename.
func tokenCacheFile() (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", err
	}
	tokenCacheDir := filepath.Join(usr.HomeDir, ".credentials")
	os.MkdirAll(tokenCacheDir, 0700)
	return filepath.Join(tokenCacheDir,
		url.QueryEscape("drive-go-quickstart.json")), err
}

// tokenFromFile retrieves a Token from a given file path.
// It returns the retrieved Token and any read error encountered.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	t := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(t)
	return t, err
}

// saveToken uses a file path to create a file and store the
// token in it.
func saveToken(file string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", file)
	f, err := os.OpenFile(file, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

// Authenticate authenticates the application with Google Drive
// server and returns a *drive.Service for further operation.
func Authenticate() *drive.Service {
	ctx := context.Background()

	usr, err := user.Current()
	if err != nil {
		log.Fatalf("Unable to get current user: %v", err)
	}
	path := usr.HomeDir + "/Documents/client_secret.json"
	b, err := ioutil.ReadFile(path)
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v", err)
	}

	config, err := google.ConfigFromJSON(b, drive.DriveScope)
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}
	client := getClient(ctx, config)

	srv, err := drive.New(client)
	if err != nil {
		log.Fatalf("Unable to retrieve drive Client: %v", err)
	}
	return srv
}

// getLeafFromParent resolves the ID of the requested leaf folder in given folder ID.
func getLeafFromParent(srv *drive.Service, leafName, parentID string) (string, error) {
	var q []string
	q = append(q, fmt.Sprintf("('%s' in parents)", parentID))
	q = append(q, fmt.Sprintf("name='%s'", leafName))
	q = append(q, fmt.Sprintf("mimeType='%s'", driveFolderType))
	q = append(q, "trashed=false")
	ansList, err := srv.Files.List().Q(strings.Join(q, "and")).Fields("files(id)").Do()
	if err != nil {
		return "", errors.New(fmt.Sprintf("failed to fetch ID of '%s': %v", leafName, err))
	} else if len(ansList.Files) == 0 {
		return "", ErrorNotFound(fmt.Sprintf("error: no '%s' in '%s'", leafName, parentID))
	} else if len(ansList.Files) > 1 {
		// we don't expect multiple archive roots
		return "", errors.New(fmt.Sprintf("error: multiple '%s' in '%s'", leafName, parentID))
	}
	return ansList.Files[0].Id, nil
}

// yesNoResponse prompts the user to make a choice, returning a boolean.
func yesNoResponse(prompt string) bool {
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

// GetUploadLocation resolves the folder ID of the given category.
func GetUploadLocation(srv *drive.Service, category string) (string, error) {
	var err error
	// get the archive root
	if archiveRootID == "" {
		archiveRootID, err = getLeafFromParent(srv, archiveRootName, "root")
		if err != nil {
			if _, ok := err.(ErrorNotFound); ok && yesNoResponse("Archive root not found; create it now?") {
				archiveRootID, err = CreateDirectory(srv, archiveRootName, "root")
				if err != nil {
					return "", errors.New(fmt.Sprintf("failed to create archive root '%s': %v",
						archiveRootName, err))
				}
			} else {
				return "", errors.New(fmt.Sprintf("failed to retrieve archive root '%s': %v",
					archiveRootName, err))
			}
		}
	}
	// get the desired category
	categoryID, ok := categoryIDs.Get(category)
	if !ok {
		categoryID, err = getLeafFromParent(srv, category, archiveRootID)
		if err != nil {
			if _, ok := err.(ErrorNotFound); ok && yesNoResponse(fmt.Sprintf(
				"Category '%s' not found; create it now?", category)) {
				categoryID, err = CreateDirectory(srv, category, archiveRootID)
				if err != nil {
					return "", errors.New(fmt.Sprintf("failed to create category '%s': %v",
						category, err))
				}
			} else {
				return "", errors.New(fmt.Sprintf("failed to retrieve category '%s': %v",
					category, err))
			}
		}
		categoryIDs.Set(category, categoryID)
	}
	//fmt.Printf("Category folder ID: %s\n", categoryID)
	return categoryID, nil
}

// CreateDirectory creates the directory with name leafName inside directory
// with ID of parentID, returning the ID of the created folder.
//
// Note: the caller shall check if the directory with leafName exists.
// Failing to do so will result in duplicate directories.
func CreateDirectory(srv *drive.Service, leafName, parentID string) (string, error) {
	createInfo := &drive.File{
		Name:        leafName,
		Description: leafName,
		MimeType:    driveFolderType,
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

// CreateFile creates the file with path leafPath inside directory
// with ID of parentID, uploads the contents of the file, and
// returns the ID of the created file. If forceRecheck is true, it checks if the MD5
// sums of remote and local matches.
//
// Note: the caller shall check if the file with leafName exists.
// Failing to do so will result in duplicate files.
func CreateFile(srv *drive.Service, leafPath, parentID string) (string, error) {
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
	// we don't need the md5Checksum field if forceRecheck != true
	intermediateCall := srv.Files.Create(createInfo).Media(uploadFile)
	if forceRecheck {
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
	if forceRecheck {
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
			return "", ErrorChecksumMismatch(fmt.Sprintf(
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

// WithRetry executes fn with retry upon failure in an exponential-backoff manner,
// if the error returned by fn satisfies shouldRetry.
func WithRetry(ctx context.Context, fn func() error, shouldRetry func(error) bool) error {
	l := getLogger(ctx)
	err := fn()
	if shouldRetry(err) {
		l.Printf("Need to retry due to: %v", err)
		err = retry(ctx, fn, shouldRetry, retryStartingRate, retryRatio)
	}
	if err != nil {
		err = errors.New(fmt.Sprintf("[Job #%s] retry failed: %v", l, err))
	}
	return err
}

// retry increases retry timeout by factor ratio every time until success, or if an error not
// worth trying occurs.
func retry(ctx context.Context, fn func() error, shouldRetry func(error) bool, currentRate, ratio int) error {
	l := getLogger(ctx)
	l.Printf("Waiting %d second(s) before retrying...", currentRate)
	time.Sleep(time.Duration(currentRate) * time.Second)
	err := fn()
	if shouldRetry(err) {
		err = retry(ctx, fn, shouldRetry, currentRate*ratio, ratio)
	}
	return err
}

// RetryIfNeeded takes an error, returning true if it's worth retrying.
func RetryIfNeeded(err error) bool {
	if err != nil {
		if realErr, ok := err.(*googleapi.Error); ok {
			// retry on rate limit and all server-side errors
			if realErr.Code == 403 && strings.Contains(
				strings.ToLower(realErr.Message), "rate limit") {
				return true
			} else if 499 < realErr.Code && realErr.Code < 600 {
				return true
			}
		} else if _, ok := err.(ErrorChecksumMismatch); ok {
			// retry on checksum mismatch
			return true
		} else if _, ok := err.(net.Error); ok {
			// retry on network problem
			return true
		}
	}
	return false
}

// SyncDirectory accepts a path to recursively upload to Google Drive to the specified category,
// returning any error that happens in the process.
//
// It creates a ".sync_finished" mark file in the directory upon finishing, and will return
// an ErrorAlreadySynced directly if that mark is present.
func SyncDirectory(srv *drive.Service, path, category string) error {
	// trim the trailing slash
	path = filepath.Clean(path)
	markFilePath := path + "/.sync_finished"
	// check if we have the mark file
	if _, err := os.Stat(markFilePath); !os.IsNotExist(err) {
		return ErrorAlreadySynced("folder already synced")
	}
	// parentIDs: key: path; value: parent ID
	parentIDs := make(map[string]string)
	var uploadWg sync.WaitGroup
	err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("Error occured while visiting path %s: %v", path, err)
			return err
		}
		if _, ok := ignoreList[info.Name()]; ok {
			return nil
		}
		parentPath, _ := filepath.Split(path)
		// trim the trailing slash
		parentPath = filepath.Clean(parentPath)
		parentID, ok := parentIDs[parentPath]
		if !ok {
			//log.Println("cache miss: ", parentPath)
			// parent path not present; this is the root of folder to upload
			parentID, err = GetUploadLocation(srv, category)
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
			err := WithRetry(ctxWithLoggerID(ctx, routineID), func() error {
				var err error
				*id, err = CreateDirectory(srv, info.Name(), parentID)
				return err
			}, RetryIfNeeded)
			if err != nil {
				return err
			} else {
				// record parent entry
				parentIDs[path] = *id
				//log.Println("added parent map entry: ", path, id)
				log.Printf("Created directory '%s' (from %s) with ID %s", info.Name(), path, *id)
			}
		} else {
			uploadWg.Add(1)
			go func() {
				defer uploadWg.Done()
				// we won't be checking if file exists (real filesystems won't have duplicate files)
				id := new(string)
				err := WithRetry(ctxWithLoggerID(ctx, routineID), func() error {
					var err error
					*id, err = CreateFile(srv, path, parentID)
					return err
				}, RetryIfNeeded)
				if err != nil {
					// TODO refine failure handling
					log.Fatalf("Failed to upload file '%s' (from %s): %v", info.Name(), path, err)
				}
				log.Printf("Uploaded file '%s' (from %s) with ID %s", info.Name(), path, *id)
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
		return errors.New(fmt.Sprintf("failed to set synced mark: %v", err))
	}
	return nil
}

func main() {
	if runtime.GOOS == "darwin" {
		// set up proxy for run in restricted network environment
		proxyUrl, _ := url.Parse("http://127.0.0.1:8001")
		http.DefaultTransport = &http.Transport{Proxy: http.ProxyURL(proxyUrl)}
	}

	// we need to do this manually for old runtime
	fmt.Println("Procs usable:", runtime.NumCPU())
	runtime.GOMAXPROCS(runtime.NumCPU())

	// authenticate to Google Drive server to get *drive.Service
	srv := Authenticate()

	fmt.Print("Enter folder to sync: ")
	target, err := reader.ReadString('\n')
	target = strings.TrimRight(target, "\n")
	if err != nil {
		log.Fatalf("Failed to scan: %v", err)
	}
	fmt.Print("Enter desired category: ")
	category, err := reader.ReadString('\n')
	category = strings.TrimRight(category, "\n")
	if err != nil {
		log.Fatalf("Failed to scan: %v", err)
	}

	err = SyncDirectory(srv, target, category)
	if err != nil {
		log.Fatalf("Failed to sync '%s': %v", target, err)
	}
	fmt.Println("Sync succeeded.")
}
