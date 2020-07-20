package zotero

type SyncDirection int64

const (
	SyncDirection_None       SyncDirection = 0
	SyncDirection_ToCloud    SyncDirection = 1 // local --> cloud
	SyncDirection_ToLocal    SyncDirection = 2 // cloud --> local
	SyncDirection_BothCloud  SyncDirection = 3 // local <--> cloud / cloud is master
	SyncDirection_BothLocal  SyncDirection = 4 // local <--> cloud / local is master
	SyncDirection_BothManual SyncDirection = 5 // local <--> cloud / manual conflict resolution
)

var SyncDirectionString = map[SyncDirection]string{
	SyncDirection_None:       "none",
	SyncDirection_ToCloud:    "tocloud",
	SyncDirection_ToLocal:    "tolocal",
	SyncDirection_BothCloud:  "bothcloud",
	SyncDirection_BothLocal:  "bothlocal",
	SyncDirection_BothManual: "bothmanual",
}

var SyncDirectionId = map[string]SyncDirection{
	"none":       SyncDirection_None,
	"tocloud":    SyncDirection_ToCloud,
	"tolocal":    SyncDirection_ToLocal,
	"bothcloud":  SyncDirection_BothCloud,
	"bothlocal":  SyncDirection_BothLocal,
	"bothmanual": SyncDirection_BothManual,
}
