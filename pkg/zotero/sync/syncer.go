package sync

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"emperror.dev/errors"
	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/je4/zsync/v2/pkg/filesystem"
	"github.com/je4/zsync/v2/pkg/zotero/client"
	"github.com/je4/zsync/v2/pkg/zotero/model"
	"github.com/je4/zsync/v2/pkg/zotero/storage"
)

type Syncer struct {
	Client  *client.Client
	Storage *storage.Storage
	Fs      filesystem.FileSystem
	Logger  zLogger.ZLogger
}

func NewSyncer(client *client.Client, storage *storage.Storage, fs filesystem.FileSystem, logger zLogger.ZLogger) *Syncer {
	return &Syncer{
		Client:  client,
		Storage: storage,
		Fs:      fs,
		Logger:  logger,
	}
}

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

func (s *Syncer) SyncGroup(group *model.Group) error {
	if s.Logger != nil {
		s.Logger.Info().Msgf("Syncing Group #%v - %v", group.Id, group.Data.Name)
	}

	_, collectionVersion, err := s.SyncCollections(group)
	if err != nil {
		return errors.Wrapf(err, "cannot sync collections of Group %v", group.Id)
	}

	_, itemVersion, err := s.UploadItems(group)
	if err != nil {
		return errors.Wrapf(err, "cannot sync items of Group %v", group.Id)
	}

	_, itemVersion, err = s.DownloadItems(group)
	if err != nil {
		return errors.Wrapf(err, "cannot sync items of Group %v", group.Id)
	}

	_, tagVersion, err := s.SyncTags(group)
	if err != nil {
		return errors.Wrapf(err, "cannot sync tags of Group %v", group.Id)
	}

	_, err = s.SyncDeleted(group)
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
		if err := s.Storage.UpdateGroup(group); err != nil {
			return errors.Wrapf(err, "cannot update group in storage")
		}
	}

	return nil
}

func (s *Syncer) SyncCollections(group *model.Group) (int64, int64, error) {
	if s.Client == nil || s.Storage == nil {
		return 0, 0, nil
	}
	var num, num2 int64
	var err error
	var lastModifiedVersion int64

	// upload data
	if group.CanUpload() {
		num, err = s.syncModifiedCollections(group)
		if err != nil {
			return 0, 0, err
		}
	}

	if group.CanDownload() {
		num2, lastModifiedVersion, err = s.syncCollections(group)
		if err != nil {
			return 0, 0, err
		}
	}

	counter := num + num2
	if counter > 0 {
		if err := s.Storage.RefreshCollectionNameHier(); err != nil {
			return counter, 0, err
		}
	}

	return counter, lastModifiedVersion, nil
}

func (s *Syncer) syncModifiedCollections(group *model.Group) (int64, error) {
	var counter int64
	colls, err := s.Storage.GetModifiedCollections(group.Id)
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
			if err := s.Client.DeleteCollection(group.Id, coll.Key, lastModifiedVersion); err != nil {
				return 0, errors.Wrapf(err, "error deleting collection %v.%v", group.Id, coll.Key)
			}
			coll.Status = model.SyncStatus_Synced
			if err := s.Storage.UpdateCollection(group.Id, coll); err != nil {
				return 0, errors.Wrapf(err, "cannot update deleted collection in storage")
			}
		} else {
			successKey, err := s.Client.UpdateCollection(group.Id, &coll.Data, &lastModifiedVersion)
			if err != nil {
				return 0, errors.Wrapf(err, "error creating/updating collection %v.%v", group.Id, coll.Key)
			}
			coll.Key = successKey
			coll.Data.Key = successKey
			coll.Version = lastModifiedVersion
			coll.Data.Version = lastModifiedVersion
			coll.Status = model.SyncStatus_Synced
			if err := s.Storage.UpdateCollection(group.Id, coll); err != nil {
				return 0, errors.Wrapf(err, "cannot update collection in storage")
			}
		}
	}
	return counter, nil
}

