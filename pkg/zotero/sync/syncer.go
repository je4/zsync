package sync

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"emperror.dev/errors"
	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/je4/zsync/v2/pkg/filesystem"
	"github.com/je4/zsync/v2/pkg/zotero/client"
	"github.com/je4/zsync/v2/pkg/zotero/model"
	"github.com/je4/zsync/v2/pkg/zotero/storage"
)

// Syncer coordinates synchronization between Zotero, PostgreSQL, and the
// optional attachment filesystem.
type Syncer struct {
	Client  *client.Client
	Storage *storage.Storage
	Fs      filesystem.FileSystem
	Logger  zLogger.ZLogger
}

// NewSyncer creates a synchronizer from its transport, persistence, filesystem,
// and logging dependencies. Client, storage, and filesystem may be nil when a
// caller only needs the corresponding subset of functionality.
func NewSyncer(client *client.Client, storage *storage.Storage, fs filesystem.FileSystem, logger zLogger.ZLogger) *Syncer {
	return &Syncer{
		Client:  client,
		Storage: storage,
		Fs:      fs,
		Logger:  logger,
	}
}

// GetGroupBucket returns, and creates when necessary, the attachment bucket
// belonging to a Zotero group.
func (s *Syncer) GetGroupBucket(groupId int64) (string, error) {
	if s.Fs == nil {
		return "", errors.New("no filesystem configured")
	}
	bucket := fmt.Sprintf("zotero-%v", groupId)
	found, err := s.Fs.FolderExists(bucket)
	if err != nil {
		return "", errors.Wrap(err, "cannot check bucket existence")
	}
	if !found {
		if err := s.Fs.FolderCreate(bucket, filesystem.FolderCreateOptions{}); err != nil {
			return "", errors.Wrapf(err, "cannot create bucket %s", bucket)
		}
	}
	return bucket, nil
}

// SyncGroup runs the complete group pipeline and persists new synchronization
// cursors only after all stages succeed.
func (s *Syncer) SyncGroup(ctx context.Context, group *model.Group) error {
	if s.Logger != nil {
		s.Logger.Info().Msgf("Syncing Group #%v - %v", group.Id, group.Data.Name)
	}

	_, collectionVersion, err := s.SyncCollections(ctx, group)
	if err != nil {
		return errors.Wrapf(err, "cannot sync collections of Group %v", group.Id)
	}

	_, itemVersion, err := s.UploadItems(ctx, group)
	if err != nil {
		return errors.Wrapf(err, "cannot sync items of Group %v", group.Id)
	}

	_, itemVersion, err = s.DownloadItems(ctx, group)
	if err != nil {
		return errors.Wrapf(err, "cannot sync items of Group %v", group.Id)
	}

	_, tagVersion, err := s.SyncTags(ctx, group)
	if err != nil {
		return errors.Wrapf(err, "cannot sync tags of Group %v", group.Id)
	}

	_, err = s.SyncDeleted(ctx, group)
	if err != nil {
		return errors.Wrapf(err, "cannot sync deleted of Group %v", group.Id)
	}

	// change to new version if everything was ok
	if collectionVersion > group.CollectionVersion {
		group.CollectionVersion = collectionVersion
		group.IsModified = true
	}
	if itemVersion > group.ItemVersion {
		group.ItemVersion = itemVersion
		group.IsModified = true
	}
	if tagVersion > group.TagVersion {
		group.TagVersion = tagVersion
		group.IsModified = true
	}

	if group.IsModified && s.Storage != nil {
		if err := s.Storage.UpdateGroup(ctx, group); err != nil {
			return errors.Wrapf(err, "cannot update group in storage")
		}
	}

	return nil
}

