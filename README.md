# 阿里云盘 volume provision controller

Dynamic volume provisioning allows storage volumes to be created on-demand.

# Deployment

## Building
Building the project will only work if the project is in your `GOPATH`. Download the project into your `GOPATH` directory by using `go get` or cloning it manually.

```
$ go get github.com/AliyunContainerService/alicloud-storage-provisioner
```

Now build the project and the Docker image by checking out the latest release and running `make container` in the project directory.

```
# cd $GOPATH/src/github.com/AliyunContainerService/alicloud-storage-provisioner/build
# sh build.sh
```

## Deploy Provisioner

Provisioner is deployed in alicloud k8s cluster by default.

```console

# kubectl create -f deploy/deployment.yaml
deployment "disk-provisioner" created
```

## Usage

```yaml
kind: PersistentVolumeClaim
apiVersion: v1
metadata:
  name: disk
spec:
  accessModes:
    - ReadWriteOnce
  storageClassName: alicloud-disk-common
  resources:
    requests:
      storage: 20Gi
```

`storageClassName`: selected from "alicloud-disk-common"(alicloud common disk), "alicloud-disk-efficiency"(alicloud efficiency disk), "alicloud-disk-ssd"(alicloud ssd disk);

`accessModes`: support "ReadWriteOnce" for alicloud disk;

`storage`: config the expect disk size;

```console
$ kubectl create -f deploy/pvc.yaml
persistentvolumeclaim "disk" created
# kubectl get pvc
NAME      STATUS    VOLUME                   CAPACITY   ACCESS MODES   STORAGECLASS           AGE
disk      Bound     d-bp1cz8sslda31ld2snbq   20Gi       RWO            alicloud-disk-common   11s
# kubectl get pv
NAME                     CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS    CLAIM          STORAGECLASS           REASON    AGE
d-bp1cz8sslda31ld2snbq   20Gi       RWO            Delete           Bound     default/disk   alicloud-disk-common             14s
```
