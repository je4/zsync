package sync

import (
	"context"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgmock"
	"github.com/jackc/pgproto3/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/je4/zsync/v2/pkg/filesystem"
	"github.com/je4/zsync/v2/pkg/zotero/client"
	"github.com/je4/zsync/v2/pkg/zotero/model"
	"github.com/je4/zsync/v2/pkg/zotero/storage"
	"github.com/op/go-logging"
	"github.com/rs/zerolog"
)

func encodeInt8(v int64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(v))
	return b
}

func encodeBool(v bool) []byte {
	if v {
		return []byte{1}
	}
	return []byte{0}
}

func encodeText(s string) []byte {
	return []byte(s)
}

func encodeTimestamp(t time.Time) []byte {
	pgEpoch := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	micro := t.UTC().Sub(pgEpoch).Microseconds()
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(micro))
	return b
}

func mockQuerySteps(paramOIDs []uint32, fields []pgproto3.FieldDescription, rows [][][]byte) []pgmock.Step {
	steps := []pgmock.Step{
		pgmock.ExpectAnyMessage(&pgproto3.Parse{}),
		pgmock.ExpectAnyMessage(&pgproto3.Describe{}),
		pgmock.ExpectAnyMessage(&pgproto3.Sync{}),
		pgmock.SendMessage(&pgproto3.ParseComplete{}),
		pgmock.SendMessage(&pgproto3.ParameterDescription{ParameterOIDs: paramOIDs}),
		pgmock.SendMessage(&pgproto3.RowDescription{Fields: fields}),
		pgmock.SendMessage(&pgproto3.ReadyForQuery{TxStatus: 'I'}),
		pgmock.ExpectAnyMessage(&pgproto3.Bind{}),
		pgmock.ExpectAnyMessage(&pgproto3.Execute{}),
		pgmock.ExpectAnyMessage(&pgproto3.Sync{}),
		pgmock.SendMessage(&pgproto3.BindComplete{}),
	}
	for _, row := range rows {
		steps = append(steps, pgmock.SendMessage(&pgproto3.DataRow{Values: row}))
	}
	steps = append(steps,
		pgmock.SendMessage(&pgproto3.CommandComplete{CommandTag: []byte(fmt.Sprintf("SELECT %d", len(rows)))}),
		pgmock.SendMessage(&pgproto3.ReadyForQuery{TxStatus: 'I'}),
	)
	return steps
}

func mockExecSteps(paramOIDs []uint32, commandTag string) []pgmock.Step {
	return []pgmock.Step{
		pgmock.ExpectAnyMessage(&pgproto3.Parse{}),
		pgmock.ExpectAnyMessage(&pgproto3.Describe{}),
		pgmock.ExpectAnyMessage(&pgproto3.Sync{}),
		pgmock.SendMessage(&pgproto3.ParseComplete{}),
		pgmock.SendMessage(&pgproto3.ParameterDescription{ParameterOIDs: paramOIDs}),
		pgmock.SendMessage(&pgproto3.NoData{}),
		pgmock.SendMessage(&pgproto3.ReadyForQuery{TxStatus: 'I'}),
		pgmock.ExpectAnyMessage(&pgproto3.Bind{}),
		pgmock.ExpectAnyMessage(&pgproto3.Execute{}),
		pgmock.ExpectAnyMessage(&pgproto3.Sync{}),
		pgmock.SendMessage(&pgproto3.BindComplete{}),
		pgmock.SendMessage(&pgproto3.CommandComplete{CommandTag: []byte(commandTag)}),
		pgmock.SendMessage(&pgproto3.ReadyForQuery{TxStatus: 'I'}),
	}
}

func mockSimpleExecSteps(commandTag string) []pgmock.Step {
	return []pgmock.Step{
		pgmock.ExpectAnyMessage(&pgproto3.Query{}),
		pgmock.SendMessage(&pgproto3.CommandComplete{CommandTag: []byte(commandTag)}),
		pgmock.SendMessage(&pgproto3.ReadyForQuery{TxStatus: 'I'}),
	}
}