// SyncCollections uploads local collection changes and downloads newer remote
// collections according to the group's sync direction. It returns the number
// of changes and the newest remote version.
func (s *Syncer) SyncCollections(ctx context.Context, group *model.Group) (int64, int64, error) {
	if s.Client == nil || s.Storage == nil {
		return 0, 0, nil
	}
	var num, num2 int64
	var err error
	var lastModifiedVersion int64

	// upload data
	if group.CanUpload() {
		num, err = s.syncModifiedCollections(ctx, group)
		if err != nil {
			return 0, 0, err
		}
	}

	if group.CanDownload() {
		num2, lastModifiedVersion, err = s.syncCollections(ctx, group)
		if err != nil {
			return 0, 0, err
		}
	}

	counter := num + num2
	if counter > 0 {
		if err := s.Storage.RefreshCollectionNameHier(ctx); err != nil {
			return counter, 0, err
		}
	}

	return counter, lastModifiedVersion, nil
}

func (s *Syncer) syncModifiedCollections(ctx context.Context, group *model.Group) (int64, error) {
	var counter int64
	colls, err := s.Storage.GetModifiedCollections(ctx, group.Id)
	if err != nil {
		return 0, err
	}

	for _, coll := range colls {
		counter++
		if s.Logger != nil {
			s.Logger.Info().Msgf("writing collection %v of %v to zotero cloud", counter, len(colls))
		}
		var lastModifiedVersion int64 = coll.Version
		if coll.Deleted {
			if err := s.Client.DeleteCollection(ctx, group.Id, coll.Key, lastModifiedVersion); err != nil {
				return 0, errors.Wrapf(err, "error deleting collection %v.%v", group.Id, coll.Key)
			}
			coll.Status = model.SyncStatus_Synced
			if err := s.Storage.UpdateCollection(ctx, group.Id, coll); err != nil {
				return 0, errors.Wrapf(err, "cannot update deleted collection in storage")
			}
		} else {
			successKey, err := s.Client.UpdateCollection(ctx, group.Id, &coll.Data, &lastModifiedVersion)
			if err != nil {
				return 0, errors.Wrapf(err, "error creating/updating collection %v.%v", group.Id, coll.Key)
			}
			coll.Key = successKey
			coll.Data.Key = successKey
			coll.Version = lastModifiedVersion
			coll.Data.Version = lastModifiedVersion
			coll.Status = model.SyncStatus_Synced
			if err := s.Storage.UpdateCollection(ctx, group.Id, coll); err != nil {
				return 0, errors.Wrapf(err, "cannot update collection in storage")
			}
		}
	}
	return counter, nil
}

func (s *Syncer) syncCollections(ctx context.Context, group *model.Group) (int64, int64, error) {
	if s.Logger != nil {
		s.Logger.Info().Msgf("Syncing collections of Group #%v", group.Id)
	}

	var counter int64
	objectList, lastModifiedVersion, err := s.Client.GetCollectionVersions(ctx, group.Id, group.CollectionVersion)
	if err != nil {
		return counter, 0, errors.Wrapf(err, "cannot get collection versions")
	}
	collectionUpdate := []string{}
	for collectionid, version := range objectList {
		oldversion, status, err := s.Storage.GetCollectionVersion(ctx, group.Id, collectionid)
		if err != nil {
			return counter, 0, errors.Wrapf(err, "cannot get version of collection %v from database: %v", collectionid, err)
		}
		if status != model.SyncStatus_Synced && status != model.SyncStatus_Incomplete {
			return counter, lastModifiedVersion, errors.Errorf("collection %v not synced. please handle conflict", collectionid)
		}
		if oldversion < version {
			collectionUpdate = append(collectionUpdate, collectionid)
		}
	}
	for part := range slices.Chunk(collectionUpdate, 50) {
		collections, _, err := s.Client.GetCollectionsByKey(ctx, group.Id, part)
		if err != nil {
			return counter, 0, errors.Wrapf(err, "cannot get collections")
		}
		if s.Logger != nil {
			s.Logger.Info().Msgf("%v collections", len(collections))
		}
		for _, coll := range collections {
			coll.Status = model.SyncStatus_Synced
			if err := s.Storage.UpdateCollection(ctx, group.Id, &coll); err != nil {
				return counter, 0, errors.Wrapf(err, "cannot update collections")
			}
			counter++
		}
	}
	if s.Logger != nil {
		s.Logger.Info().Msgf("Syncing collections of Group #%v done. %v collections changed", group.Id, counter)
	}
	return counter, lastModifiedVersion, nil
}

