package volume

import (
	"io/ioutil"
	"os"
	"path"
	"reflect"
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

func TestCephConnStr(t *testing.T) {
	tests := []struct {
		name   string
		volume *Volume
		want   string
	}{
		{"default", &Volume{Servers: []string{"localhost"}}, "localhost:/"},
		{"multiple servers", &Volume{Servers: []string{"serv1", "serv2", "serv3"}}, "serv1,serv2,serv3:/"},
		{"remote path", &Volume{Servers: []string{"mon1", "mon2"}, RemotePath: "/service"}, "mon1,mon2:/service"},
		{"path missing slash", &Volume{Servers: []string{"localhost"}, RemotePath: "service"}, "localhost:/service"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CephConnStr(tt.volume.Servers, tt.volume.RemotePath)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CephConnStr() got = %v, want %v", got, tt.want)
			}
		})
	}
}
