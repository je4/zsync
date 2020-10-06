package filesystem

import (
	"fmt"
	"github.com/goph/emperror"
	"github.com/op/go-logging"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
)

type LocalFs struct {
	basepath string
	logger   *logging.Logger
}

func FileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func FolderExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return info.IsDir()
}

func NewLocalFs(basepath string, logger *logging.Logger) (*LocalFs, error) {
	if !FolderExists(basepath) {
		return nil, fmt.Errorf("path %v does not exists", basepath)
	}
	return &LocalFs{basepath: basepath, logger: logger}, nil
}

func (fs *LocalFs) Protocol() string {
	return "file://"
}

func (fs *LocalFs) String() string {
	return fs.basepath
}

func (fs *LocalFs) FileStat(folder, name string, opts FileStatOptions) (os.FileInfo, error) {
	path := filepath.Join(folder, name)
	return os.Stat(filepath.Join(fs.basepath, path))
}

func (fs *LocalFs) FileExists(folder, name string) (bool, error) {
	path := filepath.Join(folder, name)
	return FileExists(filepath.Join(fs.basepath, path)), nil
}

func (fs *LocalFs) FolderExists(folder string) (bool, error) {
	return FolderExists(filepath.Join(fs.basepath, folder)), nil
}

func (fs *LocalFs) FolderCreate(folder string, opts FolderCreateOptions) error {
	path := filepath.Join(fs.basepath, folder)
	if FolderExists(path) {
		return nil
	}
	fs.logger.Debugf("create folder %v", path)
	err := os.MkdirAll(path, 0755)
	if err != nil {
		return emperror.Wrapf(err, "cannot create folder %v", path)
	}
	return nil
}

func (fs *LocalFs) FileGet(folder, name string, opts FileGetOptions) ([]byte, error) {
	path := filepath.Join(folder, name)
	data, err := ioutil.ReadFile(filepath.Join(fs.basepath, path))
	if err != nil {
		return nil, emperror.Wrapf(err, "cannot read file %v", path)
	}
	return data, nil
}

func (fs *LocalFs) FilePut(folder, name string, data []byte, opts FilePutOptions) error {
	if err := fs.FolderCreate(folder, FolderCreateOptions{}); err != nil {
		return emperror.Wrapf(err, "cannot create folder %v", folder)
	}
	path := filepath.Join(fs.basepath, filepath.Join(folder, name))
	fs.logger.Debugf("writing data to: %v", path)
	if err := ioutil.WriteFile(path, data, 0644); err != nil {
		return emperror.Wrapf(err, "cannot write data to %v", path)
	}
	return nil
}

func (fs *LocalFs) FileWrite(folder, name string, r io.Reader, size int64, opts FilePutOptions) error {
	if err := fs.FolderCreate(folder, FolderCreateOptions{}); err != nil {
		return emperror.Wrapf(err, "cannot create folder %v", folder)
	}
	path := filepath.Join(folder, name)
	file, err := os.OpenFile(filepath.Join(fs.basepath, path), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return emperror.Wrapf(err, "cannot open file %v", path)
	}
	defer file.Close()
	if size == -1 {
		if _, err := io.Copy(file, r); err != nil {
			return emperror.Wrapf(err, "cannot write to file %v", path)
		}
	} else {
		if _, err := io.CopyN(file, r, size); err != nil {
			if err != io.EOF && err != io.ErrUnexpectedEOF {
				return emperror.Wrapf(err, "cannot write to file %v", path)
			}
		}
	}
	return nil
}

func (fs *LocalFs) FileRead(folder, name string, w io.Writer, size int64, opts FileGetOptions) error {
	path := filepath.Join(folder, name)
	file, err := os.OpenFile(filepath.Join(fs.basepath, path), os.O_RDONLY, 0644)
	if err != nil {
		return emperror.Wrapf(err, "cannot open file %v", path)
	}
	defer file.Close()
	if size == -1 {
		if _, err := io.Copy(w, file); err != nil {
			return emperror.Wrapf(err, "cannot read from %v/%v", path, name)
		}
	} else {
		if _, err := io.CopyN(w, file, size); err != nil {
			return emperror.Wrapf(err, "cannot read from %v/%v", path, name)
		}
	}
	return nil
}

func (fs *LocalFs) FileOpenRead(folder, name string, opts FileGetOptions) (io.ReadCloser, error) {
	path := filepath.Join(folder, name)
	file, err := os.OpenFile(filepath.Join(fs.basepath, path), os.O_RDONLY, 0644)
	if err != nil {
		return nil, emperror.Wrapf(err, "cannot open file %v", path)
	}
	return file, nil
}
