package run

import (
	"context"
	"errors"
	"io"

	"github.com/maxrodrigo/clai/internal/provider"
)

// fakeProviderImpl implements provider.Provider for testing.
type fakeProviderImpl struct {
	response provider.Response
	err      error
}

func (f *fakeProviderImpl) Name() string { return "fake" }

func (f *fakeProviderImpl) Complete(_ context.Context, _ provider.Request) (provider.Response, error) {
	return f.response, f.err
}

func (f *fakeProviderImpl) CompleteStream(_ context.Context, _ provider.Request, w io.Writer) (provider.Response, error) {
	if f.err != nil {
		return provider.Response{}, f.err
	}
	_, _ = w.Write([]byte(f.response.Content))
	return f.response, nil
}

func (f *fakeProviderImpl) Models(_ context.Context) ([]string, error) {
	return nil, errors.New("not implemented")
}
