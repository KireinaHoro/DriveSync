package config

import (
	U "github.com/KireinaHoro/DriveSync/utils"
)

// runtime config
var (
	IgnoreList = map[string]struct{}{
		".DS_Store":       {},
		".localized":      {},
		".idea":           {},
		".git":            {},
		".drivesync-lock": {},
	}
	ArchiveRootID string
	CategoryIDs   = U.NewSafeMap()
)

// Constants that denote the default values for config values.
const (
	DriveFolderType   = "application/vnd.google-apps.folder"
	RetryRatio        = 2
	RetryStartingRate = 1
	ArchiveRootName   = "archive"
	Category          = "Uncategorized"
	ForceRecheck      = true
	Verbose           = true
	CreateMissing     = false
	UseProxy          = false
)

// Variables that only get used by `drivesync`
var (
	// Interactive only affects `drivesync`; `drivesyncd` is always non-interactive
	Interactive = false
	// Target here denotes the object to be synced when calling `drivesync`;
	// not Config.Target
	Target = ""
)

// configPath stores the location of the configuration file.
// It's populated by the getConfigPath method.
var configPath string

// Config is the global variable that holds the configuration for the running daemon.
var Config config

// type config denotes the configuration read by the daemon.
type config struct {
	ArchiveRootName   string `json:"archive"`
	ClientSecretPath  string `json:"client-secret-path"`
	CreateMissing     bool   `json:"create-missing"`
	DefaultCategory   string `json:"default-category"`
	ForceRecheck      bool   `json:"force-recheck"`
	LogFile           string `json:"log-file"`
	PidFile           string `json:"pid-file"`
	ProxyURL          string `json:"proxy-url"`
	RetryRatio        int    `json:"retry-ratio"`
	RetryStartingRate int    `json:"retry-starting-rate"`
	// Config.Target denotes the directory to be watched when calling `drivesyncd`
	Target   string `json:"target"`
	UseProxy bool   `json:"use-proxy"`
	Verbose  bool   `json:"verbose"`
}
