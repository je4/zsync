package storage

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgmock"
	"github.com/jackc/pgproto3/v2"
	"github.com/je4/zsync/v2/pkg/zotero/model"
)

func TestPgMock_GetGroup(t *testing.T) {
	script := &pgmock.Script{
		Steps: append(
			pgmock.AcceptUnauthenticatedConnRequestSteps(),
			pgmock.ExpectAnyMessage(&pgproto3.Parse{}),
			pgmock.ExpectAnyMessage(&pgproto3.Describe{}),
			pgmock.ExpectAnyMessage(&pgproto3.Sync{}),
			pgmock.SendMessage(&pgproto3.ParseComplete{}),
			pgmock.SendMessage(&pgproto3.ParameterDescription{ParameterOIDs: []uint32{20}}), // int8
			pgmock.SendMessage(&pgproto3.RowDescription{
				Fields: []pgproto3.FieldDescription{
					{Name: []byte("version"), DataTypeOID: 20},
					{Name: []byte("created"), DataTypeOID: 1184},
					{Name: []byte("modified"), DataTypeOID: 1184},
					{Name: []byte("data"), DataTypeOID: 25},
					{Name: []byte("active"), DataTypeOID: 16},
					{Name: []byte("direction"), DataTypeOID: 1043},
					{Name: []byte("tags"), DataTypeOID: 16},
					{Name: []byte("itemversion"), DataTypeOID: 20},
					{Name: []byte("collectionversion"), DataTypeOID: 20},
					{Name: []byte("tagversion"), DataTypeOID: 20},
					{Name: []byte("gitlab"), DataTypeOID: 1184},
				},
			}),
			pgmock.SendMessage(&pgproto3.ReadyForQuery{TxStatus: 'I'}),
			pgmock.ExpectAnyMessage(&pgproto3.Bind{}),
			pgmock.ExpectAnyMessage(&pgproto3.Execute{}),
			pgmock.ExpectAnyMessage(&pgproto3.Sync{}),
			pgmock.SendMessage(&pgproto3.BindComplete{}),
			pgmock.SendMessage(&pgproto3.DataRow{
				Values: [][]byte{
					encodeInt8(42), // version
					encodeTimestamp(time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)), // created
					encodeTimestamp(time.Date(2026, 8, 20, 13, 0, 0, 0, time.UTC)), // modified
					encodeText(`{"id":12345,"name":"Mocked Group"}`),               // data
					encodeBool(true),      // active
					encodeText("tolocal"), // direction
					encodeBool(true),      // tags
					encodeInt8(10),        // itemversion
					encodeInt8(20),        // collectionversion
					encodeInt8(30),        // tagversion
					nil,                   // gitlab (NULL)
				},
			}),
			pgmock.SendMessage(&pgproto3.CommandComplete{CommandTag: []byte("SELECT 1")}),
			pgmock.SendMessage(&pgproto3.ReadyForQuery{TxStatus: 'I'}),
		),
	}

	st, cleanup := startMockServer(t, script)
	defer cleanup()

	group, err := st.GetGroup(context.Background(), 12345)
	if err != nil {
		t.Fatalf("GetGroup failed: %v", err)
	}

	if group == nil {
		t.Fatal("expected non-nil group")
	}
	if group.Id != 12345 {
		t.Errorf("expected group ID 12345, got %d", group.Id)
	}
	if group.Version != 42 {
		t.Errorf("expected group version 42, got %d", group.Version)
	}
	if group.Data.Name != "Mocked Group" {
		t.Errorf("expected group name 'Mocked Group', got %q", group.Data.Name)
	}
	if !group.Active {
		t.Errorf("expected group to be active")
	}
	if group.ItemVersion != 10 {
		t.Errorf("expected item version 10, got %d", group.ItemVersion)
	}
	if group.CollectionVersion != 20 {
		t.Errorf("expected collection version 20, got %d", group.CollectionVersion)
	}
	if group.TagVersion != 30 {
		t.Errorf("expected tag version 30, got %d", group.TagVersion)
	}
}

