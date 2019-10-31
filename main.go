package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/boltdb/bolt"
	plugin "github.com/docker/go-plugins-helpers/volume"
	"gopkg.in/ini.v1"
)

type volume struct {
	ClientName string
	MountPoint string
	CreatedAt  string
	Status     map[string]interface{}

	MountOpts   string
	RemotePath  string
	Servers     []string
	ClusterName string
	ConfigPath  string
	Keyring     string

	Connections int
}

type driver struct {
	configPath  string
	clientName  string
	clusterName string
	servers     []string
	mnt         mounter
	*bolt.DB
	sync.RWMutex
}

type mounter interface {
	Mount(source string, target string, fstype string, data string) error
	Unmount(target string) error
}

type fsMounter struct{}

const (
	defaultConfigPath  = "/etc/ceph/"
	defaultClientName  = "admin"
	defaultClusterName = "ceph"
)

var (
	mountDir     = plugin.DefaultDockerRootDirectory
	socketName   = "cephfs"
	volumeBucket = []byte("volumes")
)

// Create will create a new volume
func (d driver) Create(req *plugin.CreateRequest) error {
	d.Lock()
	defer d.Unlock()

	v := volume{
		ClientName:  d.clientName,
		MountPoint:  "",
		CreatedAt:   time.Now().Format(time.RFC3339),
		Status:      nil,
		Servers:     d.servers,
		ClusterName: d.clusterName,
		ConfigPath:  d.configPath,
		Keyring:     fmt.Sprintf("%s/%s.client.%s.keyring", strings.TrimRight(d.configPath, "/"), d.clusterName, d.clientName),
		Connections: 0,
	}

	for key, val := range req.Options {
		switch key {
		case "client_name":
			v.ClientName = val
			v.Keyring = fmt.Sprintf("%s/%s.client.%s.keyring", strings.TrimRight(d.configPath, "/"), d.clusterName, val)
		case "keyring":
			v.Keyring = val
		case "mount_opts":
			v.MountOpts = val
		case "remote_path":
			v.RemotePath = val
		case "servers":
			v.Servers = strings.Split(val, ",")
		}
	}

	err := d.saveVol(req.Name, v)
	if err != nil {
		return fmt.Errorf("could not write to db: %s", err)
	}

	return nil
}

// List will list all of the created volumes
func (d driver) List() (*plugin.ListResponse, error) {
	d.RLock()
	defer d.RUnlock()

	var vols []*plugin.Volume
	vols = make([]*plugin.Volume, 0)

	err := d.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(volumeBucket)
		return b.ForEach(func(k, v []byte) error {
			vol, err := unserialize(v)
			if err != nil {
				return err
			}

			vols = append(vols, &plugin.Volume{
				Name:       string(k),
				Mountpoint: vol.MountPoint,
				CreatedAt:  vol.CreatedAt,
				Status:     vol.Status,
			})

			return nil
		})
	})

	if err != nil {
		return nil, fmt.Errorf("could not read from db: %s", err)
	}

	return &plugin.ListResponse{
		Volumes: vols,
	}, nil
}

// Get will return a single volume
func (d driver) Get(req *plugin.GetRequest) (*plugin.GetResponse, error) {
	d.RLock()
	defer d.RUnlock()

	vol, err := d.fetchVol(req.Name)
	if err != nil {
		return nil, fmt.Errorf("could not read from db: %s", err)
	}

	return &plugin.GetResponse{
		Volume: &plugin.Volume{
			Name:       req.Name,
			Mountpoint: vol.MountPoint,
			CreatedAt:  vol.CreatedAt,
			Status:     vol.Status,
		},
	}, nil
}

