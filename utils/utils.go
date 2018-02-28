package utils

import (
	"fmt"
	"log"
	"sync"

	"golang.org/x/net/context"
	"io"
	"crypto/md5"
	"encoding/hex"
)

// SafeMap is a concurrent-safe map[string]string
type safeMap struct {
	v map[string]string
	m sync.Mutex
}

func NewSafeMap() *safeMap {
	return &safeMap{v: make(map[string]string)}
}

func (r *safeMap) Get(key string) (value string, ok bool) {
	r.m.Lock()
	defer r.m.Unlock()
	value, ok = r.v[key]
	return
}

func (r *safeMap) Set(key, value string) {
	r.m.Lock()
	defer r.m.Unlock()
	r.v[key] = value
}

// logger provides pretty logging when used with goroutines, with pseudo-routine-id
// for logs with better readability.
type logger string

const loggerID = "logger_id"

func (l logger) Printf(format string, v ...interface{}) {
	log.Printf("[Job #%s] %s", l, fmt.Sprintf(format, v...))
}

func (l logger) Println(v ...interface{}) {
	log.Printf("[Job #%s] %s", l, fmt.Sprintln(v...))
}

// CtxWithLoggerID generates a new context with loggerID for logging.
func CtxWithLoggerID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, loggerID, id)
}

// GetLogger extracts logger from context.
func GetLogger(ctx context.Context) logger {
	return logger(ctx.Value(loggerID).(string))
}

// CalculateSum takes an io.Reader and calculates its md5Checksum.
func CalculateSum(f io.Reader) (string, error) {
	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