// UploadItems sends locally modified items and imported attachment files to
// Zotero when the group's direction permits cloud writes.
func (s *Syncer) UploadItems(ctx context.Context, group *model.Group) (int64, int64, error) {
	if s.Client == nil || s.Storage == nil {
		return 0, 0, nil
	}
	var counter int64
	var lastModifiedVersion int64

	if group.CanUpload() {
		_, lmv, err := s.Client.GetItemVersions(ctx, group.Id, 0, false)
		if err != nil {
			return 0, 0, errors.Wrapf(err, "cannot get last modified item version")
		}
		lastModifiedVersion = lmv
		num4, err := s.syncModifiedItems(ctx, group, lastModifiedVersion)
		if err != nil {
			return 0, 0, err
		}
		counter += num4
	}

	if counter > 0 {
		if err := s.Storage.RefreshItemTypeHier(ctx); err != nil {
			return counter, 0, err
		}
	}

	return counter, lastModifiedVersion, nil
}

func (s *Syncer) syncModifiedItems(ctx context.Context, group *model.Group, lastModifiedVersion int64) (int64, error) {
	var counter int64
	items, err := s.Storage.GetModifiedItems(ctx, group.Id)
	if err != nil {
		return 0, err
	}

	for _, item := range items {
		counter++
		if s.Logger != nil {
			s.Logger.Info().Msgf("writing item %v of %v to zotero cloud", counter, len(items))
		}

		if item.Deleted {
			if err := s.Client.DeleteItem(ctx, group.Id, item.Key, lastModifiedVersion); err != nil {
				return 0, errors.Wrapf(err, "delete item %v failed", item.Key)
			}
			item.Status = model.SyncStatus_Synced
			if err := s.Storage.UpdateItem(ctx, group.Id, item); err != nil {
				return 0, errors.Wrapf(err, "cannot update deleted item in storage")
			}
			continue
		}

		// Handle attachment upload if needed
		if item.Data.ItemType == "attachment" && item.Data.LinkMode == "imported_file" && s.Fs != nil {
			bucket, err := s.GetGroupBucket(group.Id)
			if err != nil {
				return 0, errors.Wrap(err, "cannot get group bucket")
			}
			fileReader, err := s.Fs.FileOpenRead(bucket, item.Key, filesystem.FileGetOptions{})
			if err == nil {
				defer fileReader.Close()
				data, readErr := io.ReadAll(fileReader)
				if readErr == nil {
					fInfo, statErr := s.Fs.FileStat(bucket, item.Key, filesystem.FileStatOptions{})
					var mtime int64
					if statErr == nil && fInfo != nil {
						mtime = fInfo.ModTime().UnixNano() / int64(time.Millisecond)
					}
					uploadedMd5, upErr := s.Client.UploadAttachment(ctx, group.Id, item.Key, data, item.Data.Filename, item.Data.ContentType, mtime, item.MD5)
					if upErr != nil {
						if s.Logger != nil {
							s.Logger.Warn().Err(upErr).Msgf("error uploading attachment for item %v", item.Key)
						}
					} else {
						item.MD5 = uploadedMd5
					}
				}
			}
		}

		successKey, err := s.Client.UpdateItem(ctx, group.Id, &item.Data, &lastModifiedVersion)
		if err != nil {
			if s.Logger != nil {
				s.Logger.Warn().Msgf("error creating/updating item %v.%v - retrying with new version", group.Id, item.Key)
			}
			successKey, err = s.Client.UpdateItem(ctx, group.Id, &item.Data, &lastModifiedVersion)
			if err != nil {
				if s.Logger != nil {
					s.Logger.Error().Msgf("error creating/updating item %v.%v: %v", group.Id, item.Key, err)
				}
				continue
			}
		}
		item.Key = successKey
		item.Data.Key = successKey
		item.Version = lastModifiedVersion
		item.Data.Version = lastModifiedVersion
		item.Status = model.SyncStatus_Synced
		if err := s.Storage.UpdateItem(ctx, group.Id, item); err != nil {
			return 0, errors.Wrapf(err, "cannot update item in storage")
		}
	}
	return counter, nil
}

