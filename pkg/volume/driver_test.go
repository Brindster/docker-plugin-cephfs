package volume

import (
	"errors"
	"fmt"
	"github.com/boltdb/bolt"
	plugin "github.com/docker/go-plugins-helpers/volume"
	"io/ioutil"
	"log"
	"os"
	"path"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func Test_newDriver(t *testing.T) {
	db := genMockDb()
	defer must(db.Close)

	tests := []struct {
		name string
		envs []string
		want *Driver
	}{
		{"default", []string{}, &Driver{
			ConfigPath:  defaultConfigPath,
			ClientName:  defaultClientName,
			ClusterName: defaultClusterName,
			Servers:     []string{"localhost"},
			DB:          db,
			RWMutex:     sync.RWMutex{},
		}},
		{"server set", []string{"SERVERS=ceph1"}, &Driver{
			ConfigPath:  defaultConfigPath,
			ClientName:  defaultClientName,
			ClusterName: defaultClusterName,
			Servers:     []string{"ceph1"},
			DB:          db,
			RWMutex:     sync.RWMutex{},
		}},
		{"servers set", []string{"SERVERS=ceph1,ceph2,ceph3"}, &Driver{
			ConfigPath:  defaultConfigPath,
			ClientName:  defaultClientName,
			ClusterName: defaultClusterName,
			Servers:     []string{"ceph1", "ceph2", "ceph3"},
			DB:          db,
			RWMutex:     sync.RWMutex{},
		}},
	}

	for _, tt := range tests {
		func() {
			defer os.Clearenv()
			for _, env := range tt.envs {
				prts := strings.SplitN(env, "=", 2)
				if err := os.Setenv(prts[0], prts[1]); err != nil {
					log.Fatalf("Unable to set environment variables")
				}
			}

			got := NewDriver(db)
			if got.ClusterName != tt.want.ClusterName {
				t.Errorf("newDriver().clusterName = %v, want %v", got.ClusterName, tt.want.ClusterName)
			}

			if got.ClientName != tt.want.ClientName {
				t.Errorf("newDriver().clientName = %v, want %v", got.ClientName, tt.want.ClientName)
			}

			if got.ConfigPath != tt.want.ConfigPath {
				t.Errorf("newDriver().configPath = %v, want %v", got.ConfigPath, tt.want.ConfigPath)
			}

			if !reflect.DeepEqual(got.Servers, tt.want.Servers) {
				t.Errorf("newDriver().servers = %v, want %v", got.Servers, tt.want.Servers)
			}

			if !reflect.DeepEqual(got.DB, tt.want.DB) {
				t.Errorf("newDriver().DB = %v, want %v", got.DB, tt.want.DB)
			}
		}()
	}
}

func TestDriver_Capabilities(t *testing.T) {
	d := Driver{}
	got := d.Capabilities()
	want := &plugin.CapabilitiesResponse{Capabilities: plugin.Capability{Scope: "local"}}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("Capabilities() = %v, want %v", got, want)
	}
}

