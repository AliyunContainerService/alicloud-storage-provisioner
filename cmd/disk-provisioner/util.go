package main

import (
	b64 "encoding/base64"
	"encoding/json"
	log "github.com/Sirupsen/logrus"
	"github.com/denverdino/aliyungo/common"
	"github.com/denverdino/aliyungo/ecs"
	"github.com/denverdino/aliyungo/metadata"
	"io/ioutil"
	"net/http"
	"os"
	"time"
)

// struct for access key
type DefaultOptions struct {
	Global struct {
		KubernetesClusterTag string
		AccessKeyID          string `json:"accessKeyID"`
		AccessKeySecret      string `json:"accessKeySecret"`
		Region               string `json:"region"`
	}
}

func newEcsClient(access_key_id, access_key_secret, access_token string) *ecs.Client {
	ecs_endpoint := ""
	m := metadata.NewMetaData(nil)
	region, err := m.Region()
	if err != nil {
		region = string(DEFAULT_REGION)
	}

	// use environment endpoint first;
	if ep := os.Getenv("ECS_ENDPOINT"); ep != "" {
		ecs_endpoint = ep
	}

	client := ecs.NewECSClientWithEndpointAndSecurityToken(ecs_endpoint, access_key_id, access_key_secret, access_token, common.Region(region))
	client.SetUserAgent(KUBERNETES_ALICLOUD_IDENTITY)

	return client
}

// read default ak from local file or from STS
func GetDefaultAK() (string, string, string) {
	accessKeyID, accessSecret := GetLocalAK()

	accessToken := ""
	if accessKeyID == "" || accessSecret == "" {
		accessKeyID, accessSecret, accessToken = GetSTSAK()
	}

	return accessKeyID, accessSecret, accessToken

}

// get STS AK
func GetSTSAK() (string, string, string) {
	m := metadata.NewMetaData(nil)

	rolename := ""
	var err error
	if rolename, err = m.Role(); err != nil {
		log.Errorf("Get role name error: ", err.Error())
		return "", "", ""
	}
	role, err := m.RamRoleToken(rolename)
	if err != nil {
		log.Errorf("Get STS Token error, ", err.Error())
		return "", "", ""
	}
	return role.AccessKeyId, role.AccessKeySecret, role.SecurityToken
}

func GetLocalAK() (string, string) {
	var accessKeyID, accessSecret string
	var defaultOpt DefaultOptions

	// first check if the environment setting
	accessKeyID = os.Getenv("ACCESS_KEY_ID")
	accessSecret = os.Getenv("ACCESS_KEY_SECRET")
	if accessKeyID != "" && accessSecret != "" {
		return accessKeyID, accessSecret
	}

	if IsFileExisting(ENCODE_CRED_PATH) {
		raw, err := ioutil.ReadFile(ENCODE_CRED_PATH)
		if err != nil {
			log.Debug("Read cred file failed:  ", err.Error())
			return "", ""
		}
		err = json.Unmarshal(raw, &defaultOpt)
		if err != nil {
			log.Warnf("Parse json cert file error: ", err.Error())
			return "", ""
		}
		keyID, err := b64.StdEncoding.DecodeString(defaultOpt.Global.AccessKeyID)
		if err != nil {
			log.Warnf("Decode accesskeyid failed: ", err.Error())
			return "", ""
		}
		secret, err := b64.StdEncoding.DecodeString(defaultOpt.Global.AccessKeySecret)
		if err != nil {
			log.Warnf("Decode secret failed: ", err.Error())
			return "", ""
		}
		accessKeyID = string(keyID)
		accessSecret = string(secret)
	} else if IsFileExisting(CRED_PATH) {
		raw, err := ioutil.ReadFile(CRED_PATH)
		if err != nil {
			log.Debug("Read cred file failed: ", err.Error())
			return "", ""
		}
		err = json.Unmarshal(raw, &defaultOpt)
		if err != nil {
			log.Debug("New Ecs Client error json: ", err.Error())
			return "", ""
		}
		accessKeyID = defaultOpt.Global.AccessKeyID
		accessSecret = defaultOpt.Global.AccessKeySecret

	} else {
		return "", ""
	}
	return accessKeyID, accessSecret
}

// check file exist in volume driver;
func IsFileExisting(filename string) bool {
	_, err := os.Stat(filename)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	return true
}

// get host regionid, zoneid
func GetMetaData(resource string) string {
	resp, err := http.Get(METADATA_URL + resource)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return ""
	}
	return string(body)
}

// rotate log file by 2M bytes
func setLogAttribute() {

	logFile := LOGFILE_PREFIX + ".log"
	f, err := os.OpenFile(logFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatal("Log File open error: ", err.Error())
	}

	// rotate the log file if too large
	if fi, err := f.Stat(); err == nil && fi.Size() > 5*MB_SIZE {
		f.Close()
		timeStr := time.Now().Format("-2006-01-02-15:04:05")
		timedLogfile := LOGFILE_PREFIX + timeStr + ".log"
		os.Rename(logFile, timedLogfile)
		f, err = os.OpenFile(logFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			log.Fatal("Log File open error2: ", err.Error())
		}
	}
	log.SetOutput(f)
}
