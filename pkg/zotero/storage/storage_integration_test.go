package storage

import (
	"testing"
	"time"

	"github.com/je4/zsync/v2/pkg/zotero/model"
)

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
	grp.Data = *sampleGroupData(groupID, "Test Integration Group")
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
	collData := sampleCollectionData("TESTCOLL1", "Top Level Collection", "")
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
	itemData := sampleItemData("ITEMKEY01", "Integration Test Book", "book")
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

	tag := sampleTag("integration-test-tag")

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
