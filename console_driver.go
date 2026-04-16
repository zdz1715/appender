package appender

import (
	"context"
	"fmt"
	"io"
)

type ConsoleDriver struct {
	w io.Writer
}

func NewConsoleDriver(w io.Writer) *ConsoleDriver {
	return &ConsoleDriver{w}
}

func (p *ConsoleDriver) Get(ctx context.Context, id string) (*Metadata, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *ConsoleDriver) GetContent(ctx context.Context, id string) (io.ReadCloser, error) {
	return nil, fmt.Errorf("not implemented")
}

func (p *ConsoleDriver) Delete(ctx context.Context, id string) error {
	return nil
}

func (p *ConsoleDriver) Append(ctx context.Context, id string, data []byte, offset int64) error {
	_, err := fmt.Fprint(p.w, string(data))
	return err
}