// DownloadItems fetches newer regular and trashed items, including imported
// attachment files, when the group's direction permits local writes.
func (s *Syncer) DownloadItems(ctx context.Context, group *model.Group) (int64, int64, error) {
	if s.Client == nil || s.Storage == nil {
		return 0, 0, nil
	}
	var counter int64
	var err error
	var lastModifiedVersion int64

	if group.CanDownload() {
		var num2 int64
		num2, lastModifiedVersion, err = s.syncItems(ctx, group, true)
		if err != nil {
			return 0, 0, err
		}
		counter += num2
		var h int64
		var num3 int64
		num3, h, err = s.syncItems(ctx, group, false)
		if err != nil {
			return 0, 0, err
		}
		counter += num3
		if h > lastModifiedVersion {
			lastModifiedVersion = h
		}
	}

	if counter > 0 {
		if err := s.Storage.RefreshItemTypeHier(ctx); err != nil {
			return counter, 0, err
		}
	}

	return counter, lastModifiedVersion, nil
}

func (s *Syncer) syncItems(ctx context.Context, group *model.Group, trashed bool) (int64, int64, error) {
	if s.Logger != nil {
		s.Logger.Info().Msgf("Syncing items of Group #%v", group.Id)
	}
	var counter int64

	objectList, lastModifiedVersion, err := s.Client.GetItemVersions(ctx, group.Id, group.ItemVersion, trashed)
	if err != nil {
		return counter, 0, errors.Wrapf(err, "cannot get item versions")
	}
	itemsUpdate := []string{}
	for itemid, version := range objectList {
		oldversion, sync, err := s.Storage.GetItemVersion(ctx, group.Id, itemid, "")
		if err != nil {
			return counter, 0, errors.Wrapf(err, "cannot get version of item %v from database: %v", itemid, err)
		}
		if sync != model.SyncStatus_Synced && sync != model.SyncStatus_Incomplete {
			if s.Logger != nil {
				s.Logger.Error().Msgf("item %v not synced. please handle conflict", itemid)
			}
			continue
		}
		if oldversion < version {
			itemsUpdate = append(itemsUpdate, itemid)
		}
	}
	numItems := len(itemsUpdate)
	for part := range slices.Chunk(itemsUpdate, 50) {
		var items []model.Item
		if trashed {
			items, err = s.Client.GetItemsTrashByKey(ctx, group.Id, part)
		} else {
			items, err = s.Client.GetItemsByKey(ctx, group.Id, part)
		}
		if err != nil {
			return counter, 0, errors.Wrapf(err, "cannot get items")
		}
		if s.Logger != nil {
			s.Logger.Info().Msgf("%v items", len(items))
		}
		for _, item := range items {
			if s.Logger != nil {
				s.Logger.Info().Msgf("Item %v of %v", counter, numItems)
			}
			item.Status = model.SyncStatus_Synced
			item.Trashed = trashed

			// Download attachment file if it's an imported_file attachment
			if item.Data.ItemType == "attachment" && item.Data.LinkMode == "imported_file" && s.Fs != nil {
				bucket, err := s.GetGroupBucket(group.Id)
				if err == nil {
					body, contentType, md5str, dlErr := s.Client.DownloadAttachment(ctx, group.Id, item.Key)
					if dlErr == nil {
						_ = s.Fs.FilePut(bucket, item.Key, body, filesystem.FilePutOptions{ContentType: contentType})
						item.MD5 = md5str
					}
				}
			}

			if err := s.Storage.UpdateItem(ctx, group.Id, &item); err != nil {
				if s.Logger != nil {
					s.Logger.Error().Msgf("cannot update item: %v", err)
				}
			}
			counter++
		}
	}
	if s.Logger != nil {
		s.Logger.Info().Msgf("Syncing items of Group #%v done. %v items changed", group.Id, counter)
	}
	return counter, lastModifiedVersion, nil
}

