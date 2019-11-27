package volume

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
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

// CephConnStr will return the ceph connection string for a list of servers and remote path
func CephConnStr(srvs []string, path string) string {
	l := len(srvs)

	var conn string
	for id, server := range srvs {
		conn += server

		if id != l-1 {
			conn += ","
		}
	}

	path = "/" + strings.TrimLeft(path, "/")

	return fmt.Sprintf("%s:%s", conn, path)
}

// EnvOrDefault will return the environment variable, or a default if it is not set
func EnvOrDefault(param, def string) string {
	if env, ok := os.LookupEnv(param); ok {
		return env
	}

	return def
}

// EnvOrDefaultBool will return the environment variable cast to a boolean as appropriate, or a default if it is not set
func EnvOrDefaultBool(param string, def bool) bool {
	if env, ok := os.LookupEnv(param); ok {
		switch strings.ToLower(env) {
		case "false", "n", "no", "off", "0":
			return false
		default:
			return true
		}
	}

	return def
}
