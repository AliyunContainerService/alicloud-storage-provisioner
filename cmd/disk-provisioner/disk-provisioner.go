package main

import (
	"fmt"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/denverdino/aliyungo/common"
	"github.com/denverdino/aliyungo/ecs"
	"github.com/kubernetes-incubator/external-storage/lib/controller"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/pkg/volume"
	"k8s.io/kubernetes/pkg/volume/util/volumehelper"
)

const (
	CRED_PATH                       = "/etc/kubernetes/cloud-config"
	ENCODE_CRED_PATH                = "/etc/kubernetes/cloud-config.alicloud"
	KUBERNETES_ALICLOUD_DISK_DRIVER = "alicloud/disk"
	PROVISION_ID                    = "alicloud-disk-dynamic-provisioner"
	METADATA_URL                    = "http://100.100.100.200/latest/meta-data/"
	REGIONID_TAG                    = "region-id"
	ZONEID_TAG                      = "zone-id"
	REGIONID_LABEL_TAG              = "failure-domain.beta.kubernetes.io/region"
	ZONEID_LABEL_TAG                = "failure-domain.beta.kubernetes.io/zone"
	LOGFILE_PREFIX                  = "/var/log/alicloud/provisioner"
	DISK_FSTYPE                     = "fstype"
	DISK_READONLY                   = "readonly"
	DISK_TYPE                       = "type"
	DISK_REGIONID                   = "regionid"
	DISK_ZONEID                     = "zoneid"
	DISK_ENCRYPTED                  = "encrypted"
	DISK_NOTAVAILABLE               = "InvalidDataDiskCategory.NotSupported"
	DISK_HIGH_AVAIL                 = "available"
	DISK_COMMON                     = "cloud"
	DISK_EFFICIENCY                 = "cloud_efficiency"
	DISK_SSD                        = "cloud_ssd"
	MB_SIZE                         = 1024 * 1024
)

type diskProvisioner struct {
	client    kubernetes.Interface
	EcsClient *ecs.Client
	region    common.Region
	identity  string
}

//
var _ controller.Provisioner = &diskProvisioner{}

// NewDiskProvisioner creates an alicloud Disk volume provisioner
func NewDiskProvisioner(client kubernetes.Interface) controller.Provisioner {
	log.Infof("Successful create alicloud disk provisioner.")
	return &diskProvisioner{
		client:    client,
		EcsClient: nil,
		region:    DEFAULT_REGION,
	}
}

// Provision creates a storage asset and returns a PV object representing it.
func (p *diskProvisioner) Provision(options controller.VolumeOptions) (*v1.PersistentVolume, error) {
	// Step 1: check input pvc
	if options.PVC == nil {
		return nil, fmt.Errorf("Recieve options with nil pvc ")
	}
	log.Infof("Create Disk by PVC: %s, PVC-Define: %v, Parameters: %s", options.PVC.Name, *options.PVC, options.Parameters)

	// Step 2: Create Disk volume
	volumeId, sizeGB, err := p.createVolume(options)
	if err != nil {
		log.Errorf("Create Disk error: %s", err.Error())
		return nil, err
	}

	// Step 3: set pv parametes, add region, zone labels if pvc defined
	labels := map[string]string{}
	regionId, zoneId, _, fsType, readOnly, _ := p.getRegionZoneId(options)
	labels[REGIONID_LABEL_TAG] = regionId
	labels[ZONEID_LABEL_TAG] = zoneId

	// Step 4: Make PV definition
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:   volumeId,
			Labels: labels,
			Annotations: map[string]string{
				volumehelper.VolumeDynamicallyCreatedByKey: PROVISION_ID,
			},
		},
		Spec: v1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: options.PersistentVolumeReclaimPolicy,
			AccessModes:                   options.PVC.Spec.AccessModes,
			Capacity: v1.ResourceList{
				v1.ResourceName(v1.ResourceStorage): resource.MustParse(fmt.Sprintf("%dGi", sizeGB)),
			},
			PersistentVolumeSource: v1.PersistentVolumeSource{
				FlexVolume: &v1.FlexPersistentVolumeSource{
					Driver: KUBERNETES_ALICLOUD_DISK_DRIVER,
					Options: map[string]string{
						"VolumeId": volumeId,
					},
					ReadOnly: readOnly,
					FSType:   fsType,
				},
			},
		},
	}
	log.Infof("Create PV with Name: %s, Labels: %s, Reclaim: %s, AccessMode: %s, Size: %v, ReadOnly: %v, fsType: %s", volumeId, labels, options.PersistentVolumeReclaimPolicy, options.PVC.Spec.AccessModes, sizeGB, readOnly, fsType)

	return pv, nil
}

