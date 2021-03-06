package volume

import (
	"reflect"
	"testing"
)

func TestSerialize(t *testing.T) {
	type args struct {
		v Volume
	}
	tests := []struct {
		name    string
		args    args
		want    []byte
		wantErr bool
	}{
		{"empty volume", args{Volume{}}, []byte{255, 177, 255, 129, 3, 1, 1, 6, 86, 111, 108, 117, 109, 101, 1, 255, 130, 0, 1, 11, 1, 10, 67, 108, 105, 101, 110, 116, 78, 97, 109, 101, 1, 12, 0, 1, 10, 77, 111, 117, 110, 116, 80, 111, 105, 110, 116, 1, 12, 0, 1, 9, 67, 114, 101, 97, 116, 101, 100, 65, 116, 1, 12, 0, 1, 6, 83, 116, 97, 116, 117, 115, 1, 255, 132, 0, 1, 9, 77, 111, 117, 110, 116, 79, 112, 116, 115, 1, 12, 0, 1, 10, 82, 101, 109, 111, 116, 101, 80, 97, 116, 104, 1, 12, 0, 1, 7, 83, 101, 114, 118, 101, 114, 115, 1, 255, 134, 0, 1, 11, 67, 108, 117, 115, 116, 101, 114, 78, 97, 109, 101, 1, 12, 0, 1, 10, 67, 111, 110, 102, 105, 103, 80, 97, 116, 104, 1, 12, 0, 1, 7, 75, 101, 121, 114, 105, 110, 103, 1, 12, 0, 1, 11, 67, 111, 110, 110, 101, 99, 116, 105, 111, 110, 115, 1, 4, 0, 0, 0, 39, 255, 131, 4, 1, 1, 23, 109, 97, 112, 91, 115, 116, 114, 105, 110, 103, 93, 105, 110, 116, 101, 114, 102, 97, 99, 101, 32, 123, 125, 1, 255, 132, 0, 1, 12, 1, 16, 0, 0, 22, 255, 133, 2, 1, 1, 8, 91, 93, 115, 116, 114, 105, 110, 103, 1, 255, 134, 0, 1, 12, 0, 0, 3, 255, 130, 0}, false},
		{"volume with data", args{Volume{ClientName: "user-1024", MountPoint: "/mnt", Keyring: "/etc/ceph/ceph.client.user-1024.keyring"}}, []byte{255, 177, 255, 129, 3, 1, 1, 6, 86, 111, 108, 117, 109, 101, 1, 255, 130, 0, 1, 11, 1, 10, 67, 108, 105, 101, 110, 116, 78, 97, 109, 101, 1, 12, 0, 1, 10, 77, 111, 117, 110, 116, 80, 111, 105, 110, 116, 1, 12, 0, 1, 9, 67, 114, 101, 97, 116, 101, 100, 65, 116, 1, 12, 0, 1, 6, 83, 116, 97, 116, 117, 115, 1, 255, 132, 0, 1, 9, 77, 111, 117, 110, 116, 79, 112, 116, 115, 1, 12, 0, 1, 10, 82, 101, 109, 111, 116, 101, 80, 97, 116, 104, 1, 12, 0, 1, 7, 83, 101, 114, 118, 101, 114, 115, 1, 255, 134, 0, 1, 11, 67, 108, 117, 115, 116, 101, 114, 78, 97, 109, 101, 1, 12, 0, 1, 10, 67, 111, 110, 102, 105, 103, 80, 97, 116, 104, 1, 12, 0, 1, 7, 75, 101, 121, 114, 105, 110, 103, 1, 12, 0, 1, 11, 67, 111, 110, 110, 101, 99, 116, 105, 111, 110, 115, 1, 4, 0, 0, 0, 39, 255, 131, 4, 1, 1, 23, 109, 97, 112, 91, 115, 116, 114, 105, 110, 103, 93, 105, 110, 116, 101, 114, 102, 97, 99, 101, 32, 123, 125, 1, 255, 132, 0, 1, 12, 1, 16, 0, 0, 22, 255, 133, 2, 1, 1, 8, 91, 93, 115, 116, 114, 105, 110, 103, 1, 255, 134, 0, 1, 12, 0, 0, 61, 255, 130, 1, 9, 117, 115, 101, 114, 45, 49, 48, 50, 52, 1, 4, 47, 109, 110, 116, 8, 39, 47, 101, 116, 99, 47, 99, 101, 112, 104, 47, 99, 101, 112, 104, 46, 99, 108, 105, 101, 110, 116, 46, 117, 115, 101, 114, 45, 49, 48, 50, 52, 46, 107, 101, 121, 114, 105, 110, 103, 0}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := serialize(tt.args.v)
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

func TestUnserialize(t *testing.T) {
	type args struct {
		in []byte
	}
	tests := []struct {
		name    string
		args    args
		want    *Volume
		wantErr bool
	}{
		{"empty volume", args{[]byte{255, 177, 255, 129, 3, 1, 1, 6, 86, 111, 108, 117, 109, 101, 1, 255, 130, 0, 1, 11, 1, 10, 67, 108, 105, 101, 110, 116, 78, 97, 109, 101, 1, 12, 0, 1, 10, 77, 111, 117, 110, 116, 80, 111, 105, 110, 116, 1, 12, 0, 1, 9, 67, 114, 101, 97, 116, 101, 100, 65, 116, 1, 12, 0, 1, 6, 83, 116, 97, 116, 117, 115, 1, 255, 132, 0, 1, 9, 77, 111, 117, 110, 116, 79, 112, 116, 115, 1, 12, 0, 1, 10, 82, 101, 109, 111, 116, 101, 80, 97, 116, 104, 1, 12, 0, 1, 7, 83, 101, 114, 118, 101, 114, 115, 1, 255, 134, 0, 1, 11, 67, 108, 117, 115, 116, 101, 114, 78, 97, 109, 101, 1, 12, 0, 1, 10, 67, 111, 110, 102, 105, 103, 80, 97, 116, 104, 1, 12, 0, 1, 7, 75, 101, 121, 114, 105, 110, 103, 1, 12, 0, 1, 11, 67, 111, 110, 110, 101, 99, 116, 105, 111, 110, 115, 1, 4, 0, 0, 0, 39, 255, 131, 4, 1, 1, 23, 109, 97, 112, 91, 115, 116, 114, 105, 110, 103, 93, 105, 110, 116, 101, 114, 102, 97, 99, 101, 32, 123, 125, 1, 255, 132, 0, 1, 12, 1, 16, 0, 0, 22, 255, 133, 2, 1, 1, 8, 91, 93, 115, 116, 114, 105, 110, 103, 1, 255, 134, 0, 1, 12, 0, 0, 3, 255, 130, 0}}, &Volume{}, false},
		{"volume with data", args{[]byte{255, 177, 255, 129, 3, 1, 1, 6, 118, 111, 108, 117, 109, 101, 1, 255, 130, 0, 1, 11, 1, 10, 67, 108, 105, 101, 110, 116, 78, 97, 109, 101, 1, 12, 0, 1, 10, 77, 111, 117, 110, 116, 80, 111, 105, 110, 116, 1, 12, 0, 1, 9, 67, 114, 101, 97, 116, 101, 100, 65, 116, 1, 12, 0, 1, 6, 83, 116, 97, 116, 117, 115, 1, 255, 132, 0, 1, 9, 77, 111, 117, 110, 116, 79, 112, 116, 115, 1, 12, 0, 1, 10, 82, 101, 109, 111, 116, 101, 80, 97, 116, 104, 1, 12, 0, 1, 7, 83, 101, 114, 118, 101, 114, 115, 1, 255, 134, 0, 1, 11, 67, 108, 117, 115, 116, 101, 114, 78, 97, 109, 101, 1, 12, 0, 1, 10, 67, 111, 110, 102, 105, 103, 80, 97, 116, 104, 1, 12, 0, 1, 7, 75, 101, 121, 114, 105, 110, 103, 1, 12, 0, 1, 11, 67, 111, 110, 110, 101, 99, 116, 105, 111, 110, 115, 1, 4, 0, 0, 0, 39, 255, 131, 4, 1, 1, 23, 109, 97, 112, 91, 115, 116, 114, 105, 110, 103, 93, 105, 110, 116, 101, 114, 102, 97, 99, 101, 32, 123, 125, 1, 255, 132, 0, 1, 12, 1, 16, 0, 0, 22, 255, 133, 2, 1, 1, 8, 91, 93, 115, 116, 114, 105, 110, 103, 1, 255, 134, 0, 1, 12, 0, 0, 61, 255, 130, 1, 9, 117, 115, 101, 114, 45, 49, 48, 50, 52, 1, 4, 47, 109, 110, 116, 8, 39, 47, 101, 116, 99, 47, 99, 101, 112, 104, 47, 99, 101, 112, 104, 46, 99, 108, 105, 101, 110, 116, 46, 117, 115, 101, 114, 45, 49, 48, 50, 52, 46, 107, 101, 121, 114, 105, 110, 103, 0}}, &Volume{ClientName: "user-1024", MountPoint: "/mnt", Keyring: "/etc/ceph/ceph.client.user-1024.keyring"}, false},
		{"invalid data", args{[]byte{}}, nil, true},
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
