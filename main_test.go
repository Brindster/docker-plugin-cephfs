package main

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
		want *driver
	}{
		{"default", []string{}, &driver{
			configPath:  defaultConfigPath,
			clientName:  defaultClientName,
			clusterName: defaultClusterName,
			servers:     []string{"localhost"},
			DB:          db,
			RWMutex:     sync.RWMutex{},
		}},
		{"server set", []string{"SERVERS=ceph1"}, &driver{
			configPath:  defaultConfigPath,
			clientName:  defaultClientName,
			clusterName: defaultClusterName,
			servers:     []string{"ceph1"},
			DB:          db,
			RWMutex:     sync.RWMutex{},
		}},
		{"servers set", []string{"SERVERS=ceph1,ceph2,ceph3"}, &driver{
			configPath:  defaultConfigPath,
			clientName:  defaultClientName,
			clusterName: defaultClusterName,
			servers:     []string{"ceph1", "ceph2", "ceph3"},
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

			got := newDriver(db)
			if got.clusterName != tt.want.clusterName {
				t.Errorf("newDriver().clusterName = %v, want %v", got.clusterName, tt.want.clusterName)
			}

			if got.clientName != tt.want.clientName {
				t.Errorf("newDriver().clientName = %v, want %v", got.clientName, tt.want.clientName)
			}

			if got.configPath != tt.want.configPath {
				t.Errorf("newDriver().configPath = %v, want %v", got.configPath, tt.want.configPath)
			}

			if !reflect.DeepEqual(got.servers, tt.want.servers) {
				t.Errorf("newDriver().servers = %v, want %v", got.servers, tt.want.servers)
			}

			if !reflect.DeepEqual(got.DB, tt.want.DB) {
				t.Errorf("newDriver().DB = %v, want %v", got.DB, tt.want.DB)
			}
		}()
	}
}