func TestPgMock_GetCollectionByKey(t *testing.T) {
	script := &pgmock.Script{
		Steps: append(
			pgmock.AcceptUnauthenticatedConnRequestSteps(),
			pgmock.ExpectAnyMessage(&pgproto3.Parse{}),
			pgmock.ExpectAnyMessage(&pgproto3.Describe{}),
			pgmock.ExpectAnyMessage(&pgproto3.Sync{}),
			pgmock.SendMessage(&pgproto3.ParseComplete{}),
			pgmock.SendMessage(&pgproto3.ParameterDescription{ParameterOIDs: []uint32{20, 1043}}), // int8, varchar
			pgmock.SendMessage(&pgproto3.RowDescription{
				Fields: []pgproto3.FieldDescription{
					{Name: []byte("key"), DataTypeOID: 1043},
					{Name: []byte("version"), DataTypeOID: 20},
					{Name: []byte("data"), DataTypeOID: 25},
					{Name: []byte("meta"), DataTypeOID: 25},
					{Name: []byte("deleted"), DataTypeOID: 16},
					{Name: []byte("sync"), DataTypeOID: 1043},
					{Name: []byte("gitlab"), DataTypeOID: 1184},
				},
			}),
			pgmock.SendMessage(&pgproto3.ReadyForQuery{TxStatus: 'I'}),
			pgmock.ExpectAnyMessage(&pgproto3.Bind{}),
			pgmock.ExpectAnyMessage(&pgproto3.Execute{}),
			pgmock.ExpectAnyMessage(&pgproto3.Sync{}),
			pgmock.SendMessage(&pgproto3.BindComplete{}),
			pgmock.SendMessage(&pgproto3.DataRow{
				Values: [][]byte{
					encodeText("COLLKEY1"),
					encodeInt8(5),
					encodeText(`{"key":"COLLKEY1","name":"Mocked Collection","parentCollection":""}`),
					encodeText(`{"numCollections":2,"numItems":10}`),
					encodeBool(false),
					encodeText("synced"),
					nil,
				},
			}),
			pgmock.SendMessage(&pgproto3.CommandComplete{CommandTag: []byte("SELECT 1")}),
			pgmock.SendMessage(&pgproto3.ReadyForQuery{TxStatus: 'I'}),
		),
	}

	st, cleanup := startMockServer(t, script)
	defer cleanup()

	coll, err := st.GetCollectionByKey(context.Background(), 12345, "COLLKEY1")
	if err != nil {
		t.Fatalf("GetCollectionByKey failed: %v", err)
	}
	if coll == nil {
		t.Fatal("expected non-nil collection")
	}
	if coll.Key != "COLLKEY1" {
		t.Errorf("expected key 'COLLKEY1', got %q", coll.Key)
	}
	if coll.Version != 5 {
		t.Errorf("expected version 5, got %d", coll.Version)
	}
	if coll.Data.Name != "Mocked Collection" {
		t.Errorf("expected collection name 'Mocked Collection', got %q", coll.Data.Name)
	}
	if coll.Status != model.SyncStatus_Synced {
		t.Errorf("expected status Synced, got %v", coll.Status)
	}
	if coll.Deleted {
		t.Errorf("expected deleted to be false")
	}
}

