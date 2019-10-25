package main

import (
	"github.com/boltdb/bolt"
	"github.com/docker/go-plugins-helpers/volume"
	"log"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"
)

func Test_cephfsDriver_Capabilities(t *testing.T) {
	type driver struct {
		configPath  string
		clientName  string
		clusterName string
		servers     []string
		DB          *bolt.DB
		RWMutex     sync.RWMutex
	}
	drv := driver{
		configPath:  defaultConfigPath,
		clientName:  defaultClientName,
		clusterName: defaultClusterName,
		servers:     []string{"localhost"},
		DB:          genMockDb(),
		RWMutex:     sync.RWMutex{},
	}
	defer must(drv.DB.Close)
	tests := []struct {
		name   string
		driver driver
		want   *volume.CapabilitiesResponse
	}{
		{"local", drv, &volume.CapabilitiesResponse{Capabilities: volume.Capability{Scope: "local"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := cephfsDriver{
				configPath:  tt.driver.configPath,
				clientName:  tt.driver.clientName,
				clusterName: tt.driver.clusterName,
				servers:     tt.driver.servers,
				DB:          tt.driver.DB,
				RWMutex:     tt.driver.RWMutex,
			}
			if got := d.Capabilities(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Capabilities() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_cephfsDriver_Create(t *testing.T) {
	type driver struct {
		configPath  string
		clientName  string
		clusterName string
		servers     []string
		DB          *bolt.DB
		RWMutex     sync.RWMutex
	}
	type args struct {
		req *volume.CreateRequest
	}
	drv := driver{
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
		driver  driver
		args    args
		wantErr bool
		want    *cephfsVolume
	}{
		{"valid", drv, args{&volume.CreateRequest{Name: "test.1"}}, false, &cephfsVolume{
			ClientName:  defaultClientName,
			Servers:     []string{"localhost"},
			ClusterName: defaultClusterName,
			ConfigPath:  defaultConfigPath,
		}},
		{"with client_name", drv, args{&volume.CreateRequest{Name: "test.2", Options: map[string]string{"client_name": "user"}}}, false, &cephfsVolume{
			ClientName:  "user",
			Servers:     []string{"localhost"},
			ClusterName: defaultClusterName,
			ConfigPath:  defaultConfigPath,
		}},
		{"with mount_opts", drv, args{&volume.CreateRequest{Name: "test.3", Options: map[string]string{"mount_opts": "name=user,secret=abc"}}}, false, &cephfsVolume{
			ClientName:  defaultClientName,
			MountOpts:   "name=user,secret=abc",
			Servers:     []string{"localhost"},
			ClusterName: defaultClusterName,
			ConfigPath:  defaultConfigPath,
		}},
		{"with remote_path", drv, args{&volume.CreateRequest{Name: "test.4", Options: map[string]string{"remote_path": "/data/mnt"}}}, false, &cephfsVolume{
			ClientName:  defaultClientName,
			RemotePath:  "/data/mnt",
			Servers:     []string{"localhost"},
			ClusterName: defaultClusterName,
			ConfigPath:  defaultConfigPath,
		}},
		{"with servers", drv, args{&volume.CreateRequest{Name: "test.5", Options: map[string]string{"servers": "monitor1:6798,monitor2:6798"}}}, false, &cephfsVolume{
			ClientName:  defaultClientName,
			Servers:     []string{"monitor1:6798", "monitor2:6798"},
			ClusterName: defaultClusterName,
			ConfigPath:  defaultConfigPath,
		}},
		{"duplicate name", drv, args{&volume.CreateRequest{Name: "test.1"}}, false, &cephfsVolume{
			ClientName:  defaultClientName,
			Servers:     []string{"localhost"},
			ClusterName: defaultClusterName,
			ConfigPath:  defaultConfigPath,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := cephfsDriver{
				configPath:  tt.driver.configPath,
				clientName:  tt.driver.clientName,
				clusterName: tt.driver.clusterName,
				servers:     tt.driver.servers,
				DB:          tt.driver.DB,
				RWMutex:     tt.driver.RWMutex,
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

func Test_cephfsDriver_Get(t *testing.T) {
	type driver struct {
		configPath  string
		clientName  string
		clusterName string
		servers     []string
		DB          *bolt.DB
		RWMutex     sync.RWMutex
	}
	type args struct {
		req *volume.GetRequest
	}
	drv := driver{
		configPath:  defaultConfigPath,
		clientName:  defaultClientName,
		clusterName: defaultClusterName,
		servers:     []string{"localhost"},
		DB:          genMockDb(),
		RWMutex:     sync.RWMutex{},
	}
	defer must(drv.DB.Close)
	vols := []cephfsVolume{
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
	_ = drv.DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(volumeBucket)
		for id, v := range vols {
			d, err := v.serialize()
			if err != nil {
				log.Fatalf("could not serialize volume: %s", err)
			}
			err = b.Put([]byte("test."+strconv.Itoa(id+1)), d)
			if err != nil {
				log.Fatalf("could not insert record: %s", err)
			}
		}
		return nil
	})
	tests := []struct {
		name    string
		driver  driver
		args    args
		want    *volume.GetResponse
		wantErr bool
	}{
		{"get existing", drv, args{&volume.GetRequest{Name: "test.1"}}, &volume.GetResponse{Volume: &volume.Volume{
			Name:       "test.1",
			Mountpoint: "",
			CreatedAt:  "2019-01-01T01:01:01Z",
			Status:     nil,
		}}, false},
		{"get mounted", drv, args{&volume.GetRequest{Name: "test.2"}}, &volume.GetResponse{Volume: &volume.Volume{
			Name:       "test.2",
			Mountpoint: "/var/www/app/data",
			CreatedAt:  "2019-02-02T02:02:02Z",
			Status:     nil,
		}}, false},
		{"non existing", drv, args{&volume.GetRequest{Name: "invalid"}}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := cephfsDriver{
				configPath:  tt.driver.configPath,
				clientName:  tt.driver.clientName,
				clusterName: tt.driver.clusterName,
				servers:     tt.driver.servers,
				DB:          tt.driver.DB,
				RWMutex:     tt.driver.RWMutex,
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

func Test_cephfsDriver_List(t *testing.T) {
	type fields struct {
		configPath  string
		clientName  string
		clusterName string
		servers     []string
		DB          *bolt.DB
		RWMutex     sync.RWMutex
	}
	tests := []struct {
		name    string
		fields  fields
		want    *volume.ListResponse
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := cephfsDriver{
				configPath:  tt.fields.configPath,
				clientName:  tt.fields.clientName,
				clusterName: tt.fields.clusterName,
				servers:     tt.fields.servers,
				DB:          tt.fields.DB,
				RWMutex:     tt.fields.RWMutex,
			}
			got, err := d.List()
			if (err != nil) != tt.wantErr {
				t.Errorf("List() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("List() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_cephfsDriver_Mount(t *testing.T) {
	type fields struct {
		configPath  string
		clientName  string
		clusterName string
		servers     []string
		DB          *bolt.DB
		RWMutex     sync.RWMutex
	}
	type args struct {
		req *volume.MountRequest
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *volume.MountResponse
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := cephfsDriver{
				configPath:  tt.fields.configPath,
				clientName:  tt.fields.clientName,
				clusterName: tt.fields.clusterName,
				servers:     tt.fields.servers,
				DB:          tt.fields.DB,
				RWMutex:     tt.fields.RWMutex,
			}
			got, err := d.Mount(tt.args.req)
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

func Test_cephfsDriver_Path(t *testing.T) {
	type fields struct {
		configPath  string
		clientName  string
		clusterName string
		servers     []string
		DB          *bolt.DB
		RWMutex     sync.RWMutex
	}
	type args struct {
		req *volume.PathRequest
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *volume.PathResponse
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := cephfsDriver{
				configPath:  tt.fields.configPath,
				clientName:  tt.fields.clientName,
				clusterName: tt.fields.clusterName,
				servers:     tt.fields.servers,
				DB:          tt.fields.DB,
				RWMutex:     tt.fields.RWMutex,
			}
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

func Test_cephfsDriver_Remove(t *testing.T) {
	type fields struct {
		configPath  string
		clientName  string
		clusterName string
		servers     []string
		DB          *bolt.DB
		RWMutex     sync.RWMutex
	}
	type args struct {
		req *volume.RemoveRequest
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := cephfsDriver{
				configPath:  tt.fields.configPath,
				clientName:  tt.fields.clientName,
				clusterName: tt.fields.clusterName,
				servers:     tt.fields.servers,
				DB:          tt.fields.DB,
				RWMutex:     tt.fields.RWMutex,
			}
			if err := d.Remove(tt.args.req); (err != nil) != tt.wantErr {
				t.Errorf("Remove() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_cephfsDriver_Unmount(t *testing.T) {
	type fields struct {
		configPath  string
		clientName  string
		clusterName string
		servers     []string
		DB          *bolt.DB
		RWMutex     sync.RWMutex
	}
	type args struct {
		req *volume.UnmountRequest
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := cephfsDriver{
				configPath:  tt.fields.configPath,
				clientName:  tt.fields.clientName,
				clusterName: tt.fields.clusterName,
				servers:     tt.fields.servers,
				DB:          tt.fields.DB,
				RWMutex:     tt.fields.RWMutex,
			}
			if err := d.Unmount(tt.args.req); (err != nil) != tt.wantErr {
				t.Errorf("Unmount() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_cephfsDriver_fetchVol(t *testing.T) {
	type fields struct {
		configPath  string
		clientName  string
		clusterName string
		servers     []string
		DB          *bolt.DB
		RWMutex     sync.RWMutex
	}
	type args struct {
		name string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *cephfsVolume
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := cephfsDriver{
				configPath:  tt.fields.configPath,
				clientName:  tt.fields.clientName,
				clusterName: tt.fields.clusterName,
				servers:     tt.fields.servers,
				DB:          tt.fields.DB,
				RWMutex:     tt.fields.RWMutex,
			}
			got, err := d.fetchVol(tt.args.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("fetchVol() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("fetchVol() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_cephfsDriver_saveVol(t *testing.T) {
	type fields struct {
		configPath  string
		clientName  string
		clusterName string
		servers     []string
		DB          *bolt.DB
		RWMutex     sync.RWMutex
	}
	type args struct {
		name string
		vol  cephfsVolume
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := cephfsDriver{
				configPath:  tt.fields.configPath,
				clientName:  tt.fields.clientName,
				clusterName: tt.fields.clusterName,
				servers:     tt.fields.servers,
				DB:          tt.fields.DB,
				RWMutex:     tt.fields.RWMutex,
			}
			if err := d.saveVol(tt.args.name, tt.args.vol); (err != nil) != tt.wantErr {
				t.Errorf("saveVol() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_cephfsVolume_connection(t *testing.T) {
	type fields struct {
		ClientName  string
		MountPoint  string
		CreatedAt   string
		Status      map[string]interface{}
		MountOpts   string
		RemotePath  string
		Servers     []string
		ClusterName string
		ConfigPath  string
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := cephfsVolume{
				ClientName:  tt.fields.ClientName,
				MountPoint:  tt.fields.MountPoint,
				CreatedAt:   tt.fields.CreatedAt,
				Status:      tt.fields.Status,
				MountOpts:   tt.fields.MountOpts,
				RemotePath:  tt.fields.RemotePath,
				Servers:     tt.fields.Servers,
				ClusterName: tt.fields.ClusterName,
				ConfigPath:  tt.fields.ConfigPath,
			}
			if got := v.connection(); got != tt.want {
				t.Errorf("connection() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_cephfsVolume_mount(t *testing.T) {
	type fields struct {
		ClientName  string
		MountPoint  string
		CreatedAt   string
		Status      map[string]interface{}
		MountOpts   string
		RemotePath  string
		Servers     []string
		ClusterName string
		ConfigPath  string
	}
	type args struct {
		mnt string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &cephfsVolume{
				ClientName:  tt.fields.ClientName,
				MountPoint:  tt.fields.MountPoint,
				CreatedAt:   tt.fields.CreatedAt,
				Status:      tt.fields.Status,
				MountOpts:   tt.fields.MountOpts,
				RemotePath:  tt.fields.RemotePath,
				Servers:     tt.fields.Servers,
				ClusterName: tt.fields.ClusterName,
				ConfigPath:  tt.fields.ConfigPath,
			}
			if err := v.mount(tt.args.mnt); (err != nil) != tt.wantErr {
				t.Errorf("mount() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_cephfsVolume_secret(t *testing.T) {
	type fields struct {
		ClientName  string
		MountPoint  string
		CreatedAt   string
		Status      map[string]interface{}
		MountOpts   string
		RemotePath  string
		Servers     []string
		ClusterName string
		ConfigPath  string
	}
	tests := []struct {
		name    string
		fields  fields
		want    string
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := cephfsVolume{
				ClientName:  tt.fields.ClientName,
				MountPoint:  tt.fields.MountPoint,
				CreatedAt:   tt.fields.CreatedAt,
				Status:      tt.fields.Status,
				MountOpts:   tt.fields.MountOpts,
				RemotePath:  tt.fields.RemotePath,
				Servers:     tt.fields.Servers,
				ClusterName: tt.fields.ClusterName,
				ConfigPath:  tt.fields.ConfigPath,
			}
			got, err := v.secret()
			if (err != nil) != tt.wantErr {
				t.Errorf("secret() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("secret() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_cephfsVolume_serialize(t *testing.T) {
	type fields struct {
		ClientName  string
		MountPoint  string
		CreatedAt   string
		Status      map[string]interface{}
		MountOpts   string
		RemotePath  string
		Servers     []string
		ClusterName string
		ConfigPath  string
	}
	tests := []struct {
		name    string
		fields  fields
		want    []byte
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := cephfsVolume{
				ClientName:  tt.fields.ClientName,
				MountPoint:  tt.fields.MountPoint,
				CreatedAt:   tt.fields.CreatedAt,
				Status:      tt.fields.Status,
				MountOpts:   tt.fields.MountOpts,
				RemotePath:  tt.fields.RemotePath,
				Servers:     tt.fields.Servers,
				ClusterName: tt.fields.ClusterName,
				ConfigPath:  tt.fields.ConfigPath,
			}
			got, err := v.serialize()
			if (err != nil) != tt.wantErr {
				t.Errorf("serialize() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("serialize() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_cephfsVolume_unmount(t *testing.T) {
	type fields struct {
		ClientName  string
		MountPoint  string
		CreatedAt   string
		Status      map[string]interface{}
		MountOpts   string
		RemotePath  string
		Servers     []string
		ClusterName string
		ConfigPath  string
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := cephfsVolume{
				ClientName:  tt.fields.ClientName,
				MountPoint:  tt.fields.MountPoint,
				CreatedAt:   tt.fields.CreatedAt,
				Status:      tt.fields.Status,
				MountOpts:   tt.fields.MountOpts,
				RemotePath:  tt.fields.RemotePath,
				Servers:     tt.fields.Servers,
				ClusterName: tt.fields.ClusterName,
				ConfigPath:  tt.fields.ConfigPath,
			}
			if err := v.unmount(); (err != nil) != tt.wantErr {
				t.Errorf("unmount() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_envOrDefault(t *testing.T) {
	type args struct {
		param string
		def   string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := envOrDefault(tt.args.param, tt.args.def); got != tt.want {
				t.Errorf("envOrDefault() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_newCephFsDriver(t *testing.T) {
	tests := []struct {
		name string
		want cephfsDriver
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := newCephFsDriver(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("newCephFsDriver() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_unserialize(t *testing.T) {
	type args struct {
		in []byte
	}
	tests := []struct {
		name    string
		args    args
		want    *cephfsVolume
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := unserialize(tt.args.in)
			if (err != nil) != tt.wantErr {
				t.Errorf("unserialize() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("unserialize() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func volCompare(want, got *cephfsVolume) bool {
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

func must(fn func() error) {
	err := fn()

	if err == nil {
		return
	}

	panic(err)
}
