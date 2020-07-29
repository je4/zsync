package filesystem

import (
	"fmt"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/goph/emperror"
	"io"
	"os"
	"path/filepath"
	"time"
)

type GitFs struct {
	localFs *LocalFs
	repo    *git.Repository
}

func NewGitFs(basepath string) (*GitFs, error) {
	if !FolderExists(basepath) {
		return nil, fmt.Errorf("path %v does not exists", basepath)
	}
	localfs := &LocalFs{basepath: basepath}
	gitFs := &GitFs{
		localFs: localfs,
	}
	if err := gitFs.Open(); err != nil {
		return nil, emperror.Wrap(err, "cannot open gitfs")
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
		return emperror.Wrapf(err, "cannot open git repository at %v", fs.localFs.basepath)
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
		return emperror.Wrapf(err, "cannot stat file %v/%v", folder, name)
	}
	if err := fs.localFs.FilePut(folder, name, data, opts); err != nil {
		return err
	}
	if !isUpdate {
		w, err := fs.repo.Worktree()
		if err != nil {
			return emperror.Wrapf(err, "cannot open worktree of %v", fs.localFs.basepath)
		}
		if _, err := w.Add(filepath.Join(folder, name)); err != nil {
			return emperror.Wrapf(err, "cannot add %v/%v/%v to repository", fs.localFs.basepath, folder, name)
		}
	}
	return nil
}

func (fs *GitFs) FileWrite(folder, name string, r io.Reader, size int64, opts FilePutOptions) error {
	isUpdate, err := fs.FileExists(folder, name)
	if err != nil {
		return emperror.Wrapf(err, "cannot stat file %v/%v", folder, name)
	}
	if err := fs.localFs.FileWrite(folder, name, r, size, opts); err != nil {
		return err
	}
	if !isUpdate {
		w, err := fs.repo.Worktree()
		if err != nil {
			return emperror.Wrapf(err, "cannot open worktree of %v", fs.localFs.basepath)
		}
		if _, err := w.Add(filepath.Join(folder, name)); err != nil {
			return emperror.Wrapf(err, "cannot add %v/%v/%v to repository", fs.localFs.basepath, folder, name)
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
		return emperror.Wrap(err, "cannot get worktree")
	}
	commit, err := w.Commit(msg, &git.CommitOptions{
		Author:    &object.Signature{
			Name:  name,
			Email: email,
			When:  time.Now()},
	});
	if err != nil {
		return emperror.Wrap(err, "cannot commit")
	}
	obj, err := fs.repo.CommitObject(commit)
	if err != nil {
		return emperror.Wrap(err, "cannot commit")
	}
	fmt.Println(obj)
	return nil
}