func startMockDatabase(t *testing.T, script *pgmock.Script) (*storage.Storage, func()) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on tcp: %v", err)
	}

	serverErrChan := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			serverErrChan <- err
			return
		}
		defer conn.Close()
		backend := pgproto3.NewBackend(pgproto3.NewChunkReader(conn), conn)
		serverErrChan <- script.Run(backend)
	}()

	port := ln.Addr().(*net.TCPAddr).Port
	dbURL := fmt.Sprintf("postgres://mockuser:mockpass@127.0.0.1:%d/mockdb?sslmode=disable&default_query_exec_mode=describe_exec", port)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		_ = ln.Close()
		t.Fatalf("failed to create connection pool to mock: %v", err)
	}

	zlog := zerolog.Nop()
	st := storage.NewStorage(pool, "public", true, &zlog)

	cleanup := func() {
		pool.Close()
		_ = ln.Close()
		select {
		case err := <-serverErrChan:
			if err != nil && !errors.Is(err, net.ErrClosed) && !strings.Contains(err.Error(), "use of closed network connection") && !errors.Is(err, io.EOF) {
				t.Errorf("mock database error: %v", err)
			}
		case <-time.After(2 * time.Second):
		}
	}

	return st, cleanup
}

func startMockZoteroLocalServer(t *testing.T, handler http.HandlerFunc) (*client.Client, func()) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Zotero-Server-ID", "MOCK-LOCAL-SERVER-ID")
		w.Header().Set("Total-Results", "0")
		handler(w, r)
	}))

	zlog := zerolog.Nop()
	c, err := client.NewClient(server.URL, "", &zlog)
	if err != nil {
		server.Close()
		t.Fatalf("failed to create mock local client: %v", err)
	}

	return c, server.Close
}

func startMockZoteroCloudServer(t *testing.T, apiKey string, handler http.HandlerFunc) (*client.Client, func()) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/keys/") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.MarshalWrite(w, model.ApiKey{
				UserId:   999,
				Username: "cloud-mock-user",
				Access: model.Access{
					User: model.AccessElements{
						Library: true,
						Files:   true,
						Notes:   true,
						Write:   true,
					},
					Groups: map[string]model.AccessElements{
						"12345": {
							Library: true,
							Files:   true,
							Notes:   true,
							Write:   true,
						},
					},
				},
			})
			return
		}
		w.Header().Set("Total-Results", "0")
		handler(w, r)
	}))

	zlog := zerolog.Nop()
	c, err := client.NewClient(server.URL, apiKey, &zlog)
	if err != nil {
		server.Close()
		t.Fatalf("failed to create mock cloud client: %v", err)
	}

	return c, server.Close
}

