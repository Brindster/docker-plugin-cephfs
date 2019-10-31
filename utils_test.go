package main

import (
	"io/ioutil"
	"os"
	"path"
	"strings"
	"testing"
)

func TestOsDirectoryMaker_IsDir(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "docker-plugin-cephfs")
	if err != nil {
		t.Fatalf("Could not initialize test tmpdir: %s", err)
	}
	defer must(func() error { return os.RemoveAll(tmpdir) })

	mker := osDirectoryMaker{}
	got := mker.IsDir(tmpdir)
	if got != true {
		t.Errorf("IsDir() got = %v, want %v", got, true)
	}

	got = mker.IsDir("/not-a-valid-dir")
	if got != false {
		t.Errorf("IsDir() got = %v, want %v", got, false)
	}
}

func TestOsDirectoryMaker_MakeDir(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "docker-plugin-cephfs")
	if err != nil {
		t.Fatalf("Could not initialize test tmpdir: %s", err)
	}
	defer must(func() error { return os.RemoveAll(tmpdir) })

	desired := path.Join(tmpdir, "TestOsDirectoryMaker_MakeDir")

	mker := osDirectoryMaker{}
	err = mker.MakeDir(desired, 0755)
	if err != nil {
		t.Errorf("MakeDir() error = %s, want nil", err)
	}

	stat, err := os.Stat(desired)
	if err != nil {
		t.Errorf("MakeDir() directory stat error = %v, want nil", err)
		return
	}

	if !stat.IsDir() {
		t.Errorf("MakeDir() directory stat is not a directory")
	}
}

func TestOsDirectoryMaker_MakeTempDir(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "docker-plugin-cephfs")
	if err != nil {
		t.Fatalf("Could not initialize test tmpdir: %s", err)
	}
	defer must(func() error { return os.RemoveAll(tmpdir) })

	err = os.Setenv("TMPDIR", tmpdir)
	if err != nil {
		t.Fatalf("Could not set the tmpdir environment variable: %s", err)
	}
	defer must(func() error { return os.Unsetenv("TMPDIR") })

	mker := osDirectoryMaker{}
	got, err := mker.MakeTempDir()
	if err != nil {
		t.Errorf("MakeTempDir() error = %s, want nil", err)
		return
	}

	want := "docker-plugin-cephfs_mnt_"
	if !strings.Contains(got, want) {
		t.Errorf("MakeTempDir() got = %s, want %s", got, want)
		return
	}

	stat, err := os.Stat(got)
	if err != nil {
		t.Errorf("MakeTempDir() directory stat error = %v, want nil", err)
		return
	}

	if !stat.IsDir() {
		t.Errorf("MakeTempDir() directory stat is not a directory")
	}
}