// Remove will remove the volume
func (d driver) Remove(req *plugin.RemoveRequest) error {
	d.Lock()
	defer d.Unlock()

	vol, err := d.fetchVol(req.Name)
	if err != nil {
		return fmt.Errorf("could not read from db: %s", err)
	}

	if vol.Connections > 0 {
		return fmt.Errorf("volume is still mounted")
	}

	err = d.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(volumeBucket)
		return b.Delete([]byte(req.Name))
	})

	if err != nil {
		return fmt.Errorf("could not update db: %s", err)
	}

	return nil
}

// Path will return the path to the volume
func (d driver) Path(req *plugin.PathRequest) (*plugin.PathResponse, error) {
	d.RLock()
	defer d.RUnlock()

	vol, err := d.fetchVol(req.Name)
	if err != nil {
		return nil, fmt.Errorf("could not read from db: %s", err)
	}

	return &plugin.PathResponse{Mountpoint: vol.MountPoint}, nil
}

// Mount will mount the volume
func (d driver) Mount(req *plugin.MountRequest) (*plugin.MountResponse, error) {
	d.Lock()
	defer d.Unlock()

	vol, err := d.fetchVol(req.Name)
	if err != nil {
		return nil, fmt.Errorf("could not read from db: %s", err)
	}

	if err = d.mountVolume(vol, req.ID); err != nil {
		return nil, fmt.Errorf("could not mount the vol: %s", err)
	}

	if err = d.saveVol(req.Name, *vol); err != nil {
		return nil, fmt.Errorf("could not update db: %s", err)
	}

	return &plugin.MountResponse{
		Mountpoint: vol.MountPoint,
	}, nil
}

// Unmount will unmount the volume
func (d driver) Unmount(req *plugin.UnmountRequest) error {
	d.Lock()
	defer d.Unlock()

	vol, err := d.fetchVol(req.Name)
	if err != nil {
		return fmt.Errorf("could not read from db: %s", err)
	}

	if err = d.unmountVolume(vol); err != nil {
		return fmt.Errorf("could not unmount vol: %s", err)
	}

	if err = d.saveVol(req.Name, *vol); err != nil {
		return fmt.Errorf("could not update db: %s", err)
	}

	return nil
}

// Capabilities will return the capabilities of the driver
func (d driver) Capabilities() *plugin.CapabilitiesResponse {
	return &plugin.CapabilitiesResponse{
		Capabilities: plugin.Capability{
			Scope: "local",
		},
	}
}

func (d driver) fetchVol(name string) (*volume, error) {
	var vol *volume
	err := d.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(volumeBucket)
		v := b.Get([]byte(name))

		if v == nil {
			return fmt.Errorf("no volume named %s found", name)
		}

		var err error
		vol, err = unserialize(v)
		return err
	})

	return vol, err
}

func (d driver) saveVol(name string, vol volume) error {
	enc, err := vol.serialize()
	if err != nil {
		return fmt.Errorf("could not serialize volume: %s", err)
	}

	return d.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(volumeBucket)
		return b.Put([]byte(name), enc)
	})
}

