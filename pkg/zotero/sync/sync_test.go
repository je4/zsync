package sync

import (
	"testing"

	"github.com/je4/zsync/v2/pkg/filesystem"
	"github.com/je4/zsync/v2/pkg/zotero/client"
	"github.com/je4/zsync/v2/pkg/zotero/storage"
	"github.com/op/go-logging"
	"github.com/rs/zerolog"
)

func TestSyncerInit(t *testing.T) {
	logger := zerolog.Nop()
	cl := &client.Client{}
	st := &storage.Storage{}
	opLogger := logging.MustGetLogger("test")
	localFs, err := filesystem.NewLocalFs(t.TempDir(), opLogger)
	if err != nil {
		t.Fatalf("failed to create local fs: %v", err)
	}

	syncer := NewSyncer(cl, st, localFs, &logger)
	if syncer == nil {
		t.Fatal("expected non-nil Syncer")
	}

	bucket, err := syncer.GetGroupBucket(12345)
	if err != nil {
		t.Fatalf("unexpected error getting group bucket: %v", err)
	}
	expectedBucket := "zotero-12345"
	if bucket != expectedBucket {
		t.Errorf("expected bucket %s, got %s", expectedBucket, bucket)
	}
}
