// Package registry opens apt repositories published to OCI registries.
package registry

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"oras.land/oras-go/v2/errdef"
	"oras.land/oras-go/v2/registry"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"
	"oras.land/oras-go/v2/registry/remote/errcode"
)

// Client opens repositories with the credentials of their registry.
type Client struct {
	cache   auth.Cache
	clients sync.Map
	store   credentials.Store
	err     error
}

// NewClient reads the docker config, so call it before dropping privileges, as
// apt reads auth.conf.
func NewClient() *Client {
	c := &Client{cache: auth.NewCache()}
	configPath, err := dockerConfigPath()
	if err == nil {
		c.store, err = credentials.NewStore(configPath, credentials.StoreOptions{})
	}
	c.err = err
	return c
}

func (c *Client) httpClient(timeout time.Duration) *http.Client {
	client, ok := c.clients.Load(timeout)
	if !ok {
		client, _ = c.clients.LoadOrStore(timeout, newClient(timeout))
	}
	return client.(*http.Client)
}

// Open resolves ref. A request fails after timeout without progress.
func (c *Client) Open(ctx context.Context, ref registry.Reference, timeout time.Duration) (*Repo, error) {
	// ref's registry, not the dialed host, as docker looks credentials up
	credential, err := c.credential(ctx, ref.Registry, timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to get credentials of %s: %w", ref.Registry, err)
	}
	authClient := &auth.Client{
		Client:     c.httpClient(timeout),
		Cache:      c.cache,
		Credential: auth.StaticCredential(ref.Registry, credential),
	}
	repository := &remote.Repository{
		Reference: ref,
		Client:    authClient,
		// TODO: support insecure non-TLS registry
		PlainHTTP: isLoopback(ref.Registry),
	}
	root, err := repository.Resolve(ctx, ref.ReferenceOrDefault())
	var statusErr *errcode.ErrorResponse
	var networkErr net.Error
	if err != nil && !errors.Is(err, errdef.ErrNotFound) && !errors.As(err, &statusErr) && !errors.As(err, &networkErr) {
		// the HEAD succeeded without Docker-Content-Digest, which a GET computes
		var body io.ReadCloser
		if root, body, err = repository.FetchReference(ctx, ref.ReferenceOrDefault()); err == nil {
			body.Close()
		}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to resolve ref=%q: %w", ref, err)
	}
	return &Repo{blobs: repository.Blobs(), target: repository, root: root}, nil
}

// isLoopback reports whether hostport is local, like containerd's docker.MatchLocalhost.
func isLoopback(hostport string) bool {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = strings.TrimSuffix(strings.TrimPrefix(hostport, "["), "]")
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
