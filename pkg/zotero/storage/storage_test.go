package storage

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/je4/zsync/v2/pkg/zotero/model"
	"github.com/rs/zerolog"
)

func loadEnv() {
	if os.Getenv("DATABASE_URL") != "" {
		return
	}
	candidates := []string{".env", "../.env", "../../.env", "../../../.env"}
	for _, c := range candidates {
		absPath, err := filepath.Abs(c)
		if err != nil {
			continue
		}
		f, err := os.Open(absPath)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				k := strings.TrimSpace(parts[0])
				v := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
				if os.Getenv(k) == "" {
					os.Setenv(k, v)
				}
			}
		}
		f.Close()
		if os.Getenv("DATABASE_URL") != "" {
			break
		}
	}
}

func ensureSchema(ctx context.Context, pool *pgxpool.Pool, schema string) error {
	ddl := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s.groups (
    id bigint NOT NULL PRIMARY KEY,
    version bigint NOT NULL DEFAULT 0,
    created timestamp with time zone NOT NULL DEFAULT NOW(),
    modified timestamp with time zone NOT NULL DEFAULT NOW(),
    data text,
    deleted boolean NOT NULL DEFAULT false,
    itemversion bigint NOT NULL DEFAULT 0,
    collectionversion bigint NOT NULL DEFAULT 0,
    tagversion bigint NOT NULL DEFAULT 0,
    gitlab timestamp with time zone
);

CREATE TABLE IF NOT EXISTS %s.syncgroups (
    id bigint NOT NULL,
    active boolean DEFAULT true NOT NULL,
    direction varchar(32) DEFAULT 'none' NOT NULL,
    tags boolean DEFAULT false NOT NULL,
    CONSTRAINT syncgroups_pkey PRIMARY KEY (id)
);

CREATE TABLE IF NOT EXISTS %s.collections (
    key varchar(32) NOT NULL,
    version bigint DEFAULT 0 NOT NULL,
    library bigint NOT NULL,
    sync varchar(32) NOT NULL,
    data text,
    meta text,
    deleted boolean DEFAULT false NOT NULL,
    modified timestamp with time zone DEFAULT NOW(),
    gitlab timestamp with time zone,
    PRIMARY KEY (library, key)
);

CREATE TABLE IF NOT EXISTS %s.items (
    key varchar(32) NOT NULL,
    version bigint DEFAULT 0 NOT NULL,
    library bigint NOT NULL,
    sync varchar(32) NOT NULL,
    data text,
    meta text,
    oldid varchar(128),
    trashed boolean DEFAULT false NOT NULL,
    deleted boolean DEFAULT false NOT NULL,
    md5 varchar(64),
    modified timestamp with time zone DEFAULT NOW(),
    gitlab timestamp with time zone,
    PRIMARY KEY (library, key),
    CONSTRAINT items_oldid_constraint UNIQUE (library, oldid)
);

CREATE TABLE IF NOT EXISTS %s.tags (
    tag varchar(255) NOT NULL,
    meta text,
    library bigint NOT NULL,
    CONSTRAINT pk_tags PRIMARY KEY (library, tag)
);

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_matviews WHERE schemaname = '%s' AND matviewname = 'collection_name_hier') THEN
        EXECUTE 'CREATE MATERIALIZED VIEW %s.collection_name_hier AS SELECT key, library, (data::json->>''name'') AS name, (data::json->>''parentCollection'') AS parent FROM %s.collections';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_matviews WHERE schemaname = '%s' AND matviewname = 'item_type_hier') THEN
        EXECUTE 'CREATE MATERIALIZED VIEW %s.item_type_hier AS SELECT key, library, (data::json->>''itemType'') AS type, (data::json->>''parentItem'') AS parent FROM %s.items';
    END IF;
END $$;

CREATE OR REPLACE FUNCTION %s.refresh_item_type_hier() RETURNS void AS $$
BEGIN
    REFRESH MATERIALIZED VIEW %s.item_type_hier WITH DATA;
