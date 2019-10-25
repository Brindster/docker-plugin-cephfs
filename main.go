package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"github.com/boltdb/bolt"
	"gopkg.in/ini.v1"
	"log"
	"os"
	"os/exec"
	"path"
	"strings"
	"sync"
	"syscall"
	"time"

	//"github.com/ceph/go-ceph/cephfs"
	"github.com/docker/go-plugins-helpers/volume"
)

type cephfsVolume struct {
	ClientName string
	MountPoint string
	CreatedAt  string
	Status     map[string]interface{}

	MountOpts   string
	RemotePath  string
	Servers     []string
	ClusterName string
	ConfigPath  string
}

type cephfsDriver struct {
	configPath  string
	clientName  string
	clusterName string
	servers     []string
	*bolt.DB
	sync.RWMutex
}

const (
	defaultConfigPath  = "/etc/ceph/"
	defaultClientName  = "admin"
	defaultClusterName = "ceph"
)

var (
	socketName   = "cephfs"
	volumeBucket = []byte("volumes")
)

// Create will create a new volume
func (d cephfsDriver) Create(req *volume.CreateRequest) error {
	d.Lock()
	defer d.Unlock()

	v := cephfsVolume{
		ClientName:  d.clientName,
		MountPoint:  "",
		CreatedAt:   time.Now().Format(time.RFC3339),
		Status:      nil,
		Servers:     d.servers,
		ClusterName: d.clusterName,
		ConfigPath:  d.configPath,
	}

	for key, val := range req.Options {
		switch key {
		case "client_name":
			v.ClientName = val
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
func (d cephfsDriver) List() (*volume.ListResponse, error) {
	d.RLock()
	defer d.RUnlock()

	var vols []*volume.Volume
	vols = make([]*volume.Volume, 0)

	err := d.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(volumeBucket)
		return b.ForEach(func(k, v []byte) error {
			vol, err := unserialize(v)
			if err != nil {
				return err
			}

			vols = append(vols, &volume.Volume{
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

	return &volume.ListResponse{
		Volumes: vols,
	}, nil
}

// Get will return a single volume
func (d cephfsDriver) Get(req *volume.GetRequest) (*volume.GetResponse, error) {
	d.RLock()
	defer d.RUnlock()

	vol, err := d.fetchVol(req.Name)
	if err != nil {
		return nil, fmt.Errorf("could not read from db: %s", err)
	}

	return &volume.GetResponse{
		Volume: &volume.Volume{
			Name:       req.Name,
			Mountpoint: vol.MountPoint,
			CreatedAt:  vol.CreatedAt,
			Status:     vol.Status,
		},
	}, nil
}

// Remove will remove the volume
func (d cephfsDriver) Remove(req *volume.RemoveRequest) error {
	d.Lock()
	defer d.Unlock()

	err := d.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(volumeBucket)
		return b.Delete([]byte(req.Name))
	})

	if err != nil {
		return fmt.Errorf("could not update db: %s", err)
	}

	return nil
}

// Path will return the path to the volume
func (d cephfsDriver) Path(req *volume.PathRequest) (*volume.PathResponse, error) {
	d.RLock()
	defer d.RUnlock()

	vol, err := d.fetchVol(req.Name)
	if err != nil {
		return nil, fmt.Errorf("could not read from db: %s", err)
	}

	return &volume.PathResponse{Mountpoint: vol.MountPoint}, nil
}

// Mount will mount the volume
func (d cephfsDriver) Mount(req *volume.MountRequest) (*volume.MountResponse, error) {
	d.Lock()
	defer d.Unlock()

	vol, err := d.fetchVol(req.Name)
	if err != nil {
		return nil, fmt.Errorf("could not read from db: %s", err)
	}

	if err = vol.mount(req.ID); err != nil {
		return nil, fmt.Errorf("could not mount the vol: %s", err)
	}

	if err = d.saveVol(req.Name, *vol); err != nil {
		return nil, fmt.Errorf("could not update db: %s", err)
	}

	return &volume.MountResponse{
		Mountpoint: vol.MountPoint,
	}, nil
}

// Unmount will unmount the volume
func (d cephfsDriver) Unmount(req *volume.UnmountRequest) error {
	d.Lock()
	defer d.Unlock()

	vol, err := d.fetchVol(req.Name)
	if err != nil {
		return fmt.Errorf("could not read from db: %s", err)
	}

	return vol.unmount()
}

// Capabilities will return the capabilities of the cephfsDriver
func (d cephfsDriver) Capabilities() *volume.CapabilitiesResponse {
	return &volume.CapabilitiesResponse{
		Capabilities: volume.Capability{
			Scope: "local",
		},
	}
}

func (d cephfsDriver) fetchVol(name string) (*cephfsVolume, error) {
	var vol *cephfsVolume
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

func (d cephfsDriver) saveVol(name string, vol cephfsVolume) error {
	enc, err := vol.serialize()
	if err != nil {
		return fmt.Errorf("could not serialize volume: %s", err)
	}

	return d.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(volumeBucket)
		return b.Put([]byte(name), enc)
	})
}

func (v *cephfsVolume) mount(mnt string) error {
	mountPoint := path.Join(volume.DefaultDockerRootDirectory, mnt)
	if err := os.MkdirAll(mountPoint, 0755); err != nil {
		return fmt.Errorf("error creating mountpoint %s: %s", mountPoint, err)
	}

	secret, err := v.secret()
	if err != nil {
		return fmt.Errorf("error loading secret: %s", err)
	}

	opts := "name=" + v.ClientName + ",secret=" + secret
	if v.MountOpts != "" {
		opts = opts + "," + v.MountOpts
	}

	cmd := exec.Command("mount", "-t", "ceph", "-o", opts, v.connection(), mountPoint)
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Printf("Mount command returned: %s\n", out)
		return fmt.Errorf("error mounting: %s", err)
	}

	v.MountPoint = mountPoint

	return nil
}

func (v cephfsVolume) unmount() error {
	if v.MountPoint == "" {
		return fmt.Errorf("volume is not mounted")
	}

	err := syscall.Unmount(v.MountPoint, 0)
	if err != nil {
		return fmt.Errorf("failed unmounting volume: %s", err)
	}

	v.MountPoint = ""

	return nil
}

func (v cephfsVolume) connection() string {
	l := len(v.Servers)

	var conn string
	for id, server := range v.Servers {
		conn += server

		if id != l-1 {
			conn += ","
		}
	}

	conn += ":"
	if v.RemotePath == "" {
		conn += "/"
	} else {
		conn += v.RemotePath
	}

	return conn
}

func (v cephfsVolume) secret() (string, error) {
	file := defaultClusterName + ".client." + v.ClientName + ".keyring"
	keyring := strings.TrimRight(defaultConfigPath, "/") + "/" + file

	cnf, err := ini.Load(keyring)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("keyring not found: %s", keyring)
		}
		return "", fmt.Errorf("could not open keyring %s", keyring)
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

func (v cephfsVolume) serialize() ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func unserialize(in []byte) (*cephfsVolume, error) {
	var out cephfsVolume
	var buf bytes.Buffer

	buf.Write(in)

	dec := gob.NewDecoder(&buf)
	if err := dec.Decode(out); err != nil {
		return nil, err
	}

	return &out, nil
}

func envOrDefault(param, def string) string {
	if env, ok := os.LookupEnv(param); ok {
		return env
	}

	return def
}

func newCephFsDriver() cephfsDriver {
	// Open the my.db data file in your current directory.
	// It will be created if it doesn't exist.
	db, err := bolt.Open(socketName+".db", 0600, nil)
	if err != nil {
		log.Fatal(err)
	}

	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(volumeBucket)
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		log.Fatalf("Could not create bucket: %s", err)
	}

	srv := envOrDefault("SERVERS", "localhost")
	servers := strings.Split(srv, ",")

	driver := cephfsDriver{
		configPath:  defaultConfigPath,
		clientName:  envOrDefault("CLIENT_NAME", defaultClientName),
		clusterName: envOrDefault("CLUSTER_NAME", defaultClusterName),
		servers:     servers,
		DB:          db,
	}
	return driver
}

func main() {
	driver := newCephFsDriver()
	handler := volume.NewHandler(driver)
	log.Fatal(handler.ServeUnix(socketName, 1))
}
