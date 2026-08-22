package storage

import (
	"context"
	"testing"
	"time"

	"github.com/je4/zsync/v2/pkg/zotero/model"
)

func TestIntegration_GroupLifecycle(t *testing.T) {
	st, groupID, cleanup := getTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	// 1. CreateEmptyGroup
	active, direction, err := st.CreateEmptyGroup(ctx, groupID)
	if err != nil {
		t.Fatalf("CreateEmptyGroup failed: %v", err)
	}
	if !active {
		t.Errorf("expected active to be true")
	}
	if direction != model.SyncDirection_ToLocal {
		t.Errorf("expected direction ToLocal, got %v", direction)
	}

	// 2. GetGroup
	grp, err := st.GetGroup(ctx, groupID)
	if err != nil {
		t.Fatalf("GetGroup failed: %v", err)
	}
	if grp.Id != groupID {
		t.Errorf("expected group ID %v, got %v", groupID, grp.Id)
	}

	// 3. UpdateGroup
	grp.Version = 10
	grp.Data = *sampleGroupData(groupID, "Test Integration Group")
	if err := st.UpdateGroup(ctx, grp); err != nil {
		t.Fatalf("UpdateGroup failed: %v", err)
	}

	grpUpdated, err := st.GetGroup(ctx, groupID)
	if err != nil {
		t.Fatalf("GetGroup after update failed: %v", err)
	}
	if grpUpdated.Version != 10 {
		t.Errorf("expected version 10, got %v", grpUpdated.Version)
	}
	if grpUpdated.Data.Name != "Test Integration Group" {
		t.Errorf("expected name 'Test Integration Group', got %q", grpUpdated.Data.Name)
	}

	// 4. UpdateGroupGitlabTimestamp
	now := time.Now().Truncate(time.Second)
	if err := st.UpdateGroupGitlabTimestamp(ctx, groupID, now); err != nil {
		t.Fatalf("UpdateGroupGitlabTimestamp failed: %v", err)
	}

	grpWithGitlab, err := st.GetGroup(ctx, groupID)
	if err != nil {
		t.Fatalf("GetGroup after gitlab timestamp failed: %v", err)
	}
	if grpWithGitlab.Gitlab == nil {
		t.Errorf("expected Gitlab timestamp to be set")
	}

	// 5. ClearGroup
	if err := st.ClearGroup(ctx, groupID); err != nil {
		t.Fatalf("ClearGroup failed: %v", err)
	}
	grpCleared, err := st.GetGroup(ctx, groupID)
	if err != nil {
		t.Fatalf("GetGroup after clear failed: %v", err)
	}
	if grpCleared.Version != 0 {
		t.Errorf("expected version 0 after clear, got %v", grpCleared.Version)
	}
}

func TestIntegration_CollectionLifecycle(t *testing.T) {
	st, groupID, cleanup := getTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	_, _, err := st.CreateEmptyGroup(ctx, groupID)
	if err != nil {
		t.Fatalf("failed to create test group: %v", err)
	}

	// 1. Create Collection
	collData := sampleCollectionData("COLLKEY1", "Top Level Collection", "")
	coll, err := st.CreateCollection(ctx, groupID, collData)
	if err != nil {
		t.Fatalf("CreateCollection failed: %v", err)
	}
	if coll.Key != "COLLKEY1" {
		t.Errorf("expected key COLLKEY1, got %v", coll.Key)
	}

	// 2. GetCollectionByKey
	loadedColl, err := st.GetCollectionByKey(ctx, groupID, "COLLKEY1")
	if err != nil {
		t.Fatalf("GetCollectionByKey failed: %v", err)
	}
	if loadedColl == nil || loadedColl.Data.Name != "Top Level Collection" {
		t.Errorf("unexpected loaded collection: %+v", loadedColl)
	}

	// 3. GetCollectionVersion
	ver, status, err := st.GetCollectionVersion(ctx, groupID, "COLLKEY1")
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
	if err := st.UpdateCollection(ctx, groupID, loadedColl); err != nil {
		t.Fatalf("UpdateCollection failed: %v", err)
	}

	updatedColl, err := st.GetCollectionByKey(ctx, groupID, "COLLKEY1")
	if err != nil {
		t.Fatalf("GetCollectionByKey after update failed: %v", err)
	}
	if updatedColl.Version != 5 || updatedColl.Data.Name != "Updated Collection Name" {
		t.Errorf("unexpected updated collection: %+v", updatedColl)
	}

	// 5. GetCollectionsByKey
	colls, err := st.GetCollectionsByKey(ctx, groupID, []string{"COLLKEY1"})
	if err != nil {
		t.Fatalf("GetCollectionsByKey failed: %v", err)
	}
	if len(colls) != 1 {
		t.Errorf("expected 1 collection, got %v", len(colls))
	}

	// 6. IterateCollections
	iterCount := 0
	err = st.IterateCollections(ctx, groupID, nil, func(c *model.Collection) error {
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
	if err := st.DeleteCollection(ctx, groupID, "COLLKEY1"); err != nil {
		t.Fatalf("DeleteCollection failed: %v", err)
	}
	deletedColl, err := st.GetCollectionByKey(ctx, groupID, "COLLKEY1")
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
	ctx := context.Background()

	_, _, err := st.CreateEmptyGroup(ctx, groupID)
	if err != nil {
		t.Fatalf("failed to create test group: %v", err)
	}

	// 1. Create Item
	itemData := sampleItemData("ITEMKEY1", "Integration Test Book", "book")
	itemMeta := &model.ItemMeta{
		CreatorSummary: "Doe",
		NumChildren:    0,
	}

	item, err := st.CreateItem(ctx, groupID, itemData, itemMeta, "old-book-01")
	if err != nil {
		t.Fatalf("CreateItem failed: %v", err)
	}
	if item.Key != "ITEMKEY1" {
		t.Errorf("expected key ITEMKEY1, got %v", item.Key)
	}

	// 2. GetItemByKey & GetItemByOldid
	byKey, err := st.GetItemByKey(ctx, groupID, "ITEMKEY1")
	if err != nil {
		t.Fatalf("GetItemByKey failed: %v", err)
	}
	if byKey == nil || byKey.Data.Title != "Integration Test Book" {
		t.Errorf("unexpected item by key: %+v", byKey)
	}

	byOldID, err := st.GetItemByOldid(ctx, groupID, "old-book-01")
	if err != nil {
		t.Fatalf("GetItemByOldid failed: %v", err)
	}
	if byOldID == nil || byOldID.Key != "ITEMKEY1" {
		t.Errorf("unexpected item by oldid: %+v", byOldID)
	}

	// 3. UpdateItem
	byKey.Version = 2
	byKey.Data.Title = "Updated Book Title"
	byKey.Status = model.SyncStatus_Synced
	if err := st.UpdateItem(ctx, groupID, byKey); err != nil {
		t.Fatalf("UpdateItem failed: %v", err)
	}

	updatedItem, err := st.GetItemByKey(ctx, groupID, "ITEMKEY1")
	if err != nil {
		t.Fatalf("GetItemByKey after update failed: %v", err)
	}
	if updatedItem.Version != 2 || updatedItem.Data.Title != "Updated Book Title" {
		t.Errorf("unexpected item after update: %+v", updatedItem)
	}

	// 4. GetItemsByKey & GetItemVersions
	itemsList, err := st.GetItemsByKey(ctx, groupID, []string{"ITEMKEY1"})
	if err != nil {
		t.Fatalf("GetItemsByKey failed: %v", err)
	}
	if len(itemsList) != 1 {
		t.Errorf("expected 1 item, got %v", len(itemsList))
	}

	versions, maxVer, err := st.GetItemVersions(ctx, groupID, 0, false)
	if err != nil {
		t.Fatalf("GetItemVersions failed: %v", err)
	}
	if maxVer < 2 || versions["ITEMKEY1"] != 2 {
		t.Errorf("unexpected versions result: maxVer=%v, map=%v", maxVer, versions)
	}

	// 5. IterateItems
	count := 0
	err = st.IterateItems(ctx, groupID, nil, func(it *model.Item) error {
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
	if err := st.DeleteItemRecursive(ctx, groupID, "ITEMKEY1"); err != nil {
		t.Fatalf("DeleteItemRecursive failed: %v", err)
	}

	delItem, err := st.GetItemByKey(ctx, groupID, "ITEMKEY1")
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
	ctx := context.Background()

	_, _, err := st.CreateEmptyGroup(ctx, groupID)
	if err != nil {
		t.Fatalf("failed to create test group: %v", err)
	}

	tag := sampleTag("integration-test-tag")

	// 1. CreateTag
	if err := st.CreateTag(ctx, groupID, tag); err != nil {
		t.Fatalf("CreateTag failed: %v", err)
	}

	// 2. Duplicate CreateTag should not return error (handled via unique violation)
	if err := st.CreateTag(ctx, groupID, tag); err != nil {
		t.Fatalf("duplicate CreateTag failed: %v", err)
	}

	// 3. DeleteTag
	if err := st.DeleteTag(ctx, groupID, "integration-test-tag"); err != nil {
		t.Fatalf("DeleteTag failed: %v", err)
	}
}
