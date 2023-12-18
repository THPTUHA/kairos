package runner

import (
	"context"
	"io"
)

type Runner interface {
	Start(ctx context.Context) error
	Stdout() io.ReadCloser
	Stderr() io.ReadCloser
	Name() string
	AttachedRunner
}
type AttachedRunner interface {
	Wait(ctx context.Context) error
	Kill(ctx context.Context) error
	ID() string
	AddrTranslator
}

type AddrTranslator interface {
	PluginToHost(pluginNet, pluginAddr string) (hostNet string, hostAddr string, err error)
	HostToPlugin(hostNet, hostAddr string) (pluginNet string, pluginAddr string, err error)
}
