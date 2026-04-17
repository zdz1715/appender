package appender

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"time"
)

type FileFollower struct {
	cc   Appender
	opts uploaderOptions

	filename string

	file   *os.File
	reader *bufio.Reader

	lines   []byte
	readLen int64

	uploadOffset   int64
	lastUploadTime time.Time

	done chan struct{}
}

func NewFileFollower(filename string, cc Appender, opts ...UploaderOption) *FileFollower {
	opt := uploaderOptions{
		readBuffSize:    4096,
		uploadChunkSize: 1024,
		interval:        500 * time.Millisecond,
	}

	for _, o := range opts {
		o(&opt)
	}

	return &FileFollower{
		cc:       cc,
		opts:     opt,
		filename: filename,
		lines:    make([]byte, 0, opt.uploadChunkSize),
		done:     make(chan struct{}),
	}
}

func (f *FileFollower) Done() <-chan struct{} {
	return f.done
}

func (f *FileFollower) close() error {
	if f.file != nil {
		return f.file.Close()
	}
	return nil
}

func (f *FileFollower) reopen() error {
	if err := f.close(); err != nil {
		return err
	}
	file, err := os.Open(f.filename)
	if err != nil {
		return err
	}
	f.file = file
	return nil
}

func (f *FileFollower) reset(offset int64) error {
	if err := f.reopen(); err != nil {
		return err
	}
	f.readLen = offset
	f.lines = f.lines[:0]
	f.uploadOffset = 0
	if f.reader == nil {
		f.reader = bufio.NewReaderSize(f.file, f.opts.readBuffSize)
	} else {
		f.reader.Reset(f.file)
	}
	return f.offset()
}

func (f *FileFollower) offset() error {
	fileInfo, err := f.file.Stat()
	if err != nil {
		return err
	}

	if fileInfo.Size() != f.readLen {
		if fileInfo.Size() < f.readLen {
			f.readLen = fileInfo.Size()
		}
		// reset offset
		f.readLen, err = f.file.Seek(f.readLen, io.SeekStart)
		if err != nil {
			return err
		}
		// Reset bufio.Reader to clear buffer and sync with new file position
		f.reader.Reset(f.file)
		return nil
	}

	return nil
}

func (f *FileFollower) append(ctx context.Context, id string, data []byte) error {
	if f.cc == nil || len(data) == 0 {
		return nil
	}
	if err := f.cc.Append(ctx, id, data, f.uploadOffset); err != nil {
		return err
	}
	f.uploadOffset += int64(len(data))
	return nil
}

func (f *FileFollower) upload(ctx context.Context, id string) error {
	f.lastUploadTime = time.Now()
	if len(f.lines) == 0 {
		return nil
	}
	lineCopy := make([]byte, len(f.lines))
	copy(lineCopy, f.lines)
	if err := f.append(ctx, id, lineCopy); err != nil {
		return err
	}
	f.lines = f.lines[:0]
	return nil
}

func (f *FileFollower) readline(ctx context.Context, id string) (err error) {
	if len(f.lines) >= f.opts.uploadChunkSize || time.Since(f.lastUploadTime) >= f.opts.interval {
		if err = f.upload(ctx, id); err != nil {
			return err
		}
	}

	line, err := f.reader.ReadBytes('\n')
	if err == nil && len(line) > 0 {
		f.readLen += int64(len(line))
		f.lines = append(f.lines, line...)
	}
	return err
}

func (f *FileFollower) RunFrom(ctx context.Context, id string, offset int64, stop <-chan struct{}) (err error) {
	defer func() {
		if finishErr := finish(f.cc, ctx, id); finishErr != nil {
			err = errors.Join(err, finishErr)
		}
		if closeErr := f.close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
		close(f.done)
	}()

	// Seek to the starting offset
	if err = f.reset(offset); err != nil {
		return err
	}
	if err = del(f.cc, ctx, id); err != nil {
		return err
	}
	if err = f.append(ctx, id, f.opts.desc); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-stop:
			// refresh file content when stop
			if err = f.offset(); err != nil {
				return err
			}

			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
					if err = f.readline(ctx, id); err != nil {
						if err == io.EOF {
							return f.upload(ctx, id)
						}
						return err
					}
				}
			}
		default:
			if err = f.readline(ctx, id); err != nil {
				if err != io.EOF {
					return err
				}
			}
			if err = f.offset(); err != nil {
				return err
			}
			// file is always EOF
			time.Sleep(f.opts.interval)
		}
	}
}

func (f *FileFollower) Run(ctx context.Context, id string, stop <-chan struct{}) error {
	return f.RunFrom(ctx, id, 0, stop)
}
