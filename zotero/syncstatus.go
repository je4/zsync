package zotero

type SyncStatus int

const (
	SyncStatus_New        SyncStatus = 0
	SyncStatus_Synced     SyncStatus = 1
	SyncStatus_Modified   SyncStatus = 2
	SyncStatus_Incomplete SyncStatus = 3
)

var SyncStatusString = map[SyncStatus]string{
	SyncStatus_New:        "new",
	SyncStatus_Synced:     "synced",
	SyncStatus_Modified:   "modified",
	SyncStatus_Incomplete: "incomplete",
}

var SyncStatusId = map[string]SyncStatus{
	"new":        SyncStatus_New,
	"synced":     SyncStatus_Synced,
	"modified":   SyncStatus_Modified,
	"incomplete": SyncStatus_Incomplete,
}

