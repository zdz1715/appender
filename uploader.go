package appender

import (
	"bufio"
	"context"
	"io"
	"time"
)

type uploaderOptions struct {
	uploadChunkSize int
	readBuffSize    int
	interval        time.Duration
	desc            []byte
}

type UploaderOption func(*uploaderOptions)

func WithDesc(desc []byte) UploaderOption {
	return func(o *uploaderOptions) {
		o.desc = desc
	}
}

func WithUploadChunkSize(chunkSize int) UploaderOption {
	return func(o *uploaderOptions) {
		if chunkSize > 0 {
			o.uploadChunkSize = chunkSize
		}
	}
}

func WithReadBuffSize(buffSize int) UploaderOption {
	return func(o *uploaderOptions) {
		if buffSize > 0 {
			o.readBuffSize = buffSize
		}
	}
}

func WithInterval(interval time.Duration) UploaderOption {
	return func(o *uploaderOptions) {
		if interval > 0 {
			o.interval = interval
		}
	}
}

type StreamUploader struct {
	cc   Driver
	opts uploaderOptions

	reader *bufio.Reader

	lines   []byte
	readLen int64

	uploadOffset   int64
	lastUploadTime time.Time

	done chan struct{}
}

func NewStreamUploader(reader io.Reader, cc Driver, opts ...UploaderOption) *StreamUploader {
	opt := uploaderOptions{
		readBuffSize:    4096,
		uploadChunkSize: 1024,
		interval:        500 * time.Millisecond,
	}

	for _, o := range opts {
		o(&opt)
	}

	return &StreamUploader{
		cc:     cc,
		opts:   opt,
		reader: bufio.NewReaderSize(reader, opt.readBuffSize),
		lines:  make([]byte, 0, opt.uploadChunkSize),
		done:   make(chan struct{}),
	}
}

func (u *StreamUploader) Done() <-chan struct{} {
	return u.done
}

func (u *StreamUploader) delete(ctx context.Context, id string) error {
	if u.cc == nil {
		return nil
	}
	return u.cc.Delete(ctx, id)
}

func (u *StreamUploader) append(ctx context.Context, id string, data []byte) error {
	if u.cc == nil || len(data) == 0 {
		return nil
	}
	if err := u.cc.Append(ctx, id, data, u.uploadOffset); err != nil {
		return err
	}
	u.uploadOffset += int64(len(data))
	return nil
}

func (u *StreamUploader) upload(ctx context.Context, id string) error {
	u.lastUploadTime = time.Now()
	if len(u.lines) == 0 {
		return nil
	}
	lineCopy := make([]byte, len(u.lines))
	copy(lineCopy, u.lines)
	if err := u.append(ctx, id, lineCopy); err != nil {
		return err
	}
	u.lines = u.lines[:0]
	return nil
}

func (u *StreamUploader) readline(ctx context.Context, id string) (err error) {
	if len(u.lines) >= u.opts.uploadChunkSize || time.Since(u.lastUploadTime) >= u.opts.interval {
		if err = u.upload(ctx, id); err != nil {
			return err
		}
	}
	line, err := u.reader.ReadBytes('\n')
	if err == nil && len(line) > 0 {
		u.readLen += int64(len(line))
		u.lines = append(u.lines, line...)
	}
	return err
}

func (u *StreamUploader) readAndUpload(ctx context.Context, id string) error {
	if err := u.readline(ctx, id); err != nil {
		if err == io.EOF {
			if uploadErr := u.upload(ctx, id); uploadErr != nil {
				return err
			}
			// return io EOF
		}
		return err
	}
	return nil
}

func (u *StreamUploader) Run(ctx context.Context, id string, stop <-chan struct{}) error {
	defer close(u.done)
	if err := u.delete(ctx, id); err != nil {
		return err
	}
	if len(u.opts.desc) > 0 {
		if err := u.append(ctx, id, u.opts.desc); err != nil {
			return err
		}
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-stop:
			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
					if err := u.readline(ctx, id); err != nil {
						if err == io.EOF {
							return u.upload(ctx, id)
						}
						return err
					}
				}
			}
		default:
			if err := u.readline(ctx, id); err != nil {
				if err == io.EOF {
					return u.upload(ctx, id)
				}
				return err
			}
		}
	}
}