func (d driver) mountVolume(v *volume, mnt string) error {
	var mountPoint string
	var err error

	if v.Connections > 0 {
		v.Connections++
		return nil
	}

	if v.RemotePath != "" && v.RemotePath != "/" {
		mountPoint, err = ioutil.TempDir("", "docker-plugin-cephfs_mnt_")
		if err != nil {
			return fmt.Errorf("error creating temporary mountpoint: %s", err)
		}
	} else {
		mountPoint = path.Join(mountDir, mnt)
		if err := os.MkdirAll(mountPoint, 0755); err != nil {
			return fmt.Errorf("error creating mountpoint %s: %s", mountPoint, err)
		}
	}

	secret, err := v.secret()
	if err != nil {
		return fmt.Errorf("error loading secret: %s", err)
	}

	opts := "name=" + v.ClientName + ",secret=" + secret
	if v.MountOpts != "" {
		opts = opts + "," + v.MountOpts
	}

	connStr := connstr(v.Servers, "/")
	if err = d.mnt.Mount(connStr, mountPoint, "ceph", "-o "+opts); err != nil {
		return fmt.Errorf("error mounting: %s", err)
	}

	if v.RemotePath != "" && v.RemotePath != "/" {
		desiredMount := path.Join(mountPoint, v.RemotePath)
		stat, err := os.Stat(desiredMount)
		if !os.IsNotExist(err) {
			if err = d.mnt.Unmount(mountPoint); err != nil {
				return fmt.Errorf("failed unmounting volume: %s", err)
			}

			if !stat.IsDir() {
				return fmt.Errorf("remote mount is a file")
			}

			return nil
		}

		if err = os.MkdirAll(desiredMount, 0755); err != nil {
			return fmt.Errorf("unable to make remote mount: %s", err)
		}

		if err = d.mnt.Unmount(mountPoint); err != nil {
			return fmt.Errorf("failed unmounting volume: %s", err)
		}

		connStr = connstr(v.Servers, v.RemotePath)
		mountPoint = path.Join(mountDir, mnt)
		if err = d.mnt.Mount(connStr, mountPoint, "ceph", "-o "+opts); err != nil {
			return fmt.Errorf("error mounting: %s", err)
		}
	}

	v.Connections++
	v.MountPoint = mountPoint
	return nil
}

func (d driver) unmountVolume(v *volume) error {
	if v.MountPoint == "" {
		return fmt.Errorf("volume is not mounted")
	}

	v.Connections--
	if v.Connections <= 0 {
		err := d.mnt.Unmount(v.MountPoint)
		if err != nil {
			return fmt.Errorf("failed unmounting volume: %s", err)
		}

		v.MountPoint = ""
		v.Connections = 0
	}

	return nil
}

func (v volume) secret() (string, error) {
	cnf, err := ini.Load(v.Keyring)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("keyring not found: %s", v.Keyring)
		}
		return "", fmt.Errorf("could not open keyring %s", v.Keyring)
	}

	sec, err := cnf.GetSection("client." + v.ClientName)
	if err != nil {
		return "", fmt.Errorf("keyring did not contain client details for %s", v.ClientName)
	}

	if !sec.HasKey("key") {
		return "", fmt.Errorf("keyring did not contain key for %s", v.ClientName)
	}

	return sec.Key("key").String(), nil
}

func (v volume) serialize() ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func unserialize(in []byte) (*volume, error) {
	var buf bytes.Buffer

	buf.Write(in)

	out := &volume{}
	dec := gob.NewDecoder(&buf)
	if err := dec.Decode(out); err != nil {
		return nil, err
	}

	return out, nil
}

func connstr(srvs []string, path string) string {
	l := len(srvs)

	var conn string
	for id, server := range srvs {
		conn += server

		if id != l-1 {
			conn += ","
		}
	}

	if path == "" {
		path = "/"
	}

	return fmt.Sprintf("%s:%s", conn, path)
}

func envOrDefault(param, def string) string {
	if env, ok := os.LookupEnv(param); ok {
		return env
	}

	return def
}

func newDriver(db *bolt.DB) driver {
	err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(volumeBucket)
		return err
	})

	if err != nil {
		log.Fatalf("Could not create bucket: %s", err)
	}

	srv := envOrDefault("SERVERS", "localhost")
	servers := strings.Split(srv, ",")

	driver := driver{
		configPath:  defaultConfigPath,
		clientName:  envOrDefault("CLIENT_NAME", defaultClientName),
		clusterName: envOrDefault("CLUSTER_NAME", defaultClusterName),
		servers:     servers,
		mnt:         fsMounter{},
		DB:          db,
	}
	return driver
}

func main() {
	// Open the my.db data file in your current directory.
	// It will be created if it doesn't exist.
	db, err := bolt.Open(socketName+".db", 0600, nil)
	if err != nil {
		log.Fatalf("Could not open the database: %s", err)
	}

	driver := newDriver(db)
	handler := plugin.NewHandler(driver)
	log.Fatal(handler.ServeUnix(socketName, 1))
}
