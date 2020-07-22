package filesystem

import (
	"fmt"
	"github.com/minio/minio-go/v7"
	"os"
	"time"
)

func NewS3FileInfo(bucket, name string, info minio.ObjectInfo) *S3FileInfo {
	return &S3FileInfo{
		bucket: bucket,
		name:   name,
		info:   info,
	}
}

// A FileInfo describes a file and is returned by Stat and Lstat.
type S3FileInfo struct {
	bucket, name string
	info minio.ObjectInfo
}

func (sfi *S3FileInfo) Name() string { // base name of the file
	return fmt.Sprintf("%v/%v", sfi.bucket, sfi.name)
}

func (sfi *S3FileInfo) Size() int64 { // length in bytes for regular files; system-dependent for others
	return sfi.info.Size
}

func (sfi *S3FileInfo) Mode() os.FileMode { // file mode bits
	return 0x700 // todo: return something better
}

func (sfi *S3FileInfo) ModTime() time.Time { // modification time
	return sfi.info.LastModified
}

func (sfi *S3FileInfo) IsDir() bool { // abbreviation for Mode().IsDir()
	return false
}

func (sfi *S3FileInfo) Sys() interface{} {   // underlying data source (can return nil)
	return nil
}



