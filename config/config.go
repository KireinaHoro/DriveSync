package config

import (
	U "github.com/KireinaHoro/DriveSync/utils"
)

var (
	IgnoreList = map[string]struct{}{
		".DS_Store":      {},
		".localized":     {},
		".idea":          {},
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
	ArchiveRootName = "archive"
	Target          = ""
	Category        = "Uncategorized"
	ForceRecheck    = true
	Interactive     = false
	Verbose         = false
	CreateMissing   = false
)