func (s *Syncer) syncCollections(group *model.Group) (int64, int64, error) {
	if s.Logger != nil {
		s.Logger.Info().Msgf("Syncing collections of Group #%v", group.Id)
	}

	var counter int64
	objectList, lastModifiedVersion, err := s.Client.GetCollectionVersions(group.Id, group.CollectionVersion)
	if err != nil {
		return counter, 0, errors.Wrapf(err, "cannot get collection versions")
	}
	collectionUpdate := []string{}
	for collectionid, version := range *objectList {
		oldversion, status, err := s.Storage.GetCollectionVersion(group.Id, collectionid)
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
	numCollections := len(collectionUpdate)
	for i := 0; i < (numCollections/50)+1; i++ {
		start := i * 50
		end := min(numCollections, start+50)
		part := collectionUpdate[start:end]
		if len(part) == 0 {
			continue
		}
		collections, _, err := s.Client.GetCollectionsByKey(group.Id, part)
		if err != nil {
			return counter, 0, errors.Wrapf(err, "cannot get collections")
		}
		if s.Logger != nil {
			s.Logger.Info().Msgf("%v collections", len(*collections))
		}
		for _, coll := range *collections {
			coll.Status = model.SyncStatus_Synced
			if err := s.Storage.UpdateCollection(group.Id, &coll); err != nil {
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

func (s *Syncer) UploadItems(group *model.Group) (int64, int64, error) {
	if s.Client == nil || s.Storage == nil {
		return 0, 0, nil
	}
	var counter int64
	var lastModifiedVersion int64

	if group.CanUpload() {
		verMap, lmv, err := s.Client.GetItemsVersion(group.Id, 0, false)
		if err != nil {
			return 0, 0, errors.Wrapf(err, "cannot get last modified item version")
		}
		_ = verMap
		lastModifiedVersion = lmv
		num4, err := s.syncModifiedItems(group, lastModifiedVersion)
		if err != nil {
			return 0, 0, err
		}
		counter += num4
	}

	if counter > 0 {
		if err := s.Storage.RefreshItemTypeHier(); err != nil {
			return counter, 0, err
		}
	}

	return counter, lastModifiedVersion, nil
}

func (s *Syncer) syncModifiedItems(group *model.Group, lastModifiedVersion int64) (int64, error) {
	var counter int64
	items, err := s.Storage.GetModifiedItems(group.Id)
	if err != nil {
		return 0, err
	}

	for _, item := range items {
		counter++
		if s.Logger != nil {
			s.Logger.Info().Msgf("writing item %v of %v to zotero cloud", counter, len(items))
		}

		if item.Deleted {
			if err := s.Client.DeleteItem(group.Id, item.Key, lastModifiedVersion); err != nil {
				return 0, errors.Wrapf(err, "delete item %v failed", item.Key)
			}
			item.Status = model.SyncStatus_Synced
			if err := s.Storage.UpdateItem(group.Id, item); err != nil {
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
					uploadedMd5, upErr := s.Client.UploadAttachment(group.Id, item.Key, data, item.Data.Filename, item.Data.ContentType, mtime, item.MD5)
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

		successKey, err := s.Client.UpdateItem(group.Id, &item.Data, &lastModifiedVersion)
		if err != nil {
			if s.Logger != nil {
				s.Logger.Warn().Msgf("error creating/updating item %v.%v - retrying with new version", group.Id, item.Key)
			}
			successKey, err = s.Client.UpdateItem(group.Id, &item.Data, &lastModifiedVersion)
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
		if err := s.Storage.UpdateItem(group.Id, item); err != nil {
			return 0, errors.Wrapf(err, "cannot update item in storage")
		}
	}
	return counter, nil
}

func (s *Syncer) DownloadItems(group *model.Group) (int64, int64, error) {
	if s.Client == nil || s.Storage == nil {
		return 0, 0, nil
	}
	var counter int64
	var err error
	var lastModifiedVersion int64

	if group.CanDownload() {
		var num2 int64
		num2, lastModifiedVersion, err = s.syncItems(group, true)
		if err != nil {
			return 0, 0, err
		}
		counter += num2
		var h int64
		var num3 int64
		num3, h, err = s.syncItems(group, false)
		if err != nil {
			return 0, 0, err
		}
		counter += num3
		if h > lastModifiedVersion {
			lastModifiedVersion = h
		}
	}

	if counter > 0 {
		if err := s.Storage.RefreshItemTypeHier(); err != nil {
			return counter, 0, err
		}
	}

	return counter, lastModifiedVersion, nil
}

func (s *Syncer) syncItems(group *model.Group, trashed bool) (int64, int64, error) {
	if s.Logger != nil {
		s.Logger.Info().Msgf("Syncing items of Group #%v", group.Id)
	}
	var counter int64

	objectList, lastModifiedVersion, err := s.Client.GetItemsVersion(group.Id, group.ItemVersion, trashed)
	if err != nil {
		return counter, 0, errors.Wrapf(err, "cannot get item versions")
	}
	itemsUpdate := []string{}
	for itemid, version := range *objectList {
		oldversion, sync, err := s.Storage.GetItemVersion(group.Id, itemid, "")
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
	for i := 0; i < (numItems/50)+1; i++ {
		start := i * 50
		end := min(numItems, start+50)
		part := itemsUpdate[start:end]
		if len(part) == 0 {
			continue
		}
		var items *[]model.Item
		if trashed {
			items, err = s.Client.GetItemsTrashByKey(group.Id, part)
		} else {
			items, err = s.Client.GetItemsByKey(group.Id, part)
		}
		if err != nil {
			return counter, 0, errors.Wrapf(err, "cannot get items")
		}
		if s.Logger != nil {
			s.Logger.Info().Msgf("%v items", len(*items))
		}
		for _, item := range *items {
			if s.Logger != nil {
				s.Logger.Info().Msgf("Item %v of %v", counter, numItems)
			}
			item.Status = model.SyncStatus_Synced
			item.Trashed = trashed

			// Download attachment file if it's an imported_file attachment
			if item.Data.ItemType == "attachment" && item.Data.LinkMode == "imported_file" && s.Fs != nil {
				bucket, err := s.GetGroupBucket(group.Id)
				if err == nil {
					body, contentType, md5str, dlErr := s.Client.DownloadAttachment(group.Id, item.Key)
					if dlErr == nil {
						_ = s.Fs.FilePut(bucket, item.Key, body, filesystem.FilePutOptions{ContentType: contentType})
						item.MD5 = md5str
					}
				}
			}

			if err := s.Storage.UpdateItem(group.Id, &item); err != nil {
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

func (s *Syncer) SyncTags(group *model.Group) (int64, int64, error) {
	if s.Client == nil || s.Storage == nil || !group.CanDownload() || !group.SyncTags {
		return 0, 0, nil
	}
	if s.Logger != nil {
		s.Logger.Info().Msgf("Syncing tags of Group #%v", group.Id)
	}
	var counter int64
	tagList, lastModifiedVersion, err := s.Client.GetTagsVersion(group.Id, group.Version)
	if err != nil {
		return counter, 0, errors.Wrapf(err, "cannot get tag versions")
	}
	for _, tag := range *tagList {
		if err := s.Storage.CreateTag(group.Id, tag); err != nil {
			return 0, 0, errors.Wrapf(err, "cannot create tag %v", tag.Tag)
		}
		counter++
	}

	if s.Logger != nil {
		s.Logger.Info().Msgf("Syncing tags of Group #%v done. %v tags changed", group.Id, counter)
	}
	return counter, lastModifiedVersion, nil
}

func (s *Syncer) SyncDeleted(group *model.Group) (int64, error) {
	if s.Client == nil || s.Storage == nil {
		return 0, nil
	}
	delCollections, delItems, delTags, err := s.Client.GetDeleted(group.Id, group.Version)
	if err != nil {
		return 0, errors.Wrapf(err, "cannot get deleted objects")
	}

	numDeleted := 0
	if delItems != nil {
		numDeleted += len(*delItems)
		for _, itemKey := range *delItems {
			if err := s.Storage.DeleteItem(group.Id, itemKey); err != nil {
				return 0, errors.Wrapf(err, "cannot delete item %v", itemKey)
			}
		}
	}
	if delCollections != nil {
		numDeleted += len(*delCollections)
		for _, collectionKey := range *delCollections {
			if err := s.Storage.DeleteCollection(group.Id, collectionKey); err != nil {
				return 0, errors.Wrapf(err, "cannot delete collection %v", collectionKey)
			}
		}
	}
	if delTags != nil {
		numDeleted += len(*delTags)
		for _, tagName := range *delTags {
			if err := s.Storage.DeleteTag(group.Id, tagName); err != nil {
				return 0, errors.Wrapf(err, "cannot delete tag %v", tagName)
			}
		}
	}

	return int64(numDeleted), nil
}

func (s *Syncer) BackupLocal(group *model.Group, backupFs filesystem.FileSystem) error {
	if s.Storage == nil {
		return errors.New("no storage configured for backup")
	}
	now := time.Now()
	groupId := group.Id

	if err := s.Storage.IterateCollectionsAll(groupId, group.Gitlab, func(coll *model.Collection) error {
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

	if err := s.Storage.UpdateCollectionsGitlabTimestamp(groupId, now, group.Gitlab); err != nil {
		return errors.Wrap(err, "cannot update collections gitlab timestamp")
	}

	if err := s.Storage.IterateItemsAll(groupId, group.Gitlab, func(item *model.Item) error {
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

	if err := s.Storage.UpdateItemsGitlabTimestamp(groupId, now, group.Gitlab); err != nil {
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

	if err := s.Storage.UpdateGroupGitlabTimestamp(groupId, now); err != nil {
		return errors.Wrap(err, "cannot update group gitlab timestamp")
	}

	return nil
}