func TestPgMock_GetItemByKey(t *testing.T) {
	script := &pgmock.Script{
		Steps: append(
			pgmock.AcceptUnauthenticatedConnRequestSteps(),
			pgmock.ExpectAnyMessage(&pgproto3.Parse{}),
			pgmock.ExpectAnyMessage(&pgproto3.Describe{}),
			pgmock.ExpectAnyMessage(&pgproto3.Sync{}),
			pgmock.SendMessage(&pgproto3.ParseComplete{}),
			pgmock.SendMessage(&pgproto3.ParameterDescription{ParameterOIDs: []uint32{20, 1043}}), // int8, varchar
			pgmock.SendMessage(&pgproto3.RowDescription{
				Fields: []pgproto3.FieldDescription{
					{Name: []byte("key"), DataTypeOID: 1043},
					{Name: []byte("version"), DataTypeOID: 20},
					{Name: []byte("data"), DataTypeOID: 25},
					{Name: []byte("meta"), DataTypeOID: 25},
					{Name: []byte("trashed"), DataTypeOID: 16},
					{Name: []byte("deleted"), DataTypeOID: 16},
					{Name: []byte("sync"), DataTypeOID: 1043},
					{Name: []byte("md5"), DataTypeOID: 1043},
					{Name: []byte("gitlab"), DataTypeOID: 1184},
				},
			}),
			pgmock.SendMessage(&pgproto3.ReadyForQuery{TxStatus: 'I'}),
			pgmock.ExpectAnyMessage(&pgproto3.Bind{}),
			pgmock.ExpectAnyMessage(&pgproto3.Execute{}),
			pgmock.ExpectAnyMessage(&pgproto3.Sync{}),
			pgmock.SendMessage(&pgproto3.BindComplete{}),
			pgmock.SendMessage(&pgproto3.DataRow{
				Values: [][]byte{
					encodeText("ITEMKEY1"),
					encodeInt8(8),
					encodeText(`{"key":"ITEMKEY1","itemType":"journalArticle","title":"Mocked Article","creators":[{"creatorType":"author","firstName":"Alice","lastName":"Smith"}]}`),
					encodeText(`{"creatorSummary":"Smith","numChildren":1}`),
					encodeBool(false),
					encodeBool(false),
					encodeText("synced"),
					encodeText("d41d8cd98f00b204e9800998ecf8427e"),
					nil,
				},
			}),
			pgmock.SendMessage(&pgproto3.CommandComplete{CommandTag: []byte("SELECT 1")}),
			pgmock.SendMessage(&pgproto3.ReadyForQuery{TxStatus: 'I'}),
		),
	}

	st, cleanup := startMockServer(t, script)
	defer cleanup()

	item, err := st.GetItemByKey(context.Background(), 12345, "ITEMKEY1")
	if err != nil {
		t.Fatalf("GetItemByKey failed: %v", err)
	}
	if item == nil {
		t.Fatal("expected non-nil item")
	}
	if item.Key != "ITEMKEY1" {
		t.Errorf("expected key 'ITEMKEY1', got %q", item.Key)
	}
	if item.Version != 8 {
		t.Errorf("expected version 8, got %d", item.Version)
	}
	if item.Data.Title != "Mocked Article" {
		t.Errorf("expected item title 'Mocked Article', got %q", item.Data.Title)
	}
	if item.MD5 != "d41d8cd98f00b204e9800998ecf8427e" {
		t.Errorf("expected item MD5 'd41d8cd98f00b204e9800998ecf8427e', got %q", item.MD5)
	}
}

func TestPgMock_CreateTag(t *testing.T) {
	script := &pgmock.Script{
		Steps: append(
			pgmock.AcceptUnauthenticatedConnRequestSteps(),
			pgmock.ExpectAnyMessage(&pgproto3.Parse{}),
			pgmock.ExpectAnyMessage(&pgproto3.Describe{}),
			pgmock.ExpectAnyMessage(&pgproto3.Sync{}),
			pgmock.SendMessage(&pgproto3.ParseComplete{}),
			pgmock.SendMessage(&pgproto3.ParameterDescription{ParameterOIDs: []uint32{1043, 25, 20}}),
			pgmock.SendMessage(&pgproto3.NoData{}),
			pgmock.SendMessage(&pgproto3.ReadyForQuery{TxStatus: 'I'}),
			pgmock.ExpectAnyMessage(&pgproto3.Bind{}),
			pgmock.ExpectAnyMessage(&pgproto3.Execute{}),
			pgmock.ExpectAnyMessage(&pgproto3.Sync{}),
			pgmock.SendMessage(&pgproto3.BindComplete{}),
			pgmock.SendMessage(&pgproto3.CommandComplete{CommandTag: []byte("INSERT 0 1")}),
			pgmock.SendMessage(&pgproto3.ReadyForQuery{TxStatus: 'I'}),
		),
	}

	st, cleanup := startMockServer(t, script)
	defer cleanup()

	err := st.CreateTag(context.Background(), 12345, model.Tag{
		Tag:  "golang",
		Meta: &model.TagMeta{NumItems: 5},
	})
	if err != nil {
		t.Fatalf("CreateTag failed: %v", err)
	}
}

