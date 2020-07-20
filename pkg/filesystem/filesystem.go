package filesystem

import (
	"fmt"
	"io"
	"os"
)

type NotFoundError struct {
	err error
}

func (nf *NotFoundError) Error() string {
	return fmt.Sprintf("file not found: %v", nf.err)
}

func IsNotFoundError(err error) bool {
	_, ok := err.(*NotFoundError)
	return ok
}

// PutObjectOptions represents options specified by user for PutObject call
type FilePutOptions struct {
	Progress    io.Reader
	ContentType string
}

// GetObjectOptions represents options specified by user for GetObject call
type FileGetOptions struct {
	VersionID string
}

type FileStatOptions struct {
}

type FolderCreateOptions struct {
	ObjectLocking bool
}

type FileSystem interface {
	FolderExists(folder string) (bool, error)
	FolderCreate(folder string, opts FolderCreateOptions) error
	FileExists(folder, name string) (bool, error)
	FileGet(folder, name string, opts FileGetOptions) ([]byte, error)
	FilePut(folder, name string, data []byte, opts FilePutOptions) error
	FileStat(folder, name string, opts FileStatOptions) (os.FileInfo, error)
}
