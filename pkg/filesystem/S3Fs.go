package filesystem

import (
	"bytes"
	"context"
	"github.com/goph/emperror"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"io"
	"net/http"
	"os"
)

type S3Fs struct {
	s3 *minio.Client
}

func NewS3Fs(Endpoint string,
	AccessKeyId string,
	SecretAccessKey string,
	UseSSL bool) (*S3Fs, error) {
	// connect to S3 / Minio
	s3, err := minio.New(Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(AccessKeyId, SecretAccessKey, ""),
		Secure: UseSSL,
	})
	if err != nil {
		return nil, emperror.Wrap(err, "cannot conntct to s3 instance")
	}
	return &S3Fs{s3: s3}, nil
}

func (fs *S3Fs) FileStat( folder, name string, opts FileStatOptions) (os.FileInfo, error) {
	sinfo, err := fs.s3.StatObject(context.Background(), folder, name, minio.StatObjectOptions{})
	if err != nil {
		// no file no error
		s3Err, ok := err.(minio.ErrorResponse)
		if ok {
			if s3Err.StatusCode == http.StatusNotFound {
				return nil, &NotFoundError{err: err}
			}
		}
		return nil, emperror.Wrapf(err, "cannot get file info for %v/%v", folder, name)
	}
	return NewS3FileInfo(folder, name, sinfo), nil
}

func (fs *S3Fs) FileExists(folder, name string) (bool, error) {
	_, err := fs.FileStat(folder, name, FileStatOptions{})
	if err != nil {
		// no file no error
		if IsNotFoundError(err) {
				return false, nil
		}
		return false, emperror.Wrapf(err, "cannot get file info for %v/%v", folder, name)
	}
	return true, nil
}

func (fs *S3Fs) FolderExists(folder string) (bool, error) {
	found, err := fs.s3.BucketExists(context.Background(), folder)
	if err != nil {
		return false, emperror.Wrapf(err, "cannot get check for folder %v", folder)
	}
	return found, nil
}

func (fs *S3Fs) FolderCreate( folder string, opts FolderCreateOptions) error {
	if err := fs.s3.MakeBucket(context.Background(), folder, minio.MakeBucketOptions{ObjectLocking: opts.ObjectLocking}); err != nil {
		return emperror.Wrapf(err, "cannot create bucket %s", folder)
	}
	return nil
}

func (fs *S3Fs) FileGet(folder, name string, opts FileGetOptions) ([]byte, error) {
	object, err := fs.s3.GetObject(context.Background(), folder, name, minio.GetObjectOptions{VersionID: opts.VersionID})
	if err != nil {
		// no file no error
		s3Err, ok := err.(minio.ErrorResponse)
		if ok {
			if s3Err.StatusCode == http.StatusNotFound {
				return nil, &NotFoundError{err: s3Err}
			}
		}
		return nil, emperror.Wrapf(err, "cannot get file info for %v/%v", folder, name)
	}

	var b = &bytes.Buffer{}
	if _, err := io.Copy(b, object); err != nil {
		return nil, emperror.Wrapf(err, "cannot copy data from %v/%v", folder, name)
	}
	return b.Bytes(), nil
}

func (fs *S3Fs) FilePut(folder, name string, data []byte, opts FilePutOptions) error {
	if _, err := fs.s3.PutObject(
		context.Background(),
		folder,
		name,
		bytes.NewReader(data),
		int64(len(data)),
		minio.PutObjectOptions{ContentType: opts.ContentType, Progress: opts.Progress},
	); err != nil {
		return emperror.Wrapf(err, "cannot put %v/%v", folder, name)
	}
	return nil
}

func (fs *S3Fs) FileWrite(folder, name string, r io.Reader, size int64, opts FilePutOptions) error {
	if _, err := fs.s3.PutObject(
		context.Background(),
		folder,
		name,
		r,
		size,
		minio.PutObjectOptions{ContentType: opts.ContentType, Progress: opts.Progress},
	); err != nil {
		return emperror.Wrapf(err, "cannot put %v/%v", folder, name)
	}
	return nil
}
