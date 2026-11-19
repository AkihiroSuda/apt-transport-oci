package registry

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"time"
)

// newClient fails a request after timeout without progress: connecting,
// awaiting headers, or receiving the body, as apt's Timeout.
func newClient(timeout time.Duration) *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DialContext = (&net.Dialer{Timeout: timeout}).DialContext
	tr.TLSHandshakeTimeout = timeout
	tr.ResponseHeaderTimeout = timeout
	// apt runs a method per host, so do not cap idle connections per host
	tr.MaxIdleConnsPerHost = tr.MaxIdleConns
	return &http.Client{Transport: &stallTransport{next: tr, timeout: timeout}}
}

// stallTransport fails a response body that stalls for timeout. It times each
// request, not the connection, as HTTP/2 multiplexes requests on one.
type stallTransport struct {
	next    http.RoundTripper
	timeout time.Duration
}

func (t *stallTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx, cancel := context.WithCancelCause(req.Context())
	resp, err := t.next.RoundTrip(req.WithContext(ctx))
	if err != nil {
		cancel(nil)
		return nil, err
	}
	timer := time.AfterFunc(t.timeout, func() { cancel(os.ErrDeadlineExceeded) })
	resp.Body = &stallBody{ReadCloser: resp.Body, ctx: ctx, cancel: cancel, timer: timer, timeout: t.timeout}
	return resp, nil
}

// stallBody resets the timer whenever data arrives.
type stallBody struct {
	io.ReadCloser
	ctx     context.Context
	cancel  context.CancelCauseFunc
	timer   *time.Timer
	timeout time.Duration
}

func (b *stallBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		b.timer.Reset(b.timeout)
	}
	if err != nil && errors.Is(context.Cause(b.ctx), os.ErrDeadlineExceeded) {
		return n, os.ErrDeadlineExceeded
	}
	return n, err
}

func (b *stallBody) Close() error {
	b.timer.Stop()
	b.cancel(nil)
	return b.ReadCloser.Close()
}