func TestPgMock_UniqueViolationFallback(t *testing.T) {
	script := &pgmock.Script{
		Steps: append(
			pgmock.AcceptUnauthenticatedConnRequestSteps(),
			// 1. Initial INSERT into items fails with unique violation on items_oldid_constraint
			pgmock.ExpectAnyMessage(&pgproto3.Parse{}),
			pgmock.ExpectAnyMessage(&pgproto3.Describe{}),
			pgmock.ExpectAnyMessage(&pgproto3.Sync{}),
			pgmock.SendMessage(&pgproto3.ParseComplete{}),
			pgmock.SendMessage(&pgproto3.ParameterDescription{ParameterOIDs: []uint32{1043, 20, 20, 1043, 25, 1043}}), // key, version, library, sync, data, oldid
			pgmock.SendMessage(&pgproto3.NoData{}),
			pgmock.SendMessage(&pgproto3.ReadyForQuery{TxStatus: 'I'}),
			pgmock.ExpectAnyMessage(&pgproto3.Bind{}),
			pgmock.ExpectAnyMessage(&pgproto3.Execute{}),
			pgmock.ExpectAnyMessage(&pgproto3.Sync{}),
			pgmock.SendMessage(&pgproto3.ErrorResponse{
				Severity:       "ERROR",
				Code:           "23505",
				Message:        "duplicate key value violates unique constraint \"items_oldid_constraint\"",
				ConstraintName: "items_oldid_constraint",
			}),
			pgmock.SendMessage(&pgproto3.ReadyForQuery{TxStatus: 'I'}),

			// 2. Fallback query GetItemByOldid fetches the existing item
			pgmock.ExpectAnyMessage(&pgproto3.Parse{}),
			pgmock.ExpectAnyMessage(&pgproto3.Describe{}),
			pgmock.ExpectAnyMessage(&pgproto3.Sync{}),
			pgmock.SendMessage(&pgproto3.ParseComplete{}),
			pgmock.SendMessage(&pgproto3.ParameterDescription{ParameterOIDs: []uint32{20, 1043}}),
			pgmock.SendMessage(&pgproto3.RowDescription{
				Fields: []pgproto3.FieldDescription{
					{Name: []byte("key"), DataTypeOID: 1043},
					{Name: []byte("version"), DataTypeOID: 20},
					{Name: []byte("data"), DataTypeOID: 25},
					{Name: []byte("meta"), DataTypeOID: 25},
					{Name: []byte("trashed"), DataTypeOID: 16},
					{Name: []byte("deleted"), DataTypeOID: 16},
					{Name: []byte("sync"), DataTypeOID: 1043},
					{Name: []byte("md5"), DataTypeOID: 1043},
					{Name: []byte("gitlab"), DataTypeOID: 1184},
				},
			}),
			pgmock.SendMessage(&pgproto3.ReadyForQuery{TxStatus: 'I'}),
			pgmock.ExpectAnyMessage(&pgproto3.Bind{}),
			pgmock.ExpectAnyMessage(&pgproto3.Execute{}),
			pgmock.ExpectAnyMessage(&pgproto3.Sync{}),
			pgmock.SendMessage(&pgproto3.BindComplete{}),
			pgmock.SendMessage(&pgproto3.DataRow{
				Values: [][]byte{
					encodeText("EXISTINGKEY"),
					encodeInt8(15),
					encodeText(`{"key":"EXISTINGKEY","itemType":"book","title":"Existing Book"}`),
					encodeText(`{"creatorSummary":"","numChildren":0}`),
					encodeBool(false),
					encodeBool(false),
					encodeText("synced"),
					nil,
					nil,
				},
			}),
			pgmock.SendMessage(&pgproto3.CommandComplete{CommandTag: []byte("SELECT 1")}),
			pgmock.SendMessage(&pgproto3.ReadyForQuery{TxStatus: 'I'}),

			// 3. UpdateItem updates the existing item with new data
			pgmock.ExpectAnyMessage(&pgproto3.Parse{}),
			pgmock.ExpectAnyMessage(&pgproto3.Describe{}),
			pgmock.ExpectAnyMessage(&pgproto3.Sync{}),
			pgmock.SendMessage(&pgproto3.ParseComplete{}),
			pgmock.SendMessage(&pgproto3.ParameterDescription{ParameterOIDs: []uint32{20, 25, 25, 16, 16, 1043, 1043, 20, 1043}}),
			pgmock.SendMessage(&pgproto3.NoData{}),
			pgmock.SendMessage(&pgproto3.ReadyForQuery{TxStatus: 'I'}),
			pgmock.ExpectAnyMessage(&pgproto3.Bind{}),
			pgmock.ExpectAnyMessage(&pgproto3.Execute{}),
			pgmock.ExpectAnyMessage(&pgproto3.Sync{}),
			pgmock.SendMessage(&pgproto3.BindComplete{}),
			pgmock.SendMessage(&pgproto3.CommandComplete{CommandTag: []byte("UPDATE 1")}),
			pgmock.SendMessage(&pgproto3.ReadyForQuery{TxStatus: 'I'}),
		),
	}

	st, cleanup := startMockServer(t, script)
	defer cleanup()

	itemData := &model.ItemGeneric{}
	itemData.Key = "NEWKEY"
	itemData.ItemType = "book"
	itemData.Title = "Duplicate OldID Book"

	item, err := st.CreateItem(context.Background(), 12345, itemData, nil, "old-duplicate-id")
	if err != nil {
		t.Fatalf("CreateItem expected fallback to succeed on duplicate oldid, but got error: %v", err)
	}
	if item == nil {
		t.Fatal("expected item from fallback, got nil")
	}
	if item.Key != "EXISTINGKEY" {
		t.Errorf("expected existing item key 'EXISTINGKEY', got %q", item.Key)
	}
	if item.Version != 15 {
		t.Errorf("expected existing item version 15, got %d", item.Version)
	}
}

