package main

import (
	"io/ioutil"
	"os"
)

type fsMounter struct{}

type osDirectoryMaker struct{}

// IsDir will return whether the given path is a directory or not
func (o osDirectoryMaker) IsDir(dir string) bool {
	stat, err := os.Stat(dir)
	if err != nil {
		return false
	}

	return stat.IsDir()
}

// MakeDir will create a directory with the given mode
func (o osDirectoryMaker) MakeDir(dir string, mode os.FileMode) error {
	return os.MkdirAll(dir, mode)
}

// MakeTempDir will create a temporary directory
func (o osDirectoryMaker) MakeTempDir() (string, error) {
	return ioutil.TempDir(os.TempDir(), "docker-plugin-cephfs_mnt_")
}
