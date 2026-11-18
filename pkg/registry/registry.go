// Package registry opens apt repositories published to OCI registries.
package registry

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/memory"
	"oras.land/oras-go/v2/registry"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"
)

const mediaTypeDockerManifestList = "application/vnd.docker.distribution.manifest.list.v2+json"

// Repo is an opened ref: its files by title, and their blobs, which the caller
// verifies.
type Repo struct {
	content.Fetcher
	Files map[string]ocispec.Descriptor
}

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

func (c *Client) credential(ctx context.Context, registry string, timeout time.Duration) (auth.Credential, error) {
	if c.err != nil {
		return auth.EmptyCredential, c.err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return c.store.Get(ctx, credentials.ServerAddressFromRegistry(registry))
}

// Open resolves ref and maps the files it serves. A request fails after
// timeout without progress.
func (c *Client) Open(ctx context.Context, ref registry.Reference, timeout time.Duration) (*Repo, error) {
	// ref's registry, not the dialed host, as docker looks credentials up
	cred, err := c.credential(ctx, ref.Registry, timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to get credentials of %s: %w", ref.Registry, err)
	}
	client := &auth.Client{
		Client:     c.httpClient(timeout),
		Cache:      c.cache,
		Credential: auth.StaticCredential(ref.Registry, cred),
	}
	repo := &remote.Repository{
		Reference: ref,
		Client:    client,
		// TODO: support insecure non-TLS registry
		PlainHTTP: isLoopback(ref.Registry),
	}
	files, err := buildFileMap(ctx, repo, ref.ReferenceOrDefault())
	if err != nil {
		return nil, fmt.Errorf("failed to resolve ref=%q: %w", ref, err)
	}
	return &Repo{Fetcher: repo.Blobs(), Files: files}, nil
}

// buildFileMap maps the title of each layer under reference to its descriptor.
// oras.Copy fetches only the manifests, verified and capped at MaxMetadataBytes.
func buildFileMap(ctx context.Context, src oras.ReadOnlyTarget, reference string) (map[string]ocispec.Descriptor, error) {
	files := make(map[string]ocispec.Descriptor)
	var filesMu sync.Mutex
	opts := oras.DefaultCopyOptions
	opts.FindSuccessors = func(ctx context.Context, fetcher content.Fetcher, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
		successors, err := content.Successors(ctx, fetcher, desc)
		if err != nil {
			return nil, err
		}
		switch desc.MediaType {
		case ocispec.MediaTypeImageIndex, mediaTypeDockerManifestList:
			return successors, nil
		}
		filesMu.Lock()
		defer filesMu.Unlock()
		for _, l := range successors {
			if title := l.Annotations[ocispec.AnnotationTitle]; title != "" {
				files[path.Clean(title)] = l
			}
		}
		return nil, nil
	}
	if _, err := oras.Copy(ctx, src, reference, memory.New(), "", opts); err != nil {
		return nil, err
	}
	return files, nil
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

// dockerConfigPath falls back to passwd if $HOME is unset.
// TODO: replace with NewStoreFromDocker after oras-project/oras-go#1785.
func dockerConfigPath() (string, error) {
	if dir := os.Getenv("DOCKER_CONFIG"); dir != "" {
		return filepath.Join(dir, "config.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		u, uerr := user.Current()
		if uerr != nil {
			return "", errors.Join(err, uerr)
		}
		home = u.HomeDir
	}
	return filepath.Join(home, ".docker", "config.json"), nil
}