func TestPgMock_GetCollectionByName(t *testing.T) {
	script := &pgmock.Script{
		Steps: append(
			pgmock.AcceptUnauthenticatedConnRequestSteps(),
			pgmock.ExpectAnyMessage(&pgproto3.Parse{}),
			pgmock.ExpectAnyMessage(&pgproto3.Describe{}),
			pgmock.ExpectAnyMessage(&pgproto3.Sync{}),
			pgmock.SendMessage(&pgproto3.ParseComplete{}),
			pgmock.SendMessage(&pgproto3.ParameterDescription{ParameterOIDs: []uint32{20, 1043, 1043}}), // int8, varchar, varchar
			pgmock.SendMessage(&pgproto3.RowDescription{
				Fields: []pgproto3.FieldDescription{
					{Name: []byte("key"), DataTypeOID: 1043},
					{Name: []byte("version"), DataTypeOID: 20},
					{Name: []byte("data"), DataTypeOID: 25},
					{Name: []byte("meta"), DataTypeOID: 25},
					{Name: []byte("deleted"), DataTypeOID: 16},
					{Name: []byte("sync"), DataTypeOID: 1043},
					{Name: []byte("gitlab"), DataTypeOID: 1184},
				},
			}),
			pgmock.SendMessage(&pgproto3.ReadyForQuery{TxStatus: 'I'}),
			pgmock.ExpectAnyMessage(&pgproto3.Bind{}),
			pgmock.ExpectAnyMessage(&pgproto3.Execute{}),
			pgmock.ExpectAnyMessage(&pgproto3.Sync{}),
			pgmock.SendMessage(&pgproto3.BindComplete{}),
			pgmock.SendMessage(&pgproto3.DataRow{
				Values: [][]byte{
					encodeText("NAMEDCOLL1"),
					encodeInt8(3),
					encodeText(`{"key":"NAMEDCOLL1","name":"Target Collection","parentCollection":""}`),
					encodeText(`{"numCollections":0,"numItems":4}`),
					encodeBool(false),
					encodeText("synced"),
					nil,
				},
			}),
			pgmock.SendMessage(&pgproto3.CommandComplete{CommandTag: []byte("SELECT 1")}),
			pgmock.SendMessage(&pgproto3.ReadyForQuery{TxStatus: 'I'}),
		),
	}

	st, cleanup := startMockServer(t, script)
	defer cleanup()

	coll, err := st.GetCollectionByName(context.Background(), 12345, "", "Target Collection")
	if err != nil {
		t.Fatalf("GetCollectionByName failed: %v", err)
	}
	if coll == nil {
		t.Fatal("expected non-nil collection")
	}
	if coll.Key != "NAMEDCOLL1" {
		t.Errorf("expected key 'NAMEDCOLL1', got %q", coll.Key)
	}
	if coll.Data.Name != "Target Collection" {
		t.Errorf("expected name 'Target Collection', got %q", coll.Data.Name)
	}
}

