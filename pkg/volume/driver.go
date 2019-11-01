package volume

import (
	"fmt"
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

type Volume struct {
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

type Driver struct {
	ConfigPath  string
	ClientName  string
	ClusterName string
	MountPath   string
	Servers     []string
	secretCache map[string]string
	mnt         Mounter
	dir         DirectoryMaker
	*bolt.DB
	sync.RWMutex
}

type Mounter interface {
	Mount(source string, target string, fstype string, data string) error
	Unmount(target string) error
}

type DirectoryMaker interface {
	IsDir(dir string) bool
	MakeDir(dir string, mode os.FileMode) error
	MakeTempDir() (string, error)
}

const (
	defaultConfigPath  = "/etc/ceph/"
	defaultClientName  = "admin"
	defaultClusterName = "ceph"
)

var bucket = []byte("volumes")

// Create will create a new volume
func (d Driver) Create(req *plugin.CreateRequest) error {
	d.Lock()
	defer d.Unlock()

	v := Volume{
		ClientName:  d.ClientName,
		MountPoint:  "",
		CreatedAt:   time.Now().Format(time.RFC3339),
		Status:      nil,
		Servers:     d.Servers,
		ClusterName: d.ClusterName,
		ConfigPath:  d.ConfigPath,
		Keyring:     fmt.Sprintf("%s/%s.client.%s.keyring", strings.TrimRight(d.ConfigPath, "/"), d.ClusterName, d.ClientName),
		Connections: 0,
		RemotePath:  req.Name,
	}

	for key, val := range req.Options {
		switch key {
		case "client_name":
			v.ClientName = val
			v.Keyring = fmt.Sprintf("%s/%s.client.%s.keyring", strings.TrimRight(d.ConfigPath, "/"), d.ClusterName, val)
		case "keyring":
			if path.IsAbs(val) {
				v.Keyring = val
			} else {
				v.Keyring = fmt.Sprintf("%s/%s", strings.TrimRight(d.ConfigPath, "/"), val)
			}
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
func (d Driver) List() (*plugin.ListResponse, error) {
	d.RLock()
	defer d.RUnlock()

	var vols []*plugin.Volume
	vols = make([]*plugin.Volume, 0)

	err := d.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucket)
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
func (d Driver) Get(req *plugin.GetRequest) (*plugin.GetResponse, error) {
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
func (d Driver) Remove(req *plugin.RemoveRequest) error {
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
		b := tx.Bucket(bucket)
		return b.Delete([]byte(req.Name))
	})

	if err != nil {
		return fmt.Errorf("could not update db: %s", err)
	}

	return nil
}

// Path will return the path to the volume
func (d Driver) Path(req *plugin.PathRequest) (*plugin.PathResponse, error) {
	d.RLock()
	defer d.RUnlock()

	vol, err := d.fetchVol(req.Name)
	if err != nil {
		return nil, fmt.Errorf("could not read from db: %s", err)
	}

	return &plugin.PathResponse{Mountpoint: vol.MountPoint}, nil
}

// Mount will mount the volume
func (d Driver) Mount(req *plugin.MountRequest) (*plugin.MountResponse, error) {
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
func (d Driver) Unmount(req *plugin.UnmountRequest) error {
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
func (d Driver) Capabilities() *plugin.CapabilitiesResponse {
	return &plugin.CapabilitiesResponse{
		Capabilities: plugin.Capability{
			Scope: "local",
		},
	}
}

func (d Driver) fetchVol(name string) (*Volume, error) {
	var vol *Volume
	err := d.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucket)
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

func (d Driver) saveVol(name string, vol Volume) error {
	enc, err := serialize(vol)
	if err != nil {
		return fmt.Errorf("could not serialize volume: %s", err)
	}

	return d.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucket)
		return b.Put([]byte(name), enc)
	})
}

func (d Driver) mountVolume(v *Volume, mnt string) error {
	var mountPoint string
	var err error

	if v.Connections > 0 {
		v.Connections++
		return nil
	}

	if v.RemotePath != "" && v.RemotePath != "/" {
		mountPoint, err = d.dir.MakeTempDir()
		if err != nil {
			return fmt.Errorf("error creating temporary mountpoint: %s", err)
		}

		defer os.RemoveAll(mountPoint)
	} else {
		mountPoint = path.Join(d.MountPath, mnt)
		if err := d.dir.MakeDir(mountPoint, 0755); err != nil {
			return fmt.Errorf("error creating mountpoint %s: %s", mountPoint, err)
		}
	}

	secret, err := d.secret(*v)
	if err != nil {
		return fmt.Errorf("error loading secret: %s", err)
	}

	opts := "name=" + v.ClientName + ",secret=" + secret
	if v.MountOpts != "" {
		opts = opts + "," + v.MountOpts
	}

	connStr := CephConnStr(v.Servers, "/")
	if err = d.mnt.Mount(connStr, mountPoint, "ceph", opts); err != nil {
		return fmt.Errorf("error mounting: %s", err)
	}

	if v.RemotePath != "" && v.RemotePath != "/" {
		desiredMount := path.Join(mountPoint, v.RemotePath)

		if !d.dir.IsDir(desiredMount) {
			if err := d.dir.MakeDir(desiredMount, 0755); err != nil {
				return fmt.Errorf("unable to make remote mount: %s", err)
			}
		}

		if err = d.mnt.Unmount(mountPoint); err != nil {
			return fmt.Errorf("failed unmounting volume: %s", err)
		}

		connStr = CephConnStr(v.Servers, v.RemotePath)
		mountPoint = path.Join(d.MountPath, mnt)
		if err = d.mnt.Mount(connStr, mountPoint, "ceph", opts); err != nil {
			return fmt.Errorf("error mounting: %s", err)
		}
	}

	v.Connections++
	v.MountPoint = mountPoint
	return nil
}

func (d Driver) unmountVolume(v *Volume) error {
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

func (d Driver) secret(v Volume) (string, error) {
	if secret, ok := d.secretCache[v.ClientName]; ok {
		return secret, nil
	}

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

	if d.secretCache == nil {
		d.secretCache = make(map[string]string)
	}

	secret := sec.Key("key").String()
	d.secretCache[v.ClientName] = secret

	return secret, nil
}

func NewDriver(db *bolt.DB) Driver {
	err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucket)
		return err
	})

	if err != nil {
		log.Fatalf("Could not create bucket: %s", err)
	}

	srv := EnvOrDefault("SERVERS", "localhost")
	servers := strings.Split(srv, ",")

	driver := Driver{
		ConfigPath:  defaultConfigPath,
		ClientName:  EnvOrDefault("CLIENT_NAME", defaultClientName),
		ClusterName: EnvOrDefault("CLUSTER_NAME", defaultClusterName),
		MountPath:   EnvOrDefault("VOLUME_MOUNT_ROOT", plugin.DefaultDockerRootDirectory),
		Servers:     servers,
		mnt:         fsMounter{},
		dir:         osDirectoryMaker{},
		DB:          db,
	}
	return driver
}
