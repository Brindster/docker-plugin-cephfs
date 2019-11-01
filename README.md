# Docker Volume Plugin for CephFS
Inspired by other similar plugins, I wanted a docker volume plugin that didn't require
the secrets to be passed to the driver, but instead one that could read from keyring
files instead.

## Requirements
Since this plugin reads from your keyring files, it requires the folder `/etc/ceph` to
exist and be readable by the docker user. Keyring files should be stored in this folder
in order to be discoverable by the plugin.

Keyring files are expected to follow the naming pattern `ceph.client.admin.keyring` by 
default where `ceph` is the name of the cluster and `admin` is the name of the client.

## Ceph client creation
To create a client on your ceph cluster, you can run a command similar to the following:
```shell script
ceph auth get-or-create client.dockeruser mon 'allow r' osd 'allow rw' mds 'allow' \
> /etc/ceph/ceph.client.dockeruser.keyring
```
Then, copy over this keyring file to your docker hosts.

## Installation
```shell script
docker plugin install --alias cephfs gplusmedia/docker-plugin-cephfs \
CLUSTER_NAME=ceph \
CLIENT_NAME=admin \
SERVERS=ceph1,ceph2,ceph3
```

There are three settings that can be modified on the plugin during installation. These
settings act as default values, all of them are overridable when creating volumes.
* *CLUSTER_NAME* is the default name of the cluster. Defaults to _ceph_.
* *CLIENT_NAME* is the default name of the client. Defaults to _admin_.
* *SERVERS* is a comma-delimited list of ceph monitors to connect to. Defaults to _localhost_.

## Usage
Create a volume directly from the command line:
```shell script
docker volume create --driver cephfs test
docker run -it --rm -v test:/data busybox sh
```

Alternatively, use from a docker-compose file:
```yaml
version: '3'

services:
  app:
    image: nginx
    volumes:
      - test:/data

volumes:
    test:
        driver: cephfs
```

## Driver options
The following options are available:
* *client_name*:  The client name to connect using. Defaults to the value set on the plugin.
* *cluster_name*: The cluster name to connect to. Defaults to the value set on the plugin.
* *keyring*:      The path to the keyring file. Defaults to '/etc/ceph/%cluster_name%.client.%client_name%.keyring'.
* *mount_opts*:   Comma separated list of additional mount options
* *remote_path*:  The path on the remote server to mount to. Defaults to the volume name. 
* *servers*:      Comma separated list of servers to connect to. Defaults to the value set on the plugin.

```yaml
version: '3'

services:
  app:
    image: nginx
    volumes:
      - test:/data

volumes:
    test:
        driver: cephfs
        driver_opts:
            client_name: dockeruser
            keyring: /etc/ceph/dockeruser.keyring
            mount_opts: mds_namespace=example
            remote_path: /shared_data
            servers: 192.168.1.10,ceph-mon.internal.example.com:16789
```