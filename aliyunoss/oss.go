package aliyunoss

import (
	"bytes"
	"context"
	"io"
	"strings"
	"time"

	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss"
	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss/credentials"
	"github.com/zdz1715/appender"
)

type Client struct {
	cc  *oss.Client
	cfg Config
}

func NewClient(cfg Config) *Client {
	osscfg := oss.LoadDefaultConfig().
		WithRegion(cfg.Region).
		WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				cfg.AccessKeyId,
				cfg.AccessKeySecret,
			),
		)

	if cfg.Endpoint != "" {
		osscfg.WithEndpoint(cfg.Endpoint)
	}

	if cfg.ObjectPrefix != "" {
		cfg.ObjectPrefix = strings.Trim(cfg.ObjectPrefix, "/")
	}

	if cfg.PresignExpire <= 0 {
		cfg.PresignExpire = time.Hour * 24 * 7 // max 7 days
	}

	c := &Client{
		cc:  oss.NewClient(osscfg),
		cfg: cfg,
	}

	return c
}

func (c *Client) key(key string) string {
	key = strings.Trim(key, "/")
	if c.cfg.ObjectPrefix == "" {
		return key
	}
	return c.cfg.ObjectPrefix + "/" + key
}

func (c *Client) GetContent(ctx context.Context, id string) (io.ReadCloser, error) {
	key := c.key(id)
	resp, err := c.cc.GetObject(ctx, &oss.GetObjectRequest{
		Bucket: oss.Ptr(c.cfg.Bucket),
		Key:    oss.Ptr(key),
	})
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (c *Client) Get(ctx context.Context, id string) (*appender.Metadata, error) {
	key := c.key(id)

	resp, err := c.cc.Presign(ctx, &oss.GetObjectRequest{
		Bucket:                     oss.Ptr(c.cfg.Bucket),
		Key:                        oss.Ptr(key),
		ResponseContentDisposition: oss.Ptr("attachment;filename=" + id),
	}, oss.PresignExpires(c.cfg.PresignExpire))

	if err != nil {
		return nil, err
	}

	return &appender.Metadata{
		Path:       resp.URL,
		Expiration: &resp.Expiration,
	}, nil
}

func (c *Client) V2Client() *oss.Client {
	return c.cc
}

func (c *Client) Append(ctx context.Context, id string, data []byte, offset int64) error {
	key := c.key(id)
	_, err := c.cc.AppendObject(ctx, &oss.AppendObjectRequest{
		Bucket:      oss.Ptr(c.cfg.Bucket),
		Key:         oss.Ptr(key),
		Position:    oss.Ptr(offset),
		Body:        bytes.NewReader(data),
		ContentType: oss.Ptr("text/plain; charset=utf-8"),
	})
	return err
}

func (c *Client) Delete(ctx context.Context, id string) error {
	key := c.key(id)
	_, err := c.cc.DeleteObject(ctx, &oss.DeleteObjectRequest{
		Bucket: oss.Ptr(c.cfg.Bucket),
		Key:    oss.Ptr(key),
	})

	return err
}
