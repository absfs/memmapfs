package memmapfs_test

import (
	"testing"

	"github.com/absfs/absfs"
	"github.com/absfs/fstesting"
	"github.com/absfs/memmapfs"
	"github.com/absfs/osfs"
)

func TestMemMapFSWrapper(t *testing.T) {
	baseFS, err := osfs.NewFS()
	if err != nil {
		t.Fatalf("failed to create base filesystem: %v", err)
	}

	suite := &fstesting.WrapperSuite{
		Name: "memmapfs",
		Factory: func(base absfs.FileSystem) (absfs.FileSystem, error) {
			config := memmapfs.DefaultConfig()
			return memmapfs.New(base, config), nil
		},
		BaseFS:         baseFS,
		TransformsData: false,
		TransformsMeta: false,
		ReadOnly:       false,
	}

	suite.Run(t)
}

func TestMemMapFSReadOnly(t *testing.T) {
	baseFS, err := osfs.NewFS()
	if err != nil {
		t.Fatalf("failed to create base filesystem: %v", err)
	}

	suite := &fstesting.WrapperSuite{
		Name: "memmapfs-readonly",
		Factory: func(base absfs.FileSystem) (absfs.FileSystem, error) {
			config := &memmapfs.Config{
				Mode:        memmapfs.ModeReadOnly,
				SyncMode:    memmapfs.SyncNever,
				MapFullFile: true,
			}
			return memmapfs.New(base, config), nil
		},
		BaseFS:         baseFS,
		TransformsData: false,
		TransformsMeta: false,
		ReadOnly:       false,
	}

	suite.Run(t)
}

func TestMemMapFSCopyOnWrite(t *testing.T) {
	baseFS, err := osfs.NewFS()
	if err != nil {
		t.Fatalf("failed to create base filesystem: %v", err)
	}

	suite := &fstesting.WrapperSuite{
		Name: "memmapfs-cow",
		Factory: func(base absfs.FileSystem) (absfs.FileSystem, error) {
			config := &memmapfs.Config{
				Mode:        memmapfs.ModeCopyOnWrite,
				SyncMode:    memmapfs.SyncLazy,
				MapFullFile: true,
			}
			return memmapfs.New(base, config), nil
		},
		BaseFS:         baseFS,
		TransformsData: false,
		TransformsMeta: false,
		ReadOnly:       false,
	}

	suite.Run(t)
}