func TestDriver_Create(t *testing.T) {
	type fields struct {
		configPath  string
		clientName  string
		clusterName string
		servers     []string
		DB          *bolt.DB
		RWMutex     sync.RWMutex
	}
	type args struct {
		req *plugin.CreateRequest
	}
	drv := fields{
		configPath:  defaultConfigPath,
		clientName:  defaultClientName,
		clusterName: defaultClusterName,
		servers:     []string{"localhost"},
		DB:          genMockDb(),
		RWMutex:     sync.RWMutex{},
	}
	defer must(drv.DB.Close)
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
		want    *Volume
	}{
		{"valid", drv, args{&plugin.CreateRequest{Name: "test.1"}}, false, &Volume{
			ClientName:  defaultClientName,
			Servers:     []string{"localhost"},
			ClusterName: defaultClusterName,
			ConfigPath:  defaultConfigPath,
			RemotePath:  "test.1",
			Keyring:     "/etc/ceph/ceph.client.admin.keyring",
		}},
		{"with client_name", drv, args{&plugin.CreateRequest{Name: "test.2", Options: map[string]string{"client_name": "user"}}}, false, &Volume{
			ClientName:  "user",
			Servers:     []string{"localhost"},
			ClusterName: defaultClusterName,
			ConfigPath:  defaultConfigPath,
			RemotePath:  "test.2",
			Keyring:     "/etc/ceph/ceph.client.user.keyring",
		}},
		{"with mount_opts", drv, args{&plugin.CreateRequest{Name: "test.3", Options: map[string]string{"mount_opts": "name=user,secret=abc"}}}, false, &Volume{
			ClientName:  defaultClientName,
			MountOpts:   "name=user,secret=abc",
			Servers:     []string{"localhost"},
			ClusterName: defaultClusterName,
			ConfigPath:  defaultConfigPath,
			RemotePath:  "test.3",
			Keyring:     "/etc/ceph/ceph.client.admin.keyring",
		}},
		{"with remote_path", drv, args{&plugin.CreateRequest{Name: "test.4", Options: map[string]string{"remote_path": "/data/mnt"}}}, false, &Volume{
			ClientName:  defaultClientName,
			RemotePath:  "/data/mnt",
			Servers:     []string{"localhost"},
			ClusterName: defaultClusterName,
			ConfigPath:  defaultConfigPath,
			Keyring:     "/etc/ceph/ceph.client.admin.keyring",
		}},
		{"with servers", drv, args{&plugin.CreateRequest{Name: "test.5", Options: map[string]string{"servers": "monitor1:6798,monitor2:6798"}}}, false, &Volume{
			ClientName:  defaultClientName,
			Servers:     []string{"monitor1:6798", "monitor2:6798"},
			ClusterName: defaultClusterName,
			ConfigPath:  defaultConfigPath,
			RemotePath:  "test.5",
			Keyring:     "/etc/ceph/ceph.client.admin.keyring",
		}},
		{"duplicate name", drv, args{&plugin.CreateRequest{Name: "test.1"}}, false, &Volume{
			ClientName:  defaultClientName,
			Servers:     []string{"localhost"},
			ClusterName: defaultClusterName,
			ConfigPath:  defaultConfigPath,
			RemotePath:  "test.1",
			Keyring:     "/etc/ceph/ceph.client.admin.keyring",
		}},
		{"with keyring", drv, args{&plugin.CreateRequest{Name: "test.6", Options: map[string]string{"keyring": "/etc/ceph/test.keyring"}}}, false, &Volume{
			ClientName:  defaultClientName,
			Servers:     []string{"localhost"},
			ClusterName: defaultClusterName,
			ConfigPath:  defaultConfigPath,
			RemotePath:  "test.6",
			Keyring:     "/etc/ceph/test.keyring",
		}},
		{"with keyring name only", drv, args{&plugin.CreateRequest{Name: "test.7", Options: map[string]string{"keyring": "test.keyring"}}}, false, &Volume{
			ClientName:  defaultClientName,
			Servers:     []string{"localhost"},
			ClusterName: defaultClusterName,
			ConfigPath:  defaultConfigPath,
			RemotePath:  "test.7",
			Keyring:     "/etc/ceph/test.keyring",
		}},
		{"with cluster_name", drv, args{&plugin.CreateRequest{Name: "test.8", Options: map[string]string{"cluster_name": "dev"}}}, false, &Volume{
			ClientName:  defaultClientName,
			Servers:     []string{"localhost"},
			ClusterName: "dev",
			ConfigPath:  defaultConfigPath,
			RemotePath:  "test.8",
			Keyring:     "/etc/ceph/dev.client.admin.keyring",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Driver{
				ConfigPath:  tt.fields.configPath,
				ClientName:  tt.fields.clientName,
				ClusterName: tt.fields.clusterName,
				Servers:     tt.fields.servers,
				mnt:         &mockMounter{},
				DB:          tt.fields.DB,
				RWMutex:     tt.fields.RWMutex,
			}
			if err := d.Create(tt.args.req); (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			} else {
				got, err := d.fetchVol(tt.args.req.Name)
				if err != nil {
					t.Errorf("Create() fetchVol: %s", err)
				}

				got.CreatedAt = ""
				if !reflect.DeepEqual(got, tt.want) {
					t.Errorf("Create() got = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestDriver_Get(t *testing.T) {
	type fields struct {
		configPath  string
		clientName  string
		clusterName string
		servers     []string
		DB          *bolt.DB
		RWMutex     sync.RWMutex
	}
	type args struct {
		req *plugin.GetRequest
	}
	drv := fields{
		configPath:  defaultConfigPath,
		clientName:  defaultClientName,
		clusterName: defaultClusterName,
		servers:     []string{"localhost"},
		DB:          genMockDb(),
		RWMutex:     sync.RWMutex{},
	}
	defer must(drv.DB.Close)
	vols := []Volume{
		{
			MountPoint: "",
			CreatedAt:  "2019-01-01T01:01:01Z",
			Status:     nil,
		},
		{
			MountPoint: "/var/www/app/data",
			CreatedAt:  "2019-02-02T02:02:02Z",
			Status:     nil,
		},
	}
	must(func() error { return prepareMockData(drv.DB, vols) })

	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *plugin.GetResponse
		wantErr bool
	}{
		{"get existing", drv, args{&plugin.GetRequest{Name: "test.1"}}, &plugin.GetResponse{Volume: &plugin.Volume{
			Name:       "test.1",
			Mountpoint: "",
			CreatedAt:  "2019-01-01T01:01:01Z",
			Status:     nil,
		}}, false},
		{"get mounted", drv, args{&plugin.GetRequest{Name: "test.2"}}, &plugin.GetResponse{Volume: &plugin.Volume{
			Name:       "test.2",
			Mountpoint: "/var/www/app/data",
			CreatedAt:  "2019-02-02T02:02:02Z",
			Status:     nil,
		}}, false},
		{"non existing", drv, args{&plugin.GetRequest{Name: "invalid"}}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Driver{
				ConfigPath:  tt.fields.configPath,
				ClientName:  tt.fields.clientName,
				ClusterName: tt.fields.clusterName,
				Servers:     tt.fields.servers,
				mnt:         &mockMounter{},
				DB:          tt.fields.DB,
				RWMutex:     tt.fields.RWMutex,
			}
			got, err := d.Get(tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("Get() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Get() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDriver_List(t *testing.T) {
	db := genMockDb()
	defer must(db.Close)

	drv := Driver{
		ConfigPath:  defaultConfigPath,
		ClientName:  defaultClientName,
		ClusterName: defaultClusterName,
		Servers:     []string{"localhost"},
		mnt:         &mockMounter{},
		DB:          db,
		RWMutex:     sync.RWMutex{},
	}

	vols := []Volume{
		{
			MountPoint: "",
			CreatedAt:  "2019-01-01T01:01:01Z",
			Status:     nil,
		},
		{
			MountPoint: "/var/www/app/data",
			CreatedAt:  "2019-02-02T02:02:02Z",
			Status:     nil,
		},
	}
	must(func() error { return prepareMockData(db, vols) })

	got, err := drv.List()
	if err != nil {
		t.Errorf("List() error = %v, wanted nil", err)
	}

	want := &plugin.ListResponse{Volumes: []*plugin.Volume{{
		Name:       "test.1",
		Mountpoint: "",
		CreatedAt:  "2019-01-01T01:01:01Z",
		Status:     nil,
	}, {
		Name:       "test.2",
		Mountpoint: "/var/www/app/data",
		CreatedAt:  "2019-02-02T02:02:02Z",
		Status:     nil,
	}}}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("List() got = %v, want %v", got, want)
	}
}

func TestDriver_Mount(t *testing.T) {
	db := genMockDb()
	defer must(db.Close)

	mountRoot, remove := prepareTempDir()
	defer must(remove)

	drv := Driver{
		ConfigPath:  defaultConfigPath,
		ClientName:  defaultClientName,
		ClusterName: defaultClusterName,
		MountPath:   mountRoot,
		Servers:     []string{"localhost"},
		mnt:         &mockMounter{},
		dir:         &osDirectoryMaker{},
		DB:          db,
		RWMutex:     sync.RWMutex{},
	}

	keyring, cleanup := prepareKeyring("[client.admin]\nkey = ABC123")
	defer must(cleanup)

	vols := []Volume{
		{
			MountPoint: "",
			CreatedAt:  "2019-01-01T01:01:01Z",
			Status:     nil,
			ClientName: "admin",
			Keyring:    keyring,
		},
		{
			MountPoint: "",
			CreatedAt:  "2019-01-01T01:01:01Z",
			Status:     nil,
			ClientName: "admin",
			Keyring:    keyring,
		},
	}
	must(func() error { return prepareMockData(db, vols) })

	type args struct {
		req  *plugin.MountRequest
		mrsp error
	}
	tests := []struct {
		name    string
		args    args
		want    *plugin.MountResponse
		wantErr bool
	}{
		{"mount volume", args{&plugin.MountRequest{Name: "test.1", ID: "624F80C6-F050-42BF-8B02-387AA892782F"}, nil}, &plugin.MountResponse{Mountpoint: path.Join(mountRoot, "624F80C6-F050-42BF-8B02-387AA892782F")}, false},
		{"mount fail", args{&plugin.MountRequest{Name: "test.2", ID: "624F80C6-F050-42BF-8B02-387AA892782F"}, errors.New("failed mounting")}, nil, true},
		{"volume not found", args{&plugin.MountRequest{Name: "test.3", ID: ""}, nil}, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			drv.mnt = &mockMounter{MountResponse: tt.args.mrsp}

			got, err := drv.Mount(tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("Mount() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Mount() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDriver_Path(t *testing.T) {
	db := genMockDb()
	defer must(db.Close)
	vols := []Volume{
		{
			MountPoint: "/var/lib/docker-volumes/D4BE6F35-11E8-4735-A330-3BA36B5B9913",
			CreatedAt:  "2019-01-01T01:01:01Z",
			Status:     nil,
		},
		{
			MountPoint: "",
			CreatedAt:  "2019-02-02T02:02:02Z",
			Status:     nil,
		},
	}
	must(func() error { return prepareMockData(db, vols) })

	type args struct {
		req *plugin.PathRequest
	}
	tests := []struct {
		name    string
		args    args
		want    *plugin.PathResponse
		wantErr bool
	}{
		{"mounted", args{&plugin.PathRequest{Name: "test.1"}}, &plugin.PathResponse{Mountpoint: "/var/lib/docker-volumes/D4BE6F35-11E8-4735-A330-3BA36B5B9913"}, false},
		{"not mounted", args{&plugin.PathRequest{Name: "test.2"}}, &plugin.PathResponse{Mountpoint: ""}, false},
		{"non existing", args{&plugin.PathRequest{Name: "test.3"}}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Driver{mnt: &mockMounter{}, DB: db}
			got, err := d.Path(tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("Path() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Path() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDriver_Remove(t *testing.T) {
	db := genMockDb()
	defer must(db.Close)

	vols := []Volume{
		{
			MountPoint:  "/var/lib/docker-volumes/D4BE6F35-11E8-4735-A330-3BA36B5B9913",
			CreatedAt:   "2019-01-01T01:01:01Z",
			Status:      nil,
			Connections: 1,
		},
		{
			MountPoint:  "",
			CreatedAt:   "2019-02-02T02:02:02Z",
			Status:      nil,
			Connections: 0,
		},
	}
	must(func() error { return prepareMockData(db, vols) })

	type args struct {
		req *plugin.RemoveRequest
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{"mounted", args{&plugin.RemoveRequest{Name: "test.1"}}, true},
		{"not mounted", args{&plugin.RemoveRequest{Name: "test.2"}}, false},
		{"non existing", args{&plugin.RemoveRequest{Name: "test.3"}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Driver{mnt: &mockMounter{}, DB: db}
			err := d.Remove(tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("Remove() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil {
				_ = db.View(func(tx *bolt.Tx) error {
					b := tx.Bucket(bucket)
					d := b.Get([]byte(tt.args.req.Name))
					if d != nil {
						t.Errorf("Remove() did not delete volume")
					}

					return nil
				})
			}
		})
	}
}

func TestDriver_Unmount(t *testing.T) {
	db := genMockDb()
	defer must(db.Close)

	vols := []Volume{
		{
			MountPoint:  "/var/lib/docker-volumes/D4BE6F35-11E8-4735-A330-3BA36B5B9913",
			CreatedAt:   "2019-01-01T01:01:01Z",
			Status:      nil,
			Connections: 1,
		},
		{
			MountPoint:  "/var/lib/docker-volumes/D4BE6F35-11E8-4735-A330-3BA36B5B9913",
			CreatedAt:   "2019-02-02T02:02:02Z",
			Status:      nil,
			Connections: 2,
		},
		{
			MountPoint: "",
			CreatedAt:  "2019-02-02T02:02:02Z",
			Status:     nil,
		},
	}
	must(func() error { return prepareMockData(db, vols) })

	type args struct {
		req *plugin.UnmountRequest
	}
	tests := []struct {
		name     string
		args     args
		umntResp error
		wantErr  bool
	}{
		{"mounted", args{&plugin.UnmountRequest{Name: "test.1"}}, nil, false},
		{"mounted twice", args{&plugin.UnmountRequest{Name: "test.2"}}, nil, false},
		{"non mounted", args{&plugin.UnmountRequest{Name: "test.3"}}, errors.New("not mounted"), true},
		{"non existing", args{&plugin.UnmountRequest{Name: "test.4"}}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Driver{mnt: &mockMounter{UnmountResponse: tt.umntResp}, DB: db}
			err := d.Unmount(tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("Unmount() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil && !tt.wantErr {
				_ = db.View(func(tx *bolt.Tx) error {
					b := tx.Bucket(bucket)
					data := b.Get([]byte(tt.args.req.Name))
					if data == nil {
						t.Errorf("Unmount() volume is removed")
						return nil
					}
					vol, err := unserialize(data)
					if err != nil {
						t.Errorf("Unmount() volume could not be unserialized")
						return nil
					}

					if vol.MountPoint != "" {
						t.Errorf("Unmount() volume remains mounted")
					}

					return nil
				})
			}
		})
	}
}

func TestDriver_mountVolume(t *testing.T) {
	mnt := mockMounter{}
	dir := mockDirectoryMaker{}
	drv := Driver{
		ConfigPath:  defaultConfigPath,
		ClientName:  defaultClientName,
		ClusterName: defaultClusterName,
		MountPath:   plugin.DefaultDockerRootDirectory,
		Servers:     []string{"localhost"},
		dir:         &dir,
		mnt:         &mnt,
		DB:          &bolt.DB{},
	}

	keyring, cleanup := prepareKeyring("[client.admin]\nkey = ABC123")
	defer must(cleanup)

	vol := &Volume{
		ClientName: "admin",
		Keyring:    keyring,
		Servers:    []string{"localhost"},
	}
	got := drv.mountVolume(vol, "B1BF0ABD-85A6-4004-A10F-326D8F0C4C6F")
	if got != nil {
		t.Errorf("mountVolume() error = %s, expected nil", got)
		return
	}

	if !mnt.receivedCallWithArgs("Mount", "localhost:/", "/var/lib/docker-volumes/B1BF0ABD-85A6-4004-A10F-326D8F0C4C6F", "ceph", "name=admin,secret=ABC123") {
		t.Errorf("mountVolume() did not call mount with expected args")
		return
	}

	if vol.MountPoint != "/var/lib/docker-volumes/B1BF0ABD-85A6-4004-A10F-326D8F0C4C6F" {
		t.Errorf("mountVolume() did update volume mount point")
		return
	}

	if vol.Connections != 1 {
		t.Errorf("mountVolume() did update volume connections")
		return
	}
}

func TestDriver_mountVolume_alreadyConnected(t *testing.T) {
	mnt := mockMounter{}
	dir := mockDirectoryMaker{}
	drv := Driver{DB: &bolt.DB{}, mnt: &mnt, dir: &dir}
	vol := &Volume{Connections: 1}

	got := drv.mountVolume(vol, "B1BF0ABD-85A6-4004-A10F-326D8F0C4C6F")
	if got != nil {
		t.Errorf("mountVolume() error = %s, expected nil", got)
		return
	}

	if mnt.receivedCall("mount") {
		t.Errorf("mountVolume() unexpectedly called mount method")
		return
	}

	if vol.Connections != 2 {
		t.Errorf("mountVolume() did not increase connections on volume")
	}
}

func TestDriver_mountVolume_withRemotePath(t *testing.T) {
	mnt := mockMounter{}
	dir := mockDirectoryMaker{MakeTempDirResponse: MakeTempDirResponse{"/tmp/docker-plugin-cephfs_mnt", nil}}
	drv := Driver{
		MountPath: plugin.DefaultDockerRootDirectory,
		DB:        &bolt.DB{},
		mnt:       &mnt,
		dir:       &dir,
	}

	keyring, cleanup := prepareKeyring("[client.admin]\nkey = ABC123")
	defer must(cleanup)

	vol := &Volume{
		ClientName: "admin",
		Keyring:    keyring,
		Servers:    []string{"localhost"},
		RemotePath: "/stack/service",
	}

	got := drv.mountVolume(vol, "B1BF0ABD-85A6-4004-A10F-326D8F0C4C6F")
	if got != nil {
		t.Errorf("mountVolume() error = %s, expected nil", got)
		return
	}

	if !mnt.receivedCallWithArgs("Mount", "localhost:/", "/tmp/docker-plugin-cephfs_mnt", "ceph", "name=admin,secret=ABC123") {
		t.Errorf("mountVolume() did not call Mount with expected args")
		return
	}

	if !dir.receivedCallWithArgs("MakeDir", "/tmp/docker-plugin-cephfs_mnt/stack/service", os.FileMode(0755)) {
		t.Errorf("mountVolume() did not call MakeDir with expected args")
		return
	}

	if !mnt.receivedCallWithArgs("Unmount", "/tmp/docker-plugin-cephfs_mnt") {
		t.Errorf("mountVolume() did not call Unmount with expected args")
		return
	}

	if !mnt.receivedCallWithArgs("Mount", "localhost:/stack/service", "/var/lib/docker-volumes/B1BF0ABD-85A6-4004-A10F-326D8F0C4C6F", "ceph", "name=admin,secret=ABC123") {
		t.Errorf("mountVolume() did not call Mount with expected args")
	}
}

func TestDriver_mountVolume_withMountOpts(t *testing.T) {
	mnt := mockMounter{}
	dir := mockDirectoryMaker{MakeTempDirResponse: struct {
		string
		error
	}{"/tmp/docker-plugin-cephfs_mnt", nil}}
	drv := Driver{
		MountPath: plugin.DefaultDockerRootDirectory,
		DB:        &bolt.DB{},
		mnt:       &mnt,
		dir:       &dir,
	}

	keyring, cleanup := prepareKeyring("[client.admin]\nkey = ABC123")
	defer must(cleanup)

	vol := &Volume{
		ClientName: "admin",
		Keyring:    keyring,
		Servers:    []string{"localhost"},
		MountOpts:  "mds_namespace=staging",
	}

	got := drv.mountVolume(vol, "B1BF0ABD-85A6-4004-A10F-326D8F0C4C6F")
	if got != nil {
		t.Errorf("mountVolume() error = %s, expected nil", got)
		return
	}

	if !mnt.receivedCallWithArgs("Mount", "localhost:/", "/var/lib/docker-volumes/B1BF0ABD-85A6-4004-A10F-326D8F0C4C6F", "ceph", "name=admin,secret=ABC123,mds_namespace=staging") {
		t.Errorf("mountVolume() did not call Mount with expected args")
		return
	}
}

func TestDriver_mountVolume_tempDirFailure(t *testing.T) {
	want := errors.New("bad permissions")

	mnt := mockMounter{}
	dir := mockDirectoryMaker{MakeTempDirResponse: MakeTempDirResponse{"", want}}
	drv := Driver{DB: &bolt.DB{}, mnt: &mnt, dir: &dir}

	vol := &Volume{RemotePath: "Banana"}

	got := drv.mountVolume(vol, "B1BF0ABD-85A6-4004-A10F-326D8F0C4C6F")
	if got == nil || !strings.Contains(got.Error(), want.Error()) {
		t.Errorf("mountVolume() error = %s, expected %s", got, want)
		return
	}
}

func TestDriver_mountVolume_makeDirFailure(t *testing.T) {
	want := errors.New("file exists")

	mnt := mockMounter{}
	dir := mockDirectoryMaker{MakeDirResponse: want}
	drv := Driver{DB: &bolt.DB{}, mnt: &mnt, dir: &dir}

	vol := &Volume{}

	got := drv.mountVolume(vol, "B1BF0ABD-85A6-4004-A10F-326D8F0C4C6F")
	if got == nil || !strings.Contains(got.Error(), want.Error()) {
		t.Errorf("mountVolume() error = %s, expected %s", got, want)
		return
	}
}

func TestDriver_mountVolume_keyringDoesntExist(t *testing.T) {
	want := errors.New("keyring not found")

	mnt := mockMounter{}
	dir := mockDirectoryMaker{}
	drv := Driver{DB: &bolt.DB{}, mnt: &mnt, dir: &dir}

	vol := &Volume{Keyring: "/not-a-real-file"}

	got := drv.mountVolume(vol, "B1BF0ABD-85A6-4004-A10F-326D8F0C4C6F")
	if got == nil || !strings.Contains(got.Error(), want.Error()) {
		t.Errorf("mountVolume() error = %s, expected %s", got, want)
		return
	}
}

func TestDriver_mountVolume_keyringMismatch(t *testing.T) {
	want := errors.New("keyring did not contain client details")

	mnt := mockMounter{}
	dir := mockDirectoryMaker{}
	drv := Driver{DB: &bolt.DB{}, mnt: &mnt, dir: &dir}

	keyring, cleanup := prepareKeyring("[client.user]\nkey = ABC123")
	defer must(cleanup)

	vol := &Volume{ClientName: "docker", Keyring: keyring}

	got := drv.mountVolume(vol, "B1BF0ABD-85A6-4004-A10F-326D8F0C4C6F")
	if got == nil || !strings.Contains(got.Error(), want.Error()) {
		t.Errorf("mountVolume() error = %s, expected %s", got, want)
		return
	}
}

func TestDriver_mountVolume_keyringMissingKey(t *testing.T) {
	want := errors.New("keyring did not contain key")

	mnt := mockMounter{}
	dir := mockDirectoryMaker{}
	drv := Driver{DB: &bolt.DB{}, mnt: &mnt, dir: &dir}

	keyring, cleanup := prepareKeyring("[client.user]\nods = 'allow rw'")
	defer must(cleanup)

	vol := &Volume{ClientName: "user", Keyring: keyring}

	got := drv.mountVolume(vol, "B1BF0ABD-85A6-4004-A10F-326D8F0C4C6F")
	if got == nil || !strings.Contains(got.Error(), want.Error()) {
		t.Errorf("mountVolume() error = %s, expected %s", got, want)
		return
	}
}

func TestDriver_unmountVolume_singleConnection(t *testing.T) {
	mnt := mockMounter{}
	dir := mockDirectoryMaker{}
	drv := Driver{DB: &bolt.DB{}, mnt: &mnt, dir: &dir}

	vol := &Volume{Connections: 1, MountPoint: "/mnt/test"}

	got := drv.unmountVolume(vol)
	if got != nil {
		t.Errorf("unmountVolume() error = %s, expected nil", got)
		return
	}

	if vol.Connections != 0 {
		t.Errorf("unmountVolume() vol.Connections was not decremented")
		return
	}

	if vol.MountPoint != "" {
		t.Errorf("unmountVolume() vol.MountPoint was not set blank")
		return
	}

	if !mnt.receivedCallWithArgs("Unmount", "/mnt/test") {
		t.Errorf("unmountVolume() did not call Unmount with expected parameters")
	}
}

func TestDriver_unmountVolume_multipleConnections(t *testing.T) {
	mnt := mockMounter{}
	dir := mockDirectoryMaker{}
	drv := Driver{DB: &bolt.DB{}, mnt: &mnt, dir: &dir}

	vol := &Volume{Connections: 2, MountPoint: "/mnt/test"}

	got := drv.unmountVolume(vol)
	if got != nil {
		t.Errorf("unmountVolume() error = %s, expected nil", got)
		return
	}

	if vol.Connections != 1 {
		t.Errorf("unmountVolume() vol.Connections was not decremented")
		return
	}

	if vol.MountPoint != "/mnt/test" {
		t.Errorf("unmountVolume() vol.MountPoint was unexpectedly altered")
		return
	}

	if mnt.receivedCall("Unmount") {
		t.Errorf("unmountVolume() unexpectedly called Unmount")
	}
}

func TestDriver_unmountVolume_failedUnmount(t *testing.T) {
	want := errors.New("file in use")

	mnt := mockMounter{UnmountResponse: want}
	dir := mockDirectoryMaker{}
	drv := Driver{DB: &bolt.DB{}, mnt: &mnt, dir: &dir}

	vol := &Volume{Connections: 1, MountPoint: "/mnt/test"}

	got := drv.unmountVolume(vol)
	if got == nil || !strings.Contains(got.Error(), want.Error()) {
		t.Errorf("unmountVolume() error = %s, expected %s", got, want)
	}
}

type call struct {
	method string
	args   []interface{}
}

type mockMounter struct {
	MountResponse   error
	UnmountResponse error

	calls []call
}

func (m *mockMounter) Mount(source string, target string, fstype string, data string) error {
	m.calls = append(m.calls, call{"Mount", []interface{}{source, target, fstype, data}})
	return m.MountResponse
}

func (m *mockMounter) Unmount(target string) error {
	m.calls = append(m.calls, call{"Unmount", []interface{}{target}})
	return m.UnmountResponse
}

func (m mockMounter) receivedCall(method string) bool {
	for _, call := range m.calls {
		if method == call.method {
			return true
		}
	}

	return false
}

func (m mockMounter) receivedCallWithArgs(method string, args ...interface{}) bool {
	search := call{method: method, args: args}
	for _, call := range m.calls {
		if reflect.DeepEqual(search, call) {
			return true
		}
	}

	return false
}

type MakeTempDirResponse struct {
	string
	error
}

type mockDirectoryMaker struct {
	MakeTempDirResponse MakeTempDirResponse
	MakeDirResponse     error
	IsDirResponse       bool

	calls []call
}

func (m *mockDirectoryMaker) IsDir(dir string) bool {
	m.calls = append(m.calls, call{"IsDir", []interface{}{dir}})
	return m.IsDirResponse
}

func (m *mockDirectoryMaker) MakeDir(dir string, mode os.FileMode) error {
	m.calls = append(m.calls, call{"MakeDir", []interface{}{dir, mode}})
	return m.MakeDirResponse
}

func (m *mockDirectoryMaker) MakeTempDir() (string, error) {
	m.calls = append(m.calls, call{"MakeTempDir", []interface{}{}})
	return m.MakeTempDirResponse.string, m.MakeTempDirResponse.error
}

func (m mockDirectoryMaker) receivedCall(method string) bool {
	for _, call := range m.calls {
		if method == call.method {
			return true
		}
	}

	return false
}

func (m mockDirectoryMaker) receivedCallWithArgs(method string, args ...interface{}) bool {
	search := call{method: method, args: args}
	for _, call := range m.calls {
		if reflect.DeepEqual(search, call) {
			return true
		}
	}

	return false
}

func genMockDb() *bolt.DB {
	db, err := bolt.Open("/tmp/docker-plugin-cephfs_test.db", 0600, &bolt.Options{Timeout: time.Second * 2})
	if err != nil {
		log.Fatalf("Error creating test database: %s", err)
	}

	if err = db.Update(func(tx *bolt.Tx) error {
		_ = tx.DeleteBucket(bucket)
		_, err := tx.CreateBucket(bucket)
		if err != nil {
			return err
		}

		return nil
	}); err != nil {
		log.Fatalf("Error setting up test database: %s", err)
	}

	return db
}

func prepareMockData(db *bolt.DB, vols []Volume) error {
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucket)
		for id, v := range vols {
			d, err := serialize(v)
			if err != nil {
				return fmt.Errorf("could not serialize volume: %s", err)
			}
			err = b.Put([]byte("test."+strconv.Itoa(id+1)), d)
			if err != nil {
				return fmt.Errorf("could not insert record: %s", err)
			}
		}
		return nil
	})
}

func prepareKeyring(data string) (string, func() error) {
	f, err := ioutil.TempFile(os.TempDir(), "docker-plugin-cephfs_test.keyring")
	if err != nil {
		log.Fatalf("Could not prepare test keychain file: %s", err)
	}

	_, err = f.Write([]byte(data))
	if err != nil {
		log.Fatalf("Could not write test keychain data: %s", err)
	}

	err = f.Close()
	if err != nil {
		log.Fatalf("Could not close temporary keychain file: %s", err)
	}

	cleanup := func() error { return os.Remove(f.Name()) }
	return f.Name(), cleanup
}

func prepareTempDir() (string, func() error) {
	f, err := ioutil.TempDir(os.TempDir(), "docker-plugin-cephfs_test_mnt")
	if err != nil {
		log.Fatalf("Could not create temp dir: %s", err)
	}

	cleanup := func() error { return os.RemoveAll(f) }
	return f, cleanup
}

func must(fn func() error) {
	err := fn()

	if err == nil {
		return
	}

	panic(err)
}