func TestSyncer_Mock_Local_SyncCollections(t *testing.T) {
	groupId := int64(12345)
	collKey := "COLLKEY01"

	collectionFields := []pgproto3.FieldDescription{
		{Name: []byte("key"), DataTypeOID: 1043},
		{Name: []byte("version"), DataTypeOID: 20},
		{Name: []byte("data"), DataTypeOID: 25},
		{Name: []byte("meta"), DataTypeOID: 25},
		{Name: []byte("deleted"), DataTypeOID: 16},
		{Name: []byte("sync"), DataTypeOID: 1043},
		{Name: []byte("gitlab"), DataTypeOID: 1184},
	}
	versionFields := []pgproto3.FieldDescription{
		{Name: []byte("version"), DataTypeOID: 20},
		{Name: []byte("sync"), DataTypeOID: 1043},
	}

	script := &pgmock.Script{
		Steps: append(
			pgmock.AcceptUnauthenticatedConnRequestSteps(),
			// 1. GetModifiedCollections -> 0 rows
			append(
				mockQuerySteps([]uint32{20, 25, 25}, collectionFields, nil),
				// 2. GetCollectionVersion for COLLKEY01 -> exists with version 0, synced
				append(
					mockQuerySteps([]uint32{20, 1043}, versionFields, [][][]byte{
						{encodeInt8(0), encodeText("synced")},
					}),
					// 3. UpdateCollection
					append(
						mockExecSteps([]uint32{20, 1043, 25, 25, 16, 20, 1043}, "UPDATE 1"),
						// 4. RefreshCollectionNameHier
						mockSimpleExecSteps("REFRESH MATERIALIZED VIEW")...,
					)...,
				)...,
			)...,
		),
	}

	st, cleanupDB := startMockDatabase(t, script)
	defer cleanupDB()

	cl, cleanupHTTP := startMockZoteroLocalServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Last-Modified-Version", "15")
		if strings.Contains(r.URL.RawQuery, "format=versions") {
			w.Header().Set("Total-Results", "1")
			w.Header().Set("Content-Type", "application/json")
			_ = json.MarshalWrite(w, map[string]int64{collKey: 15})
			return
		}
		if strings.Contains(r.URL.Path, "/collections") {
			w.Header().Set("Total-Results", "1")
			w.Header().Set("Content-Type", "application/json")
			colls := []model.Collection{
				{
					Key:     collKey,
					Version: 15,
					Library: model.Library{
						Id:   groupId,
						Type: "group",
					},
					Data: model.CollectionData{
						Key:  collKey,
						Name: "Mocked Local Collection",
					},
					Meta: model.CollectionMeta{
						NumCollections: 0,
						NumItems:       2,
					},
				},
			}
			_ = json.MarshalWrite(w, colls)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	defer cleanupHTTP()

	zlog := zerolog.Nop()
	syncer := NewSyncer(cl, st, nil, &zlog)

	group := &model.Group{
		Id:                groupId,
		Active:            true,
		Direction:         model.SyncDirection_BothCloud,
		CollectionVersion: 0,
	}

	counter, lastModVersion, err := syncer.SyncCollections(group)
	if err != nil {
		t.Fatalf("SyncCollections failed: %v", err)
	}
	if counter != 1 {
		t.Errorf("expected 1 synced collection, got %d", counter)
	}
	if lastModVersion != 15 {
		t.Errorf("expected lastModifiedVersion 15, got %d", lastModVersion)
	}
}

func TestSyncer_Mock_Cloud_DownloadAndUploadItems(t *testing.T) {
	groupId := int64(12345)
	itemKey := "ITEMKEY01"
	attachKey := "ATTACH01"
	testContent := []byte("hello zotero attachment binary data")
	contentMD5 := md5.Sum(testContent)
	contentMD5Hex := hex.EncodeToString(contentMD5[:])

	opLogger := logging.MustGetLogger("test")
	tempDir := t.TempDir()
	fsDir := filepath.Join(tempDir, "storage")
	if err := os.MkdirAll(fsDir, 0755); err != nil {
		t.Fatalf("failed to create fsDir: %v", err)
	}
	fs, err := filesystem.NewLocalFs(fsDir, opLogger)
	if err != nil {
		t.Fatalf("failed to create local fs: %v", err)
	}

	versionFields := []pgproto3.FieldDescription{
		{Name: []byte("version"), DataTypeOID: 20},
		{Name: []byte("sync"), DataTypeOID: 1043},
	}

	script := &pgmock.Script{
		Steps: append(
			pgmock.AcceptUnauthenticatedConnRequestSteps(),
			// 1. GetItemVersion for ATTACH01 (returns version 0, synced)
			append(
				mockQuerySteps([]uint32{20, 1043}, versionFields, [][][]byte{
					{encodeInt8(0), encodeText("synced")},
				}),
				// 2. GetItemVersion for ITEMKEY01 (returns version 0, synced)
				append(
					mockQuerySteps([]uint32{20, 1043}, versionFields, [][][]byte{
						{encodeInt8(0), encodeText("synced")},
					}),
					// 3. UpdateItem for ATTACH01
					append(
						mockExecSteps([]uint32{20, 25, 25, 16, 16, 1043, 1043, 20, 1043}, "UPDATE 1"),
						// 4. UpdateItem for ITEMKEY01
						append(
							mockExecSteps([]uint32{20, 25, 25, 16, 16, 1043, 1043, 20, 1043}, "UPDATE 1"),
							// 5. RefreshItemTypeHier
							mockSimpleExecSteps("SELECT 1")...,
						)...,
					)...,
				)...,
			)...,
		),
	}

	st, cleanupDB := startMockDatabase(t, script)
	defer cleanupDB()

	cl, cleanupHTTP := startMockZoteroCloudServer(t, "test-cloud-key", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Last-Modified-Version", "20")

		if strings.Contains(r.URL.Path, fmt.Sprintf("/items/%s/file", attachKey)) {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Last-Modified-Version", "20")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(testContent)
			return
		}

		if strings.Contains(r.URL.Path, "/items/trash") {
			if strings.Contains(r.URL.RawQuery, "format=versions") {
				w.Header().Set("Total-Results", "0")
				w.Header().Set("Content-Type", "application/json")
				_ = json.MarshalWrite(w, map[string]int64{})
				return
			}
		}

		if strings.Contains(r.URL.RawQuery, "format=versions") {
			w.Header().Set("Total-Results", "2")
			w.Header().Set("Content-Type", "application/json")
			_ = json.MarshalWrite(w, map[string]int64{attachKey: 20, itemKey: 20})
			return
		}

		if strings.Contains(r.URL.Path, "/items") {
			w.Header().Set("Total-Results", "2")
			w.Header().Set("Content-Type", "application/json")
			items := []model.Item{
				{
					Key:     attachKey,
					Version: 20,
					Data: model.ItemGeneric{
						ItemDataBase: model.ItemDataBase{
							Key:      attachKey,
							Version:  20,
							ItemType: "attachment",
						},
						LinkMode:    "imported_file",
						ContentType: "application/octet-stream",
						Filename:    "test.bin",
						MD5:         contentMD5Hex,
					},
				},
				{
					Key:     itemKey,
					Version: 20,
					Data: model.ItemGeneric{
						ItemDataBase: model.ItemDataBase{
							Key:      itemKey,
							Version:  20,
							ItemType: "journalArticle",
						},
						Title: "Mock Cloud Article",
					},
				},
			}
			_ = json.MarshalWrite(w, items)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	})
	defer cleanupHTTP()

	zlog := zerolog.Nop()
	syncer := NewSyncer(cl, st, fs, &zlog)

	group := &model.Group{
		Id:          groupId,
		Active:      true,
		Direction:   model.SyncDirection_BothCloud,
		ItemVersion: 0,
	}

	counter, lastModVersion, err := syncer.DownloadItems(group)
	if err != nil {
		t.Fatalf("DownloadItems failed: %v", err)
	}
	if counter != 2 {
		t.Errorf("expected 2 downloaded items (1 attachment + 1 article), got %d", counter)
	}
	if lastModVersion != 20 {
		t.Errorf("expected lastModifiedVersion 20, got %d", lastModVersion)
	}

	bucket, err := syncer.GetGroupBucket(groupId)
	if err != nil {
		t.Fatalf("GetGroupBucket failed: %v", err)
	}
	exists, err := fs.FileExists(bucket, attachKey)
	if err != nil || !exists {
		t.Errorf("expected attachment file %s to exist in fs bucket %s", attachKey, bucket)
	}
}

func TestSyncer_Mock_SyncTagsAndDeleted(t *testing.T) {
	groupId := int64(12345)

	script := &pgmock.Script{
		Steps: append(
			pgmock.AcceptUnauthenticatedConnRequestSteps(),
			// 1. CreateTag for "mock-tag"
			append(
				mockExecSteps([]uint32{1043, 25, 20}, "INSERT 0 1"),
				// 2. DeleteItem for "DELITEM01"
				append(
					mockExecSteps([]uint32{1043, 1043, 20}, "UPDATE 1"),
					// 3. DeleteCollection for "DELCOLL01"
					append(
						mockExecSteps([]uint32{1043, 20, 1043}, "UPDATE 1"),
						// 4. DeleteTag for "deltag"
						mockExecSteps([]uint32{1043, 20}, "DELETE 1")...,
					)...,
				)...,
			)...,
		),
	}

	st, cleanupDB := startMockDatabase(t, script)
	defer cleanupDB()

	cl, cleanupHTTP := startMockZoteroCloudServer(t, "test-api-key", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Last-Modified-Version", "30")

		if strings.Contains(r.URL.Path, "/tags") {
			w.Header().Set("Total-Results", "1")
			w.Header().Set("Content-Type", "application/json")
			tags := []model.Tag{
				{
					Tag: "mock-tag",
					Meta: &model.TagMeta{
						NumItems: 1,
					},
				},
			}
			_ = json.MarshalWrite(w, tags)
			return
		}

		if strings.Contains(r.URL.Path, "/deleted") {
			w.Header().Set("Content-Type", "application/json")
			deleted := model.Delete{
				Collections: []string{"DELCOLL01"},
				Items:       []string{"DELITEM01"},
				Tags:        []string{"deltag"},
			}
			_ = json.MarshalWrite(w, deleted)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	})
	defer cleanupHTTP()

	zlog := zerolog.Nop()
	syncer := NewSyncer(cl, st, nil, &zlog)

	group := &model.Group{
		Id:         groupId,
		Active:     true,
		Direction:  model.SyncDirection_BothCloud,
		SyncTags:   true,
		TagVersion: 0,
		Version:    0,
	}

	// Test SyncTags
	tagCount, tagVersion, err := syncer.SyncTags(group)
	if err != nil {
		t.Fatalf("SyncTags failed: %v", err)
	}
	if tagCount != 1 {
		t.Errorf("expected 1 tag synced, got %d", tagCount)
	}
	if tagVersion != 30 {
		t.Errorf("expected tagVersion 30, got %d", tagVersion)
	}

	// Test SyncDeleted
	deletedCount, err := syncer.SyncDeleted(group)
	if err != nil {
		t.Fatalf("SyncDeleted failed: %v", err)
	}
	if deletedCount != 3 {
		t.Errorf("expected 3 deleted entities, got %d", deletedCount)
	}
}

func TestSyncer_Mock_BackupLocal(t *testing.T) {
	groupId := int64(12345)
	collKey := "BACKUPCOLL01"
	itemKey := "BACKUPITEM01"
	attachKey := "BACKUPATTACH01"
	attachContent := []byte("backup binary content")

	opLogger := logging.MustGetLogger("test")
	tempDir := t.TempDir()
	sourceDir := filepath.Join(tempDir, "source")
	backupDir := filepath.Join(tempDir, "backup")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatalf("failed to create source dir: %v", err)
	}
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		t.Fatalf("failed to create backup dir: %v", err)
	}

	sourceFs, err := filesystem.NewLocalFs(sourceDir, opLogger)
	if err != nil {
		t.Fatalf("failed to create source fs: %v", err)
	}
	backupFs, err := filesystem.NewLocalFs(backupDir, opLogger)
	if err != nil {
		t.Fatalf("failed to create backup fs: %v", err)
	}

	bucket := fmt.Sprintf("zotero-%d", groupId)
	if err := sourceFs.FilePut(bucket, attachKey, attachContent, filesystem.FilePutOptions{}); err != nil {
		t.Fatalf("failed to write source attachment: %v", err)
	}

	collCountFields := []pgproto3.FieldDescription{{Name: []byte("count"), DataTypeOID: 20}}
	collFields := []pgproto3.FieldDescription{
		{Name: []byte("key"), DataTypeOID: 1043},
		{Name: []byte("version"), DataTypeOID: 20},
		{Name: []byte("data"), DataTypeOID: 25},
		{Name: []byte("meta"), DataTypeOID: 25},
		{Name: []byte("deleted"), DataTypeOID: 16},
		{Name: []byte("sync"), DataTypeOID: 1043},
		{Name: []byte("gitlab"), DataTypeOID: 1184},
	}
	itemCountFields := []pgproto3.FieldDescription{{Name: []byte("count"), DataTypeOID: 20}}
	itemFields := []pgproto3.FieldDescription{
		{Name: []byte("key"), DataTypeOID: 1043},
		{Name: []byte("version"), DataTypeOID: 20},
		{Name: []byte("data"), DataTypeOID: 25},
		{Name: []byte("meta"), DataTypeOID: 25},
		{Name: []byte("trashed"), DataTypeOID: 16},
		{Name: []byte("deleted"), DataTypeOID: 16},
		{Name: []byte("sync"), DataTypeOID: 1043},
		{Name: []byte("md5"), DataTypeOID: 1043},
		{Name: []byte("gitlab"), DataTypeOID: 1184},
	}

	script := &pgmock.Script{
		Steps: append(
			pgmock.AcceptUnauthenticatedConnRequestSteps(),
			// 1. IterateCollectionsAll: count query
			append(
				mockQuerySteps([]uint32{20}, collCountFields, [][][]byte{{encodeInt8(1)}}),
				// 2. IterateCollectionsAll: select rows
				append(
					mockQuerySteps([]uint32{20}, collFields, [][][]byte{
						{
							encodeText(collKey),
							encodeInt8(10),
							encodeText(`{"key":"` + collKey + `","name":"Backup Coll"}`),
							encodeText(`{"numCollections":0,"numItems":1}`),
							encodeBool(false),
							encodeText("synced"),
							nil,
						},
					}),
					// 3. UpdateCollectionsGitlabTimestamp
					append(
						mockExecSteps([]uint32{1043, 20}, "UPDATE 1"),
						// 4. IterateItemsAll: count query
						append(
							mockQuerySteps([]uint32{20}, itemCountFields, [][][]byte{{encodeInt8(2)}}),
							// 5. IterateItemsAll: select rows
							append(
								mockQuerySteps([]uint32{20}, itemFields, [][][]byte{
									{
										encodeText(itemKey),
										encodeInt8(10),
										encodeText(`{"key":"` + itemKey + `","itemType":"journalArticle","title":"Backup Article"}`),
										encodeText(`{}`),
										encodeBool(false),
										encodeBool(false),
										encodeText("synced"),
										nil,
										nil,
									},
									{
										encodeText(attachKey),
										encodeInt8(10),
										encodeText(`{"key":"` + attachKey + `","itemType":"attachment","linkMode":"imported_file","filename":"file.bin"}`),
										encodeText(`{}`),
										encodeBool(false),
										encodeBool(false),
										encodeText("synced"),
										encodeText("mockmd5"),
										nil,
									},
								}),
								// 6. UpdateItemsGitlabTimestamp
								append(
									mockExecSteps([]uint32{1043, 20}, "UPDATE 2"),
									// 7. UpdateGroupGitlabTimestamp
									mockExecSteps([]uint32{1043, 20}, "UPDATE 1")...,
								)...,
							)...,
						)...,
					)...,
				)...,
			)...,
		),
	}

	st, cleanupDB := startMockDatabase(t, script)
	defer cleanupDB()

	zlog := zerolog.Nop()
	syncer := NewSyncer(nil, st, sourceFs, &zlog)

	group := &model.Group{
		Id:                groupId,
		Active:            true,
		CollectionVersion: 10,
		ItemVersion:       10,
		TagVersion:        10,
		Data: model.GroupData{
			Name: "Backup Mock Group",
		},
	}

	if err := syncer.BackupLocal(group, backupFs); err != nil {
		t.Fatalf("BackupLocal failed: %v", err)
	}

	// Verify generated backup files in backupFs (folder is <groupId>/collections, <groupId>/items, <groupId>)
	collFolder := fmt.Sprintf("%d/collections", groupId)
	collExists, err := backupFs.FileExists(collFolder, collKey+".json")
	if err != nil || !collExists {
		t.Errorf("expected collection backup file %s/%s.json to exist", collFolder, collKey)
	}

	itemFolder := fmt.Sprintf("%d/items", groupId)
	itemExists, err := backupFs.FileExists(itemFolder, itemKey+".json")
	if err != nil || !itemExists {
		t.Errorf("expected item backup file %s/%s.json to exist", itemFolder, itemKey)
	}

	binExists, err := backupFs.FileExists(itemFolder, attachKey+".bin")
	if err != nil || !binExists {
		t.Errorf("expected binary attachment file %s/%s.bin to exist in backup", itemFolder, attachKey)
	}

	groupFolder := fmt.Sprintf("%d", groupId)
	groupExists, err := backupFs.FileExists(groupFolder, "group.json")
	if err != nil || !groupExists {
		t.Errorf("expected group backup file %s/group.json to exist", groupFolder)
	}
}