// SyncTags downloads tags changed since the group's tag cursor when tag sync is
// enabled and the group permits downloads.
func (s *Syncer) SyncTags(ctx context.Context, group *model.Group) (int64, int64, error) {
	if s.Client == nil || s.Storage == nil || !group.CanDownload() || !group.SyncTags {
		return 0, 0, nil
	}
	if s.Logger != nil {
		s.Logger.Info().Msgf("Syncing tags of Group #%v", group.Id)
	}
	var counter int64
	tagList, lastModifiedVersion, err := s.Client.GetTags(ctx, group.Id, group.Version)
	if err != nil {
		return counter, 0, errors.Wrapf(err, "cannot get tag versions")
	}
	for _, tag := range tagList {
		if err := s.Storage.CreateTag(ctx, group.Id, tag); err != nil {
			return 0, 0, errors.Wrapf(err, "cannot create tag %v", tag.Tag)
		}
		counter++
	}

	if s.Logger != nil {
		s.Logger.Info().Msgf("Syncing tags of Group #%v done. %v tags changed", group.Id, counter)
	}
	return counter, lastModifiedVersion, nil
}

// SyncDeleted applies Zotero's deleted-object feed to local storage.
func (s *Syncer) SyncDeleted(ctx context.Context, group *model.Group) (int64, error) {
	if s.Client == nil || s.Storage == nil {
		return 0, nil
	}
	deleted, err := s.Client.GetDeleted(ctx, group.Id, group.Version)
	if err != nil {
		return 0, errors.Wrapf(err, "cannot get deleted objects")
	}

	var numDeleted int64
	num, err := s.Storage.DeleteItems(ctx, group.Id, deleted.Items)
	if err != nil {
		return 0, errors.Wrapf(err, "cannot delete items %v", deleted.Items)
	}
	numDeleted += num

	num, err = s.Storage.DeleteCollections(ctx, group.Id, deleted.Collections)
	if err != nil {
		return 0, errors.Wrapf(err, "cannot delete collections %v", deleted.Collections)
	}
	numDeleted += num

	num, err = s.Storage.DeleteTags(ctx, group.Id, deleted.Tags)
	if err != nil {
		return 0, errors.Wrapf(err, "cannot delete tags %v", deleted.Tags)
	}
	numDeleted += num

	return numDeleted, nil
}

