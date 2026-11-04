package runtime

import (
	"context"
	"io"
	"net/http"
)

// UseDownloadContext scopes downloads to the lifetime of this CLI worker.
// Each project runs in its own process; call before starting its preparation,
// and restore only after all synchronous runtime operations have returned.
func UseDownloadContext(ctx context.Context) func() {
	previous := downloadClient
	copy := *previous
	base := previous.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	copy.Transport = contextTransport{ctx, base}
	downloadClient = &copy
	return func() { downloadClient = previous }
}

type contextTransport struct {
	ctx  context.Context
	base http.RoundTripper
}

func (t contextTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	ctx, cancel := context.WithCancel(r.Context())
	stop := context.AfterFunc(t.ctx, cancel)
	response, err := t.base.RoundTrip(r.Clone(ctx))
	if err != nil {
		stop()
		cancel()
		return nil, err
	}
	response.Body = &contextBody{ReadCloser: response.Body, finish: func() { stop(); cancel() }}
	return response, nil
}

type contextBody struct {
	io.ReadCloser
	finish func()
}

func (b *contextBody) Close() error { err := b.ReadCloser.Close(); b.finish(); return err }
