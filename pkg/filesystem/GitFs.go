package filesystem

import (
	"emperror.dev/errors"
	"fmt"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/op/go-logging"
	"io"
	"os"
	"path/filepath"
	"time"
)

type GitFs struct {
	localFs *LocalFs
	repo    *git.Repository
	logger  *logging.Logger
}

func NewGitFs(basepath string, logger *logging.Logger) (*GitFs, error) {
	if !FolderExists(basepath) {
		return nil, fmt.Errorf("path %v does not exists", basepath)
	}
	localfs, err := NewLocalFs(basepath, logger)
	if err != nil {
		return nil, errors.Wrap(err, "cannot create local fs")
	}
	gitFs := &GitFs{
		localFs: localfs,
		logger:  logger,
	}
	if err := gitFs.Open(); err != nil {
		return nil, errors.Wrap(err, "cannot open gitfs")
	}

	return gitFs, nil
}

func (fs *GitFs) String() string {
	return fs.localFs.basepath
}

func (fs *GitFs) Open() error {
	var err error
	fs.repo, err = git.PlainOpen(fs.localFs.basepath)
	if err != nil {
		return errors.Wrapf(err, "cannot open git repository at %v", fs.localFs.basepath)
	}
	return nil
}

func (fs *GitFs) FileStat(folder, name string, opts FileStatOptions) (os.FileInfo, error) {
	return fs.localFs.FileStat(folder, name, opts)
}

func (fs *GitFs) FileExists(folder, name string) (bool, error) {
	return fs.localFs.FileExists(folder, name)
}

func (fs *GitFs) FolderExists(folder string) (bool, error) {
	return fs.localFs.FolderExists(folder)
}

func (fs *GitFs) FolderCreate(folder string, opts FolderCreateOptions) error {
	return fs.localFs.FolderCreate(folder, opts)
}

func (fs *GitFs) FileGet(folder, name string, opts FileGetOptions) ([]byte, error) {
	return fs.localFs.FileGet(folder, name, opts)
}

func (fs *GitFs) FilePut(folder, name string, data []byte, opts FilePutOptions) error {
	isUpdate, err := fs.FileExists(folder, name)
	if err != nil {
		return errors.Wrapf(err, "cannot stat file %v/%v", folder, name)
	}
	if err := fs.localFs.FilePut(folder, name, data, opts); err != nil {
		return err
	}
	if !isUpdate {
		w, err := fs.repo.Worktree()
		if err != nil {
			return errors.Wrapf(err, "cannot open worktree of %v", fs.localFs.basepath)
		}
		if _, err := w.Add(filepath.Join(folder, name)); err != nil {
			return errors.Wrapf(err, "cannot add %v/%v/%v to repository", fs.localFs.basepath, folder, name)
		}
	}
	return nil
}

func (fs *GitFs) FileWrite(folder, name string, r io.Reader, size int64, opts FilePutOptions) error {
	isUpdate, err := fs.FileExists(folder, name)
	if err != nil {
		return errors.Wrapf(err, "cannot stat file %v/%v", folder, name)
	}
	if err := fs.localFs.FileWrite(folder, name, r, size, opts); err != nil {
		return err
	}
	if !isUpdate {
		w, err := fs.repo.Worktree()
		if err != nil {
			return errors.Wrapf(err, "cannot open worktree of %v", fs.localFs.basepath)
		}
		fname := filepath.Join(folder, name)
		fs.logger.Debugf("adding %v to git", fname)
		if _, err := w.Add(fname); err != nil {
			return errors.Wrapf(err, "cannot add %v/%v/%v to repository", fs.localFs.basepath, folder, name)
		}
	}

	return nil
}

func (fs *GitFs) FileRead(folder, name string, w io.Writer, size int64, opts FileGetOptions) error {
	return fs.localFs.FileRead(folder, name, w, size, opts)
}

func (fs *GitFs) FileOpenRead(folder, name string, opts FileGetOptions) (io.ReadCloser, error) {
	return fs.localFs.FileOpenRead(folder, name, opts)
}

func (fs *GitFs) Commit(msg, name, email string) error {
	w, err := fs.repo.Worktree()
	if err != nil {
		return errors.Wrap(err, "cannot get worktree")
	}
	commit, err := w.Commit(msg, &git.CommitOptions{
		Author: &object.Signature{
			Name:  name,
			Email: email,
			When:  time.Now()},
	})
	if err != nil {
		return errors.Wrap(err, "cannot commit")
	}
	obj, err := fs.repo.CommitObject(commit)
	if err != nil {
		return errors.Wrap(err, "cannot commit")
	}
	fmt.Println(obj)
	return nil
}