func TestPgMock_CreateEmptyGroup(t *testing.T) {
	script := &pgmock.Script{
		Steps: append(
			pgmock.AcceptUnauthenticatedConnRequestSteps(),
			// 1. INSERT INTO groups
			pgmock.ExpectAnyMessage(&pgproto3.Parse{}),
			pgmock.ExpectAnyMessage(&pgproto3.Describe{}),
			pgmock.ExpectAnyMessage(&pgproto3.Sync{}),
			pgmock.SendMessage(&pgproto3.ParseComplete{}),
			pgmock.SendMessage(&pgproto3.ParameterDescription{ParameterOIDs: []uint32{20}}),
			pgmock.SendMessage(&pgproto3.NoData{}),
			pgmock.SendMessage(&pgproto3.ReadyForQuery{TxStatus: 'I'}),
			pgmock.ExpectAnyMessage(&pgproto3.Bind{}),
			pgmock.ExpectAnyMessage(&pgproto3.Execute{}),
			pgmock.ExpectAnyMessage(&pgproto3.Sync{}),
			pgmock.SendMessage(&pgproto3.BindComplete{}),
			pgmock.SendMessage(&pgproto3.CommandComplete{CommandTag: []byte("INSERT 0 1")}),
			pgmock.SendMessage(&pgproto3.ReadyForQuery{TxStatus: 'I'}),

			// 2. INSERT INTO syncgroups
			pgmock.ExpectAnyMessage(&pgproto3.Parse{}),
			pgmock.ExpectAnyMessage(&pgproto3.Describe{}),
			pgmock.ExpectAnyMessage(&pgproto3.Sync{}),
			pgmock.SendMessage(&pgproto3.ParseComplete{}),
			pgmock.SendMessage(&pgproto3.ParameterDescription{ParameterOIDs: []uint32{20, 16, 1043}}),
			pgmock.SendMessage(&pgproto3.NoData{}),
			pgmock.SendMessage(&pgproto3.ReadyForQuery{TxStatus: 'I'}),
			pgmock.ExpectAnyMessage(&pgproto3.Bind{}),
			pgmock.ExpectAnyMessage(&pgproto3.Execute{}),
			pgmock.ExpectAnyMessage(&pgproto3.Sync{}),
			pgmock.SendMessage(&pgproto3.BindComplete{}),
			pgmock.SendMessage(&pgproto3.CommandComplete{CommandTag: []byte("INSERT 0 1")}),
			pgmock.SendMessage(&pgproto3.ReadyForQuery{TxStatus: 'I'}),
		),
	}

	st, cleanup := startMockServer(t, script)
	defer cleanup()

	active, direction, err := st.CreateEmptyGroup(context.Background(), 99999)
	if err != nil {
		t.Fatalf("CreateEmptyGroup failed: %v", err)
	}
	if !active {
		t.Errorf("expected active to be true")
	}
	if direction != model.SyncDirection_ToLocal {
		t.Errorf("expected direction ToLocal, got %v", direction)
	}
}
