package config

import (
	U "github.com/KireinaHoro/DriveSync/utils"
)

var (
	IgnoreList = map[string]struct{}{
		".DS_Store":      {},
		".localized":     {},
		".idea":          {},
		".sync_finished": {},
	}
	ArchiveRootID string
	CategoryIDs   = U.NewSafeMap()
)

const (
	DriveFolderType   = "application/vnd.google-apps.folder"
	RetryRatio        = 2
	RetryStartingRate = 1
)

var (
	ArchiveRootName string // "archive"
	Target          string // ""
	Category        string // "Uncategorized"
	ForceRecheck    bool   // true
	Interactive     bool   // false
	Verbose         bool   // false
	CreateMissing   bool   // false
)