// BackupLocal writes the group's records and imported attachments to backupFs
// and advances the corresponding backup timestamps.
func (s *Syncer) BackupLocal(ctx context.Context, group *model.Group, backupFs filesystem.FileSystem) error {
	if s.Storage == nil {
		return errors.New("no storage configured for backup")
	}
	now := time.Now()
	groupId := group.Id

	if err := s.Storage.IterateCollectionsAll(ctx, groupId, group.Gitlab, func(coll *model.Collection) error {
		if s.Logger != nil {
			s.Logger.Info().Msgf("collection #%v.%v - %v", groupId, coll.Key, coll.Data.Name)
		}
		folder := filepath.Clean(fmt.Sprintf("%v/collections", groupId))
		fname := filepath.Clean(fmt.Sprintf("%v.json", coll.Key))
		data := struct {
			LibraryId int64                `json:"libraryid"`
			Id        string               `json:"id"`
			Data      model.CollectionData `json:"data"`
			Meta      model.CollectionMeta `json:"meta"`
		}{
			LibraryId: groupId,
			Id:        coll.Key,
			Data:      coll.Data,
			Meta:      coll.Meta,
		}
		b, err := json.Marshal(data, jsontext.WithIndent("  "))
		if err != nil {
			return errors.Wrapf(err, "cannot marshal collection backup data %v", data)
		}
		if err := backupFs.FilePut(folder, fname, b, filesystem.FilePutOptions{}); err != nil {
			return errors.Wrap(err, "cannot write collection backup file")
		}
		return nil
	}); err != nil {
		return errors.Wrap(err, "cannot iterate collections for backup")
	}

	if err := s.Storage.UpdateCollectionsGitlabTimestamp(ctx, groupId, now, group.Gitlab); err != nil {
		return errors.Wrap(err, "cannot update collections gitlab timestamp")
	}

	if err := s.Storage.IterateItemsAll(ctx, groupId, group.Gitlab, func(item *model.Item) error {
		if s.Logger != nil {
			s.Logger.Info().Msgf("item #%v.%v - %v", groupId, item.Key, item.Data.Title)
		}
		folder := filepath.Clean(fmt.Sprintf("%v/items", groupId))
		fname := filepath.Clean(fmt.Sprintf("%v.json", item.Key))
		data := struct {
			LibraryId int64             `json:"libraryid"`
			Id        string            `json:"id"`
			Data      model.ItemGeneric `json:"data"`
			Meta      model.ItemMeta    `json:"meta"`
		}{
			LibraryId: groupId,
			Id:        item.Key,
			Data:      item.Data,
			Meta:      item.Meta,
		}
		b, err := json.Marshal(data, jsontext.WithIndent("  "))
		if err != nil {
			return errors.Wrapf(err, "cannot marshal item backup data %v", data)
		}
		if err := backupFs.FilePut(folder, fname, b, filesystem.FilePutOptions{}); err != nil {
			return errors.Wrap(err, "cannot write item backup file")
		}

		if strings.ToLower(item.Data.ItemType) == "attachment" && s.Fs != nil {
			bucket, err := s.GetGroupBucket(groupId)
			if err == nil {
				file, err := s.Fs.FileOpenRead(bucket, item.Key, filesystem.FileGetOptions{})
				if err == nil {
					defer file.Close()
					_ = backupFs.FileWrite(folder, fmt.Sprintf("%v.bin", item.Key), file, -1, filesystem.FilePutOptions{})
				}
			}
		}
		return nil
	}); err != nil {
		return errors.Wrap(err, "cannot iterate items for backup")
	}

	if err := s.Storage.UpdateItemsGitlabTimestamp(ctx, groupId, now, group.Gitlab); err != nil {
		return errors.Wrap(err, "cannot update items gitlab timestamp")
	}

	// Backup Group itself
	groupFolder := fmt.Sprintf("%v", groupId)
	fname := "group.json"
	groupData := struct {
		Id                int64           `json:"id"`
		Data              model.GroupData `json:"data"`
		CollectionVersion int64           `json:"collectionversion"`
		ItemVersion       int64           `json:"itemversion"`
		TagVersion        int64           `json:"tagversion"`
	}{
		Id:                groupId,
		Data:              group.Data,
		CollectionVersion: group.CollectionVersion,
		ItemVersion:       group.ItemVersion,
		TagVersion:        group.TagVersion,
	}
	b, err := json.Marshal(groupData, jsontext.WithIndent("  "))
	if err != nil {
		return errors.Wrapf(err, "cannot marshal group backup data %v", groupData)
	}
	if err := backupFs.FilePut(groupFolder, fname, b, filesystem.FilePutOptions{}); err != nil {
		return errors.Wrap(err, "cannot write group backup file")
	}

	if err := s.Storage.UpdateGroupGitlabTimestamp(ctx, groupId, now); err != nil {
		return errors.Wrap(err, "cannot update group gitlab timestamp")
	}

	return nil
}