func TestSyncer_Mock_SyncGroup_Full(t *testing.T) {
	groupId := int64(12345)

	collectionFields := []pgproto3.FieldDescription{
		{Name: []byte("key"), DataTypeOID: 1043},
		{Name: []byte("version"), DataTypeOID: 20},
		{Name: []byte("data"), DataTypeOID: 25},
		{Name: []byte("meta"), DataTypeOID: 25},
		{Name: []byte("deleted"), DataTypeOID: 16},
		{Name: []byte("sync"), DataTypeOID: 1043},
		{Name: []byte("gitlab"), DataTypeOID: 1184},
	}
	itemFields := []pgproto3.FieldDescription{
		{Name: []byte("key"), DataTypeOID: 1043},
		{Name: []byte("version"), DataTypeOID: 20},
		{Name: []byte("data"), DataTypeOID: 25},
		{Name: []byte("meta"), DataTypeOID: 25},
		{Name: []byte("trashed"), DataTypeOID: 16},
		{Name: []byte("deleted"), DataTypeOID: 16},
		{Name: []byte("sync"), DataTypeOID: 1043},
		{Name: []byte("md5"), DataTypeOID: 1043},
		{Name: []byte("gitlab"), DataTypeOID: 1184},
	}

	script := &pgmock.Script{
		Steps: append(
			pgmock.AcceptUnauthenticatedConnRequestSteps(),
			// 1. SyncCollections: GetModifiedCollections -> 0 rows
			append(
				mockQuerySteps([]uint32{20, 25, 25}, collectionFields, nil),
				// 2. UploadItems: GetModifiedItems -> 0 rows
				append(
					mockQuerySteps([]uint32{20, 25, 25}, itemFields, nil),
					// 3. SyncTags: CreateTag
					append(
						mockExecSteps([]uint32{1043, 25, 20}, "INSERT 0 1"),
						// 4. UpdateGroup
						mockExecSteps([]uint32{20, 1184, 1184, 25, 16, 20, 20, 20, 20}, "UPDATE 1")...,
					)...,
				)...,
			)...,
		),
	}

	st, cleanupDB := startMockDatabase(t, script)
	defer cleanupDB()

	cl, cleanupHTTP := startMockZoteroCloudServer(t, "test-api-key", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Last-Modified-Version", "100")
		w.Header().Set("Total-Results", "0")
		if strings.Contains(r.URL.RawQuery, "format=versions") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.MarshalWrite(w, map[string]int64{})
			return
		}
		if strings.Contains(r.URL.Path, "/tags") {
			w.Header().Set("Total-Results", "1")
			w.Header().Set("Content-Type", "application/json")
			tags := []model.Tag{
				{
					Tag: "synced-tag",
					Meta: &model.TagMeta{
						NumItems: 1,
					},
				},
			}
			_ = json.MarshalWrite(w, tags)
			return
		}
		if strings.Contains(r.URL.Path, "/deleted") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.MarshalWrite(w, model.Delete{})
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("[]"))
	})
	defer cleanupHTTP()

	zlog := zerolog.Nop()
	syncer := NewSyncer(cl, st, nil, &zlog)

	group := &model.Group{
		Id:                groupId,
		Active:            true,
		Direction:         model.SyncDirection_BothCloud,
		CollectionVersion: 50,
		ItemVersion:       50,
		TagVersion:        50,
		Version:           50,
		SyncTags:          true,
		Data: model.GroupData{
			Name: "Full Sync Group",
		},
	}

	if err := syncer.SyncGroup(group); err != nil {
		t.Fatalf("SyncGroup failed: %v", err)
	}

	if group.CollectionVersion != 100 {
		t.Errorf("expected group CollectionVersion 100, got %d", group.CollectionVersion)
	}
	if group.ItemVersion != 100 {
		t.Errorf("expected group ItemVersion 100, got %d", group.ItemVersion)
	}
	if group.TagVersion != 100 {
		t.Errorf("expected group TagVersion 100, got %d", group.TagVersion)
	}
}
