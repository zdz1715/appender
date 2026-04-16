package aliyunoss

import "time"

type Config struct {
	Endpoint        string
	Region          string
	AccessKeyId     string
	AccessKeySecret string
	Bucket          string
	ObjectPrefix    string
	PresignExpire   time.Duration
}