END;
$$ LANGUAGE plpgsql;
`, schema, schema, schema, schema, schema, schema, schema, schema, schema, schema, schema, schema, schema)

	_, err := pool.Exec(ctx, ddl)
	return err
}

func getTestStorage(t *testing.T) (*Storage, int64, func()) {
	t.Helper()
	loadEnv()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL is not set; skipping integration tests")
		return nil, 0, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Skipf("cannot create connection pool (%v); skipping integration tests", err)
		return nil, 0, nil
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("cannot ping database (%v); skipping integration tests", err)
		return nil, 0, nil
	}

	schema := "public"
	if s := os.Getenv("DATABASE_SCHEMA"); s != "" {
		schema = s
	}

	if err := ensureSchema(ctx, pool, schema); err != nil {
		pool.Close()
		t.Skipf("cannot ensure schema (%v); skipping integration tests", err)
		return nil, 0, nil
	}

	zlog := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).With().Timestamp().Logger()
	st := NewStorage(pool, schema, true, &zlog)

	testGroupID := int64(88880000 + (time.Now().UnixNano() % 10000))

	// Clean up any stale records for this test group
	cleanupGroup := func() {
		bgCtx := context.Background()
		_, _ = pool.Exec(bgCtx, fmt.Sprintf("DELETE FROM %s.tags WHERE library=$1", schema), testGroupID)
		_, _ = pool.Exec(bgCtx, fmt.Sprintf("DELETE FROM %s.items WHERE library=$1", schema), testGroupID)
		_, _ = pool.Exec(bgCtx, fmt.Sprintf("DELETE FROM %s.collections WHERE library=$1", schema), testGroupID)
		_, _ = pool.Exec(bgCtx, fmt.Sprintf("DELETE FROM %s.syncgroups WHERE id=$1", schema), testGroupID)
		_, _ = pool.Exec(bgCtx, fmt.Sprintf("DELETE FROM %s.groups WHERE id=$1", schema), testGroupID)
	}
	cleanupGroup()

	cleanup := func() {
		cleanupGroup()
		pool.Close()
	}

	return st, testGroupID, cleanup
}

func TestIsEmptyResult(t *testing.T) {
	if !IsEmptyResult(sql.ErrNoRows) {
		t.Errorf("expected IsEmptyResult(sql.ErrNoRows) to be true")
	}
	if !IsEmptyResult(pgx.ErrNoRows) {
		t.Errorf("expected IsEmptyResult(pgx.ErrNoRows) to be true")
	}
	if !IsEmptyResult(fmt.Errorf("wrapped error: %w", pgx.ErrNoRows)) {
		t.Errorf("expected IsEmptyResult(wrapped pgx.ErrNoRows) to be true")
	}
	if IsEmptyResult(errors.New("other error")) {
		t.Errorf("expected IsEmptyResult(other error) to be false")
	}
	if IsEmptyResult(nil) {
		t.Errorf("expected IsEmptyResult(nil) to be false")
	}
}

func TestIsUniqueViolation(t *testing.T) {
	pgErr := &pgconn.PgError{
		Code:           "23505",
		ConstraintName: "pk_tags",
	}
	if !IsUniqueViolation(pgErr, "pk_tags") {
		t.Errorf("expected IsUniqueViolation with matching constraint to be true")
	}
	if !IsUniqueViolation(pgErr, "") {
		t.Errorf("expected IsUniqueViolation with empty constraint to be true")
	}
	if IsUniqueViolation(pgErr, "other_constraint") {
		t.Errorf("expected IsUniqueViolation with different constraint to be false")
	}

	wrappedPgErr := fmt.Errorf("wrapped pg error: %w", pgErr)
	if !IsUniqueViolation(wrappedPgErr, "pk_tags") {
		t.Errorf("expected IsUniqueViolation with wrapped error to be true")
	}

	otherPgErr := &pgconn.PgError{
		Code:           "23503",
		ConstraintName: "pk_tags",
	}
	if IsUniqueViolation(otherPgErr, "pk_tags") {
		t.Errorf("expected IsUniqueViolation with code 23503 to be false")
	}

	regularErr := errors.New("some error")
	if IsUniqueViolation(regularErr, "") {
		t.Errorf("expected IsUniqueViolation for non-pg error to be false")
	}
	if IsUniqueViolation(nil, "") {
		t.Errorf("expected IsUniqueViolation for nil error to be false")
	}
}

func TestStorageAccessors(t *testing.T) {
	st := NewStorage(nil, "public", true, nil)
	if st.GetPool() != nil {
		t.Errorf("expected GetPool() to be nil")
	}
	if st.GetDB() != nil {
		t.Errorf("expected GetDB() to be nil")
	}
	if st.GetSchema() != "public" {
		t.Errorf("expected GetSchema() to be 'public', got %q", st.GetSchema())
	}
}

func TestIntegration_GroupLifecycle(t *testing.T) {
	st, groupID, cleanup := getTestStorage(t)
	defer cleanup()

	// 1. CreateEmptyGroup
	active, direction, err := st.CreateEmptyGroup(groupID)
	if err != nil {
		t.Fatalf("CreateEmptyGroup failed: %v", err)
	}
	if !active {
		t.Errorf("expected active to be true")
	}
	if direction != model.SyncDirection_ToLocal {
		t.Errorf("expected direction ToLocal, got %v", direction)
	}

	// 2. LoadGroup
	grp, err := st.LoadGroup(groupID)
	if err != nil {
		t.Fatalf("LoadGroup failed: %v", err)
	}
	if grp.Id != groupID {
		t.Errorf("expected group ID %v, got %v", groupID, grp.Id)
	}

	// 3. UpdateGroup
	grp.Version = 10
	grp.Data.Name = "Test Integration Group"
	if err := st.UpdateGroup(grp); err != nil {
		t.Fatalf("UpdateGroup failed: %v", err)
	}

	grpUpdated, err := st.LoadGroup(groupID)
	if err != nil {
		t.Fatalf("LoadGroup after update failed: %v", err)
	}
	if grpUpdated.Version != 10 {
		t.Errorf("expected version 10, got %v", grpUpdated.Version)
	}
	if grpUpdated.Data.Name != "Test Integration Group" {
		t.Errorf("expected name 'Test Integration Group', got %q", grpUpdated.Data.Name)
	}

	// 4. UpdateGroupGitlabTimestamp
	now := time.Now().Truncate(time.Second)
	if err := st.UpdateGroupGitlabTimestamp(groupID, now); err != nil {
		t.Fatalf("UpdateGroupGitlabTimestamp failed: %v", err)
	}

	grpWithGitlab, err := st.LoadGroup(groupID)
	if err != nil {
		t.Fatalf("LoadGroup after gitlab timestamp failed: %v", err)
	}
	if grpWithGitlab.Gitlab == nil {
		t.Errorf("expected Gitlab timestamp to be set")
	}

	// 5. ClearGroup
	if err := st.ClearGroup(groupID); err != nil {
		t.Fatalf("ClearGroup failed: %v", err)
	}
	grpCleared, err := st.LoadGroup(groupID)
	if err != nil {
		t.Fatalf("LoadGroup after clear failed: %v", err)
	}
	if grpCleared.Version != 0 {
		t.Errorf("expected version 0 after clear, got %v", grpCleared.Version)
	}
}

func TestIntegration_CollectionLifecycle(t *testing.T) {
	st, groupID, cleanup := getTestStorage(t)
	defer cleanup()

	_, _, err := st.CreateEmptyGroup(groupID)
	if err != nil {
		t.Fatalf("failed to create test group: %v", err)
	}

	// 1. Create Collection
	collData := &model.CollectionData{
		Key:              "TESTCOLL1",
		Name:             "Top Level Collection",
		ParentCollection: "",
	}
	coll, err := st.CreateCollection(groupID, collData)
	if err != nil {
		t.Fatalf("CreateCollection failed: %v", err)
	}
	if coll.Key != "TESTCOLL1" {
		t.Errorf("expected key TESTCOLL1, got %v", coll.Key)
	}

	// 2. GetCollectionByKey
	loadedColl, err := st.GetCollectionByKey(groupID, "TESTCOLL1")
	if err != nil {
		t.Fatalf("GetCollectionByKey failed: %v", err)
	}
	if loadedColl == nil || loadedColl.Data.Name != "Top Level Collection" {
		t.Errorf("unexpected loaded collection: %+v", loadedColl)
	}

	// 3. GetCollectionVersion
	ver, status, err := st.GetCollectionVersion(groupID, "TESTCOLL1")
	if err != nil {
		t.Fatalf("GetCollectionVersion failed: %v", err)
	}
	if status != model.SyncStatus_New {
		t.Errorf("expected status New, got %v", status)
	}
	if ver != 0 {
		t.Errorf("expected version 0, got %v", ver)
	}

	// 4. UpdateCollection
	loadedColl.Version = 5
	loadedColl.Status = model.SyncStatus_Synced
	loadedColl.Data.Name = "Updated Collection Name"
	if err := st.UpdateCollection(groupID, loadedColl); err != nil {
		t.Fatalf("UpdateCollection failed: %v", err)
	}

	updatedColl, err := st.GetCollectionByKey(groupID, "TESTCOLL1")
	if err != nil {
		t.Fatalf("GetCollectionByKey after update failed: %v", err)
	}
	if updatedColl.Version != 5 || updatedColl.Data.Name != "Updated Collection Name" {
		t.Errorf("unexpected updated collection: %+v", updatedColl)
	}

	// 5. GetCollections
	colls, err := st.GetCollections(groupID, []string{"TESTCOLL1"})
	if err != nil {
		t.Fatalf("GetCollections failed: %v", err)
	}
	if len(*colls) != 1 {
		t.Errorf("expected 1 collection, got %v", len(*colls))
	}

	// 6. IterateCollections
	iterCount := 0
	err = st.IterateCollections(groupID, nil, func(c *model.Collection) error {
		iterCount++
		return nil
	})
	if err != nil {
		t.Fatalf("IterateCollections failed: %v", err)
	}
	if iterCount < 1 {
		t.Errorf("expected at least 1 iterated collection, got %v", iterCount)
	}

	// 7. DeleteCollection
	if err := st.DeleteCollection(groupID, "TESTCOLL1"); err != nil {
		t.Fatalf("DeleteCollection failed: %v", err)
	}
	deletedColl, err := st.GetCollectionByKey(groupID, "TESTCOLL1")
	if err != nil {
		t.Fatalf("GetCollectionByKey after delete failed: %v", err)
	}
	if !deletedColl.Deleted {
		t.Errorf("expected collection to be marked deleted")
	}
}

func TestIntegration_ItemLifecycle(t *testing.T) {
	st, groupID, cleanup := getTestStorage(t)
	defer cleanup()

	_, _, err := st.CreateEmptyGroup(groupID)
	if err != nil {
		t.Fatalf("failed to create test group: %v", err)
	}

	// 1. Create Item
	itemData := &model.ItemGeneric{}
	itemData.Key = "ITEMKEY01"
	itemData.ItemType = "book"
	itemData.Title = "Integration Test Book"
	itemData.Creators = []model.ItemDataPerson{
		{
			CreatorType: "author",
			FirstName:   "John",
			LastName:    "Doe",
		},
	}
	itemMeta := &model.ItemMeta{
		CreatorSummary: "Doe",
		NumChildren:    0,
	}

	item, err := st.CreateItem(groupID, itemData, itemMeta, "old-book-01")
	if err != nil {
		t.Fatalf("CreateItem failed: %v", err)
	}
	if item.Key != "ITEMKEY01" {
		t.Errorf("expected key ITEMKEY01, got %v", item.Key)
	}

	// 2. GetItemByKey & GetItemByOldid
	byKey, err := st.GetItemByKey(groupID, "ITEMKEY01")
	if err != nil {
		t.Fatalf("GetItemByKey failed: %v", err)
	}
	if byKey == nil || byKey.Data.Title != "Integration Test Book" {
		t.Errorf("unexpected item by key: %+v", byKey)
	}

	byOldID, err := st.GetItemByOldid(groupID, "old-book-01")
	if err != nil {
		t.Fatalf("GetItemByOldid failed: %v", err)
	}
	if byOldID == nil || byOldID.Key != "ITEMKEY01" {
		t.Errorf("unexpected item by oldid: %+v", byOldID)
	}

	// 3. UpdateItem
	byKey.Version = 2
	byKey.Data.Title = "Updated Book Title"
	byKey.Status = model.SyncStatus_Synced
	if err := st.UpdateItem(groupID, byKey); err != nil {
		t.Fatalf("UpdateItem failed: %v", err)
	}

	updatedItem, err := st.GetItemByKey(groupID, "ITEMKEY01")
	if err != nil {
		t.Fatalf("GetItemByKey after update failed: %v", err)
	}
	if updatedItem.Version != 2 || updatedItem.Data.Title != "Updated Book Title" {
		t.Errorf("unexpected item after update: %+v", updatedItem)
	}

	// 4. GetItems & GetItemsVersion
	itemsList, err := st.GetItems(groupID, []string{"ITEMKEY01"})
	if err != nil {
		t.Fatalf("GetItems failed: %v", err)
	}
	if len(*itemsList) != 1 {
		t.Errorf("expected 1 item, got %v", len(*itemsList))
	}

	versions, maxVer, err := st.GetItemsVersion(groupID, 0, false)
	if err != nil {
		t.Fatalf("GetItemsVersion failed: %v", err)
	}
	if maxVer < 2 || (*versions)["ITEMKEY01"] != 2 {
		t.Errorf("unexpected versions result: maxVer=%v, map=%v", maxVer, *versions)
	}

	// 5. IterateItems
	count := 0
	err = st.IterateItems(groupID, nil, func(it *model.Item) error {
		count++
		return nil
	})
	if err != nil {
		t.Fatalf("IterateItems failed: %v", err)
	}
	if count < 1 {
		t.Errorf("expected at least 1 item in iteration, got %v", count)
	}

	// 6. DeleteItemRecursive
	if err := st.DeleteItemRecursive(groupID, "ITEMKEY01"); err != nil {
		t.Fatalf("DeleteItemRecursive failed: %v", err)
	}

	delItem, err := st.GetItemByKey(groupID, "ITEMKEY01")
	if err != nil {
		t.Fatalf("GetItemByKey after delete failed: %v", err)
	}
	if !delItem.Deleted {
		t.Errorf("expected item to be marked deleted")
	}
}

func TestIntegration_TagLifecycle(t *testing.T) {
	st, groupID, cleanup := getTestStorage(t)
	defer cleanup()

	_, _, err := st.CreateEmptyGroup(groupID)
	if err != nil {
		t.Fatalf("failed to create test group: %v", err)
	}

	tag := model.Tag{
		Tag: "integration-test-tag",
		Meta: &model.TagMeta{
			Type: 0,
		},
	}

	// 1. CreateTag
	if err := st.CreateTag(groupID, tag); err != nil {
		t.Fatalf("CreateTag failed: %v", err)
	}

	// 2. Duplicate CreateTag should not return error (handled via unique violation)
	if err := st.CreateTag(groupID, tag); err != nil {
		t.Fatalf("duplicate CreateTag failed: %v", err)
	}

	// 3. DeleteTag
	if err := st.DeleteTag(groupID, "integration-test-tag"); err != nil {
		t.Fatalf("DeleteTag failed: %v", err)
	}
}