func TestDriver_Capabilities(t *testing.T) {
	d := driver{}
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
		want    *volume
	}{
		{"valid", drv, args{&plugin.CreateRequest{Name: "test.1"}}, false, &volume{
			ClientName:  defaultClientName,
			Servers:     []string{"localhost"},
			ClusterName: defaultClusterName,
			ConfigPath:  defaultConfigPath,
			Keyring:     "/etc/ceph/ceph.client.admin.keyring",
		}},
		{"with client_name", drv, args{&plugin.CreateRequest{Name: "test.2", Options: map[string]string{"client_name": "user"}}}, false, &volume{
			ClientName:  "user",
			Servers:     []string{"localhost"},
			ClusterName: defaultClusterName,
			ConfigPath:  defaultConfigPath,
			Keyring:     "/etc/ceph/ceph.client.user.keyring",
		}},
		{"with mount_opts", drv, args{&plugin.CreateRequest{Name: "test.3", Options: map[string]string{"mount_opts": "name=user,secret=abc"}}}, false, &volume{
			ClientName:  defaultClientName,
			MountOpts:   "name=user,secret=abc",
			Servers:     []string{"localhost"},
			ClusterName: defaultClusterName,
			ConfigPath:  defaultConfigPath,
			Keyring:     "/etc/ceph/ceph.client.admin.keyring",
		}},
		{"with remote_path", drv, args{&plugin.CreateRequest{Name: "test.4", Options: map[string]string{"remote_path": "/data/mnt"}}}, false, &volume{
			ClientName:  defaultClientName,
			RemotePath:  "/data/mnt",
			Servers:     []string{"localhost"},
			ClusterName: defaultClusterName,
			ConfigPath:  defaultConfigPath,
			Keyring:     "/etc/ceph/ceph.client.admin.keyring",
		}},
		{"with servers", drv, args{&plugin.CreateRequest{Name: "test.5", Options: map[string]string{"servers": "monitor1:6798,monitor2:6798"}}}, false, &volume{
			ClientName:  defaultClientName,
			Servers:     []string{"monitor1:6798", "monitor2:6798"},
			ClusterName: defaultClusterName,
			ConfigPath:  defaultConfigPath,
			Keyring:     "/etc/ceph/ceph.client.admin.keyring",
		}},
		{"duplicate name", drv, args{&plugin.CreateRequest{Name: "test.1"}}, false, &volume{
			ClientName:  defaultClientName,
			Servers:     []string{"localhost"},
			ClusterName: defaultClusterName,
			ConfigPath:  defaultConfigPath,
			Keyring:     "/etc/ceph/ceph.client.admin.keyring",
		}},
		{"with keyring", drv, args{&plugin.CreateRequest{Name: "test.6", Options: map[string]string{"keyring": "/etc/ceph/test.keyring"}}}, false, &volume{
			ClientName:  defaultClientName,
			Servers:     []string{"localhost"},
			ClusterName: defaultClusterName,
			ConfigPath:  defaultConfigPath,
			Keyring:     "/etc/ceph/test.keyring",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := driver{
				configPath:  tt.fields.configPath,
				clientName:  tt.fields.clientName,
				clusterName: tt.fields.clusterName,
				servers:     tt.fields.servers,
				mnt:         mockMounter{},
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
				if !volCompare(got, tt.want) {
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
	vols := []volume{
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
			d := driver{
				configPath:  tt.fields.configPath,
				clientName:  tt.fields.clientName,
				clusterName: tt.fields.clusterName,
				servers:     tt.fields.servers,
				mnt:         mockMounter{},
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

	drv := driver{
		configPath:  defaultConfigPath,
		clientName:  defaultClientName,
		clusterName: defaultClusterName,
		servers:     []string{"localhost"},
		mnt:         mockMounter{},
		DB:          db,
		RWMutex:     sync.RWMutex{},
	}

	vols := []volume{
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

	drv := driver{
		configPath:  defaultConfigPath,
		clientName:  defaultClientName,
		clusterName: defaultClusterName,
		servers:     []string{"localhost"},
		mnt:         mockMounter{},
		DB:          db,
		RWMutex:     sync.RWMutex{},
	}

	f, _ := ioutil.TempFile("", "docker-plugin-cephfs_test.keyring")
	_, _ = f.Write([]byte("[client.admin]\nkey = ABC123"))
	_ = f.Close()

	vol := volume{
		MountPoint: "",
		CreatedAt:  "2019-01-01T01:01:01Z",
		Status:     nil,
		ClientName: "admin",
		Keyring:    f.Name(),
	}
	must(func() error { return prepareMockData(drv.DB, []volume{vol}) })

	mountDir = path.Join(os.TempDir(), "docker-plugin-cephfs_test_mnt")
	defer func() { mountDir = plugin.DefaultDockerRootDirectory }()

	got, err := drv.Mount(&plugin.MountRequest{Name: "test.1", ID: "624F80C6-F050-42BF-8B02-387AA892782F"})
	if err != nil {
		t.Errorf("Mount() error = %s, wanted nil", err)
		return
	}

	want := &plugin.MountResponse{Mountpoint: path.Join(mountDir, "624F80C6-F050-42BF-8B02-387AA892782F")}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Mount() got = %v, want %v", got, want)
	}
}

func TestDriver_Path(t *testing.T) {
	db := genMockDb()
	defer must(db.Close)
	vols := []volume{
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
			d := driver{mnt: mockMounter{}, DB: db}
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

	vols := []volume{
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
			d := driver{mnt: mockMounter{}, DB: db}
			err := d.Remove(tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("Remove() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil {
				_ = db.View(func(tx *bolt.Tx) error {
					b := tx.Bucket(volumeBucket)
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

	vols := []volume{
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
			d := driver{mnt: mockMounter{UnmountResponse: tt.umntResp}, DB: db}
			err := d.Unmount(tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("Unmount() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil && !tt.wantErr {
				_ = db.View(func(tx *bolt.Tx) error {
					b := tx.Bucket(volumeBucket)
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

//func TestDriver_fetchVol(t *testing.T) {
//	type fields struct {
//		configPath  string
//		clientName  string
//		clusterName string
//		servers     []string
//		DB          *bolt.DB
//		RWMutex     sync.RWMutex
//	}
//	type args struct {
//		name string
//	}
//	tests := []struct {
//		name    string
//		fields  fields
//		args    args
//		want    *volume
//		wantErr bool
//	}{
//		// TODO: Add test cases.
//	}
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//			d := driver{
//				configPath:  tt.fields.configPath,
//				clientName:  tt.fields.clientName,
//				clusterName: tt.fields.clusterName,
//				servers:     tt.fields.servers,
//				DB:          tt.fields.DB,
//				RWMutex:     tt.fields.RWMutex,
//			}
//			got, err := d.fetchVol(tt.args.name)
//			if (err != nil) != tt.wantErr {
//				t.Errorf("fetchVol() error = %v, wantErr %v", err, tt.wantErr)
//				return
//			}
//			if !reflect.DeepEqual(got, tt.want) {
//				t.Errorf("fetchVol() got = %v, want %v", got, tt.want)
//			}
//		})
//	}
//}
//
//func TestDriver_saveVol(t *testing.T) {
//	type fields struct {
//		configPath  string
//		clientName  string
//		clusterName string
//		servers     []string
//		DB          *bolt.DB
//		RWMutex     sync.RWMutex
//	}
//	type args struct {
//		name string
//		vol  volume
//	}
//	tests := []struct {
//		name    string
//		fields  fields
//		args    args
//		wantErr bool
//	}{
//		// TODO: Add test cases.
//	}
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//			d := driver{
//				configPath:  tt.fields.configPath,
//				clientName:  tt.fields.clientName,
//				clusterName: tt.fields.clusterName,
//				servers:     tt.fields.servers,
//				DB:          tt.fields.DB,
//				RWMutex:     tt.fields.RWMutex,
//			}
//			if err := d.saveVol(tt.args.name, tt.args.vol); (err != nil) != tt.wantErr {
//				t.Errorf("saveVol() error = %v, wantErr %v", err, tt.wantErr)
//			}
//		})
//	}
//}
//
//func TestCephfsVolume_mount(t *testing.T) {
//	type fields struct {
//		ClientName  string
//		MountPoint  string
//		CreatedAt   string
//		Status      map[string]interface{}
//		MountOpts   string
//		RemotePath  string
//		Servers     []string
//		ClusterName string
//		ConfigPath  string
//	}
//	type args struct {
//		mnt string
//	}
//	tests := []struct {
//		name    string
//		fields  fields
//		args    args
//		wantErr bool
//	}{
//		// TODO: Add test cases.
//	}
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//			v := &volume{
//				ClientName:  tt.fields.ClientName,
//				MountPoint:  tt.fields.MountPoint,
//				CreatedAt:   tt.fields.CreatedAt,
//				Status:      tt.fields.Status,
//				MountOpts:   tt.fields.MountOpts,
//				RemotePath:  tt.fields.RemotePath,
//				Servers:     tt.fields.Servers,
//				ClusterName: tt.fields.ClusterName,
//				ConfigPath:  tt.fields.ConfigPath,
//			}
//			if err := v.mount(tt.args.mnt); (err != nil) != tt.wantErr {
//				t.Errorf("mount() error = %v, wantErr %v", err, tt.wantErr)
//			}
//		})
//	}
//}
//
//func TestCephfsVolume_secret(t *testing.T) {
//	type fields struct {
//		ClientName  string
//		MountPoint  string
//		CreatedAt   string
//		Status      map[string]interface{}
//		MountOpts   string
//		RemotePath  string
//		Servers     []string
//		ClusterName string
//		ConfigPath  string
//	}
//	tests := []struct {
//		name    string
//		fields  fields
//		want    string
//		wantErr bool
//	}{
//		// TODO: Add test cases.
//	}
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//			v := volume{
//				ClientName:  tt.fields.ClientName,
//				MountPoint:  tt.fields.MountPoint,
//				CreatedAt:   tt.fields.CreatedAt,
//				Status:      tt.fields.Status,
//				MountOpts:   tt.fields.MountOpts,
//				RemotePath:  tt.fields.RemotePath,
//				Servers:     tt.fields.Servers,
//				ClusterName: tt.fields.ClusterName,
//				ConfigPath:  tt.fields.ConfigPath,
//			}
//			got, err := v.secret()
//			if (err != nil) != tt.wantErr {
//				t.Errorf("secret() error = %v, wantErr %v", err, tt.wantErr)
//				return
//			}
//			if got != tt.want {
//				t.Errorf("secret() got = %v, want %v", got, tt.want)
//			}
//		})
//	}
//}
//
//func TestCephfsVolume_serialize(t *testing.T) {
//	type fields struct {
//		ClientName  string
//		MountPoint  string
//		CreatedAt   string
//		Status      map[string]interface{}
//		MountOpts   string
//		RemotePath  string
//		Servers     []string
//		ClusterName string
//		ConfigPath  string
//	}
//	tests := []struct {
//		name    string
//		fields  fields
//		want    []byte
//		wantErr bool
//	}{
//		// TODO: Add test cases.
//	}
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//			v := volume{
//				ClientName:  tt.fields.ClientName,
//				MountPoint:  tt.fields.MountPoint,
//				CreatedAt:   tt.fields.CreatedAt,
//				Status:      tt.fields.Status,
//				MountOpts:   tt.fields.MountOpts,
//				RemotePath:  tt.fields.RemotePath,
//				Servers:     tt.fields.Servers,
//				ClusterName: tt.fields.ClusterName,
//				ConfigPath:  tt.fields.ConfigPath,
//			}
//			got, err := v.serialize()
//			if (err != nil) != tt.wantErr {
//				t.Errorf("serialize() error = %v, wantErr %v", err, tt.wantErr)
//				return
//			}
//			if !reflect.DeepEqual(got, tt.want) {
//				t.Errorf("serialize() got = %v, want %v", got, tt.want)
//			}
//		})
//	}
//}
//
//func TestCephfsVolume_unmount(t *testing.T) {
//	type fields struct {
//		ClientName  string
//		MountPoint  string
//		CreatedAt   string
//		Status      map[string]interface{}
//		MountOpts   string
//		RemotePath  string
//		Servers     []string
//		ClusterName string
//		ConfigPath  string
//	}
//	tests := []struct {
//		name    string
//		fields  fields
//		wantErr bool
//	}{
//		// TODO: Add test cases.
//	}
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//			v := volume{
//				ClientName:  tt.fields.ClientName,
//				MountPoint:  tt.fields.MountPoint,
//				CreatedAt:   tt.fields.CreatedAt,
//				Status:      tt.fields.Status,
//				MountOpts:   tt.fields.MountOpts,
//				RemotePath:  tt.fields.RemotePath,
//				Servers:     tt.fields.Servers,
//				ClusterName: tt.fields.ClusterName,
//				ConfigPath:  tt.fields.ConfigPath,
//			}
//			if err := v.unmount(); (err != nil) != tt.wantErr {
//				t.Errorf("unmount() error = %v, wantErr %v", err, tt.wantErr)
//			}
//		})
//	}
//}
//
//func TestEnvOrDefault(t *testing.T) {
//	type args struct {
//		param string
//		def   string
//	}
//	tests := []struct {
//		name string
//		args args
//		want string
//	}{
//		// TODO: Add test cases.
//	}
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//			if got := envOrDefault(tt.args.param, tt.args.def); got != tt.want {
//				t.Errorf("envOrDefault() = %v, want %v", got, tt.want)
//			}
//		})
//	}
//}
//
//func TestNewCephFsDriver(t *testing.T) {
//	tests := []struct {
//		name string
//		want driver
//	}{
//		// TODO: Add test cases.
//	}
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//			if got := newDriver(); !reflect.DeepEqual(got, tt.want) {
//				t.Errorf("newDriver() = %v, want %v", got, tt.want)
//			}
//		})
//	}
//}
//
//func TestUnserialize(t *testing.T) {
//	type args struct {
//		in []byte
//	}
//	tests := []struct {
//		name    string
//		args    args
//		want    *volume
//		wantErr bool
//	}{
//		// TODO: Add test cases.
//	}
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//			got, err := unserialize(tt.args.in)
//			if (err != nil) != tt.wantErr {
//				t.Errorf("unserialize() error = %v, wantErr %v", err, tt.wantErr)
//				return
//			}
//			if !reflect.DeepEqual(got, tt.want) {
//				t.Errorf("unserialize() got = %v, want %v", got, tt.want)
//			}
//		})
//	}
//}

type mockMounter struct {
	MountResponse   error
	UnmountResponse error
}

func (m mockMounter) Mount(source string, target string, fstype string, data string) error {
	return m.MountResponse
}

func (m mockMounter) Unmount(target string) error {
	return m.UnmountResponse
}

func volCompare(want, got *volume) bool {
	if (want != nil) != (got != nil) {
		return false
	}

	if want == nil {
		return true
	}

	if !reflect.DeepEqual(want.ClusterName, got.ClusterName) {
		return false
	}

	if !reflect.DeepEqual(want.MountOpts, got.MountOpts) {
		return false
	}

	if !reflect.DeepEqual(want.Servers, got.Servers) {
		return false
	}

	if !reflect.DeepEqual(want.RemotePath, got.RemotePath) {
		return false
	}

	if !reflect.DeepEqual(want.MountPoint, got.MountPoint) {
		return false
	}

	if !reflect.DeepEqual(want.ClientName, got.ClientName) {
		return false
	}

	if !reflect.DeepEqual(want.Status, got.Status) {
		return false
	}

	if !reflect.DeepEqual(want.ConfigPath, got.ConfigPath) {
		return false
	}

	return true
}

func genMockDb() *bolt.DB {
	db, err := bolt.Open("/tmp/docker-plugin-cephfs_test.db", 0600, &bolt.Options{Timeout: time.Second * 2})
	if err != nil {
		log.Fatalf("Error creating test database: %s", err)
	}

	if err = db.Update(func(tx *bolt.Tx) error {
		_ = tx.DeleteBucket(volumeBucket)
		_, err := tx.CreateBucket(volumeBucket)
		if err != nil {
			return err
		}

		return nil
	}); err != nil {
		log.Fatalf("Error setting up test database: %s", err)
	}

	return db
}

func prepareMockData(db *bolt.DB, vols []volume) error {
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(volumeBucket)
		for id, v := range vols {
			d, err := v.serialize()
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

func must(fn func() error) {
	err := fn()

	if err == nil {
		return
	}

	panic(err)
}
