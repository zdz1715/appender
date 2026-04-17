package appender

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
)

type FileDriver struct {
	dir   string
	files sync.Map // map[string]*os.File
	opts  fileDriverOptions
}

type fileDriverOptions struct {
	dirMode  os.FileMode
	fileMode os.FileMode
}

type FileDriverOption func(*fileDriverOptions)

func WithDirMode(perm os.FileMode) FileDriverOption {
	return func(o *fileDriverOptions) {
		if perm != 0 {
			o.dirMode = perm
		}
	}
}

func WithFileMode(perm os.FileMode) FileDriverOption {
	return func(o *fileDriverOptions) {
		if perm != 0 {
			o.fileMode = perm
		}
	}
}

func NewFileDriver(dir string, opt ...FileDriverOption) (*FileDriver, error) {
	opts := fileDriverOptions{
		dirMode:  0755,
		fileMode: 0644,
	}
	for _, o := range opt {
		o(&opts)
	}
	if err := os.MkdirAll(dir, opts.dirMode); err != nil {
		return nil, err
	}

	return &FileDriver{
		dir:  dir,
		opts: opts,
	}, nil
}

func (d *FileDriver) filePath(id string) string {
	return filepath.Join(d.dir, id)
}

func (d *FileDriver) getFile(id string) (*os.File, error) {
	if f, ok := d.files.Load(id); ok {
		return f.(*os.File), nil
	}

	path := d.filePath(id)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, d.opts.fileMode)
	if err != nil {
		return nil, err
	}

	d.files.Store(id, file)
	return file, nil
}

func (d *FileDriver) Get(ctx context.Context, id string) (*Metadata, error) {
	path := d.filePath(id)
	_, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	return &Metadata{
		Path: path,
	}, nil
}

func (d *FileDriver) GetContent(ctx context.Context, id string) (io.ReadCloser, error) {
	path := d.filePath(id)
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	return file, nil
}

func (d *FileDriver) Delete(ctx context.Context, id string) error {
	if f, ok := d.files.LoadAndDelete(id); ok {
		_ = f.(*os.File).Close()
	}

	path := d.filePath(id)
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (d *FileDriver) Append(ctx context.Context, id string, data []byte, offset int64) error {
	file, err := d.getFile(id)
	if err != nil {
		return err
	}

	if _, err := file.Write(data); err != nil {
		return err
	}

	return nil
}

func (d *FileDriver) Close() error {
	d.files.Range(func(key, value interface{}) bool {
		if f, ok := value.(*os.File); ok {
			_ = f.Close()
		}
		d.files.Delete(key)
		return true
	})
	return nil
}

func (d *FileDriver) Finish(ctx context.Context, id string) error {
	if f, ok := d.files.LoadAndDelete(id); ok {
		return f.(*os.File).Close()
	}
	return nil
}