// create disk with api
func (p *diskProvisioner) createVolume(options controller.VolumeOptions) (string, int, error) {
	// Step 1: alicloud works with gigabytes, convert to GiB with rounding up
	capacity := options.PVC.Spec.Resources.Requests[v1.ResourceName(v1.ResourceStorage)]
	requestBytes := capacity.Value()
	requestGB := int(volume.RoundUpSize(requestBytes, 1024*1024*1024))

	// Step 2: init client
	p.initEcsClient()
	if p.EcsClient == nil {
		//log.Warnf("New Ecs Client error when create")
		return "", 0, fmt.Errorf("init ecs client error")
	}

	// Step 3: Get region_id and zone_id, if pvc not define, get from ecs metadata
	regionId, zoneId, disktypeSet, _, _, encrypted := p.getRegionZoneId(options)
	if regionId == "" || zoneId == "" {
		return "", 0, fmt.Errorf("Get region_id, zone_id error: %s, %s ", regionId, zoneId)
	}

	disktype := disktypeSet
	if DISK_HIGH_AVAIL == disktypeSet {
		disktype = DISK_EFFICIENCY
	}

	// Step 4: init Disk create args
	volumeOptions := &ecs.CreateDiskArgs{
		Size:         requestGB,
		RegionId:     common.Region(regionId),
		ZoneId:       zoneId,
		DiskCategory: ecs.DiskCategory(disktype),
		Encrypted:    encrypted,
	}

	// Step 5: Create Disk
	volumeId, err := p.EcsClient.CreateDisk(volumeOptions)
	if err != nil {
		// if available feature enable, try with ssd again
		if disktypeSet == DISK_HIGH_AVAIL && strings.Contains(err.Error(), DISK_NOTAVAILABLE) {
			disktype = DISK_SSD
			volumeOptions.DiskCategory = ecs.DiskCategory(disktype)
			volumeId, err = p.EcsClient.CreateDisk(volumeOptions)

			if err != nil {
				if strings.Contains(err.Error(), DISK_NOTAVAILABLE) {
					disktype = DISK_COMMON
					volumeOptions.DiskCategory = ecs.DiskCategory(disktype)
					volumeId, err = p.EcsClient.CreateDisk(volumeOptions)
					if err != nil {
						return "", 0, err
					}

				} else {
					return "", 0, err
				}
			}

		} else {
			return "", 0, err
		}
	}
	log.Infof("Successfully created Disk: %s, %s, %s, %s", volumeId, regionId, zoneId, disktype)

	return volumeId, int(requestGB), nil
}

func (p *diskProvisioner) getRegionZoneId(options controller.VolumeOptions) (string, string, string, string, bool, bool) {
	regionId, zoneId, diskType := "", "", ""
	fsType := "ext4"
	readOnly := false
	encrypted := false

	for k, v := range options.Parameters {
		switch strings.ToLower(k) {
		case DISK_REGIONID:
			regionId = strings.TrimSpace(v)
		case DISK_ZONEID:
			zoneId = strings.TrimSpace(v)
		case DISK_TYPE:
			diskType = strings.TrimSpace(v)
		case DISK_FSTYPE:
			fsType = strings.TrimSpace(v)
		case DISK_READONLY:
			if strings.ToLower(v) == "true" || v == "1" || strings.ToLower(v) == "yes" {
				readOnly = true
			}
		case DISK_ENCRYPTED:
			if strings.ToLower(v) == "true" || v == "1" || strings.ToLower(v) == "yes" {
				encrypted = true
			}
		default:
		}
	}

	if options.PVC.Spec.Selector != nil {
		for label, value := range options.PVC.Spec.Selector.MatchLabels {
			if label == REGIONID_LABEL_TAG {
				regionId = value
			} else if label == ZONEID_LABEL_TAG {
				zoneId = value
			}
		}
	}

	if zoneId == "" || regionId == "" {
		regionId = GetMetaData(REGIONID_TAG)
		zoneId = GetMetaData(ZONEID_TAG)
	}
	if diskType == "" {
		diskType = DISK_HIGH_AVAIL
	}
	return regionId, zoneId, diskType, fsType, readOnly, encrypted
}

// Delete removes the storage asset that was created by Provision represented
// by the given PV.
func (p *diskProvisioner) Delete(volume *v1.PersistentVolume) error {
	log.Infof("Delete disk: %s, %v", volume.Name, volume.Spec)

	provisioned, err := p.provisioned(volume)
	if err != nil {
		log.Errorf("error determining if this provisioner was the one to provision volume %s: %s", volume.Name, err.Error())
		return fmt.Errorf("error determining if this provisioner was the one to provision volume %q: %v", volume.Name, err)

	}
	if !provisioned {
		strerr := fmt.Sprintf("this provisioner id %s didn't provision volume %q and so can't delete it; id %s did & can", p.identity, volume.Name, volume.Annotations[volumehelper.VolumeDynamicallyCreatedByKey])
		log.Warnf(strerr)
		return &controller.IgnoredError{Reason: strerr}
	}

	p.initEcsClient()
	if p.EcsClient == nil {
		log.Warnf("New Ecs Client error when delete")
		return fmt.Errorf("init ecs client error while delete")
	}

	err = p.EcsClient.DeleteDisk(volume.Name)
	if err != nil {
		// wait flexvolume to detach disk first.
		if strings.Contains(err.Error(), "IncorrectDiskStatus") {
			for i := 0; i < 3; i++ {
				time.Sleep(3000 * time.Millisecond)
				err = p.EcsClient.DeleteDisk(volume.Name)
				if err == nil {
					break
				} else if i == 2 {
					log.Warnf("Delete disk 3 times, but error: %s", err.Error())
					return fmt.Errorf("Delete disk 3 times, but error: %s ", err.Error())
				}
			}
		} else {
			log.Warnf("Delete disk with error: %s", err.Error())
			return fmt.Errorf("Delete disk with error: %s ", err.Error())
		}
	}

	log.Infof("Successful Delete disk: %s", volume.Name)
	return nil
}

func (p *diskProvisioner) provisioned(volume *v1.PersistentVolume) (bool, error) {
	provisionerID, ok := volume.Annotations[volumehelper.VolumeDynamicallyCreatedByKey]
	if !ok {
		return false, fmt.Errorf("PV doesn't have an annotation %s", volumehelper.VolumeDynamicallyCreatedByKey)
	}
	return provisionerID == PROVISION_ID, nil

}

func (p *diskProvisioner) initEcsClient() {
	accessKeyID, accessSecret, accessToken := GetDefaultAK()
	log.Debug("Debug Ak: ", accessKeyID, "***", "***")
	p.EcsClient = newEcsClient(accessKeyID, accessSecret, accessToken)
}
