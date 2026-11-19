package registry

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/registry"
)

func TestIsLoopback(t *testing.T) {
	for hostport, want := range map[string]bool{
		"localhost":      true,
		"localhost:5000": true,
		"127.0.0.1":      true,
		"127.0.0.1:5000": true,
		"[::1]":          true,
		"[::1]:5000":     true,
		"ghcr.io":        false,
		"10.0.0.1:5000":  false,
	} {
		if got := isLoopback(hostport); got != want {
			t.Errorf("isLoopback(%q): expected %v, got %v", hostport, want, got)
		}
	}
}

// TestOpenTimeout fails a registry that stops sending, before the headers of
// Open's HEAD or in the body of a manifest Lookup maps, after the timeout given
// to Open.
func TestOpenTimeout(t *testing.T) {
	t.Setenv("DOCKER_CONFIG", t.TempDir())
	manifest := []byte(`{"schemaVersion":2,"mediaType":"` + ocispec.MediaTypeImageManifest + `"}`)
	for name, inBody := range map[string]bool{"headers": false, "body": true} {
		t.Run(name, func(t *testing.T) {
			done := make(chan struct{})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if inBody {
					w.Header().Set("Content-Type", ocispec.MediaTypeImageManifest)
					w.Header().Set("Docker-Content-Digest", digest.FromBytes(manifest).String())
					w.Header().Set("Content-Length", strconv.Itoa(len(manifest)))
					if r.Method == http.MethodHead {
						return
					}
					w.Write(manifest[:1])
					w.(http.Flusher).Flush()
				}
				select {
				case <-r.Context().Done():
				case <-done:
				}
			}))
			defer srv.Close()
			defer close(done)
			ref, err := registry.ParseReference(strings.TrimPrefix(srv.URL, "http://") + "/repo:tag")
			if err != nil {
				t.Fatal(err)
			}
			start := time.Now()
			repo, err := NewClient().Open(context.Background(), ref, 100*time.Millisecond)
			if err == nil {
				_, _, err = repo.Lookup(context.Background(), "InRelease")
			}
			var netErr net.Error
			if !errors.As(err, &netErr) || !netErr.Timeout() || time.Since(start) > 5*time.Second {
				t.Errorf("expected a timeout soon after 100ms, got %v after %v", err, time.Since(start))
			}
		})
	}
}

// TestOpenResolve resolves the ref with a HEAD, and with a GET only when the
// HEAD succeeds without Docker-Content-Digest, as oras-go computes its digest.
func TestOpenResolve(t *testing.T) {
	t.Setenv("DOCKER_CONFIG", t.TempDir())
	manifest := []byte(`{"schemaVersion":2,"mediaType":"` + ocispec.MediaTypeImageManifest + `"}`)
	for name, tc := range map[string]struct {
		status       int
		digestHeader bool
		want         []string
	}{
		"digest header":    {http.StatusOK, true, []string{"HEAD"}},
		"no digest header": {http.StatusOK, false, []string{"HEAD", "GET"}},
		"not found":        {http.StatusNotFound, true, []string{"HEAD"}},
		"server error":     {http.StatusInternalServerError, true, []string{"HEAD"}},
	} {
		t.Run(name, func(t *testing.T) {
			var methods []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				methods = append(methods, r.Method)
				if tc.status != http.StatusOK {
					w.WriteHeader(tc.status)
					return
				}
				w.Header().Set("Content-Type", ocispec.MediaTypeImageManifest)
				if tc.digestHeader {
					w.Header().Set("Docker-Content-Digest", digest.FromBytes(manifest).String())
				}
				w.Header().Set("Content-Length", strconv.Itoa(len(manifest)))
				w.Write(manifest)
			}))
			defer srv.Close()
			ref, err := registry.ParseReference(strings.TrimPrefix(srv.URL, "http://") + "/repo:tag")
			if err != nil {
				t.Fatal(err)
			}
			repo, err := NewClient().Open(context.Background(), ref, time.Second)
			if ok := tc.status == http.StatusOK; ok != (err == nil) {
				t.Fatalf("expected success=%v, got %v", ok, err)
			}
			if err == nil && repo.root.Digest != digest.FromBytes(manifest) {
				t.Errorf("expected root %s, got %s", digest.FromBytes(manifest), repo.root.Digest)
			}
			if !slices.Equal(methods, tc.want) {
				t.Errorf("expected requests %v, got %v", tc.want, methods)
			}
		})
	}
}
