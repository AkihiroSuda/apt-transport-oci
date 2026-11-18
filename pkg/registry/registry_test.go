package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/memory"
	"oras.land/oras-go/v2/errdef"
	"oras.land/oras-go/v2/registry"
	"oras.land/oras-go/v2/registry/remote"
)

const mediaTypeOctetStream = "application/octet-stream"

func push(t *testing.T, store *memory.Store, mediaType string, b []byte) ocispec.Descriptor {
	t.Helper()
	desc := content.NewDescriptorFromBytes(mediaType, b)
	if err := store.Push(context.Background(), desc, bytes.NewReader(b)); err != nil {
		t.Fatal(err)
	}
	return desc
}

func pushJSON(t *testing.T, store *memory.Store, mediaType string, v any) ocispec.Descriptor {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return push(t, store, mediaType, b)
}

func tag(t *testing.T, store *memory.Store, desc ocispec.Descriptor, reference string) {
	t.Helper()
	if err := store.Tag(context.Background(), desc, reference); err != nil {
		t.Fatal(err)
	}
}

func layer(t *testing.T, store *memory.Store, title, data string) ocispec.Descriptor {
	t.Helper()
	desc := push(t, store, mediaTypeOctetStream, []byte(data))
	if title != "" {
		desc.Annotations = map[string]string{ocispec.AnnotationTitle: title}
	}
	return desc
}

func TestBuildFileMap(t *testing.T) {
	for _, tc := range []struct {
		name, indexType, manifestType string
	}{
		{"oci", ocispec.MediaTypeImageIndex, ocispec.MediaTypeImageManifest},
		{"docker", mediaTypeDockerManifestList, "application/vnd.docker.distribution.manifest.v2+json"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := memory.New()
			config := push(t, store, ocispec.MediaTypeEmptyJSON, []byte("{}"))
			inRelease := layer(t, store, "dists/sid/InRelease", "in-release")
			packages := layer(t, store, "./dists/sid/main/binary-amd64/Packages", "packages")
			deb := layer(t, store, "pool/main/h/hello/hello_1_amd64.deb", "deb")
			untitled := layer(t, store, "", "untitled")

			manifest := func(layers ...ocispec.Descriptor) ocispec.Descriptor {
				return pushJSON(t, store, tc.manifestType, ocispec.Manifest{
					MediaType: tc.manifestType,
					Config:    config,
					Layers:    layers,
				})
			}
			root := pushJSON(t, store, tc.indexType, ocispec.Index{
				MediaType: tc.indexType,
				Manifests: []ocispec.Descriptor{
					manifest(inRelease, packages),
					manifest(deb, untitled),
				},
			})

			tag(t, store, root, "sid")
			files, err := buildFileMap(context.Background(), store, "sid")
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]digest.Digest{
				"dists/sid/InRelease":                  inRelease.Digest,
				"dists/sid/main/binary-amd64/Packages": packages.Digest,
				"pool/main/h/hello/hello_1_amd64.deb":  deb.Digest,
			}
			if len(files) != len(want) {
				t.Fatalf("expected %d files, got %d: %v", len(want), len(files), files)
			}
			for title, dgst := range want {
				if files[title].Digest != dgst {
					t.Errorf("expected %q to be %s, got %s", title, dgst, files[title].Digest)
				}
			}
		})
	}
}

type tamperedTarget struct{ *memory.Store }

func (tamperedTarget) Fetch(context.Context, ocispec.Descriptor) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("{}")), nil
}

func TestBuildFileMapMismatchedManifest(t *testing.T) {
	store := memory.New()
	tag(t, store, pushJSON(t, store, ocispec.MediaTypeImageManifest, ocispec.Manifest{}), "sid")
	if _, err := buildFileMap(context.Background(), tamperedTarget{store}, "sid"); err == nil {
		t.Fatal("expected an error")
	}
}

func TestBuildFileMapOversizedManifest(t *testing.T) {
	store := memory.New()
	big := pushJSON(t, store, ocispec.MediaTypeImageManifest, ocispec.Manifest{
		MediaType:   ocispec.MediaTypeImageManifest,
		Annotations: map[string]string{"pad": strings.Repeat("a", 4<<20)},
	})
	index := pushJSON(t, store, ocispec.MediaTypeImageIndex, ocispec.Index{
		MediaType: ocispec.MediaTypeImageIndex,
		Manifests: []ocispec.Descriptor{big},
	})
	tag(t, store, big, "root")
	tag(t, store, index, "child")
	for _, name := range []string{"root", "child"} {
		t.Run(name, func(t *testing.T) {
			_, err := buildFileMap(context.Background(), store, name)
			if !errors.Is(err, errdef.ErrSizeExceedsLimit) {
				t.Fatalf("expected %v, got %v", errdef.ErrSizeExceedsLimit, err)
			}
		})
	}
}

func TestBuildFileMapOversizedRemoteManifest(t *testing.T) {
	b, err := json.Marshal(ocispec.Manifest{
		MediaType:   ocispec.MediaTypeImageManifest,
		Annotations: map[string]string{"pad": strings.Repeat("a", 4<<20)},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/repo/manifests/sid" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", ocispec.MediaTypeImageManifest)
		w.Header().Set("Docker-Content-Digest", digest.FromBytes(b).String())
		w.Header().Set("Content-Length", strconv.Itoa(len(b)))
		w.Write(b)
	}))
	defer srv.Close()

	repo, err := remote.NewRepository(strings.TrimPrefix(srv.URL, "http://") + "/repo")
	if err != nil {
		t.Fatal(err)
	}
	repo.PlainHTTP = true
	if _, err := buildFileMap(context.Background(), repo, "sid"); !errors.Is(err, errdef.ErrSizeExceedsLimit) {
		t.Fatalf("expected %v, got %v", errdef.ErrSizeExceedsLimit, err)
	}
}

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

// writeDockerConfig points DOCKER_CONFIG at config and returns its path.
func writeDockerConfig(t *testing.T, config string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", dir)
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	return configPath
}

// fakeHelper installs docker-credential-fake, which prints out and exits with code.
func fakeHelper(t *testing.T, out string, code int) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the credential helper is a shell script")
	}
	bin := t.TempDir()
	script := fmt.Sprintf("#!/bin/sh\ncat >/dev/null\nprintf '%%s' '%s'\nexit %d\n", out, code)
	if err := os.WriteFile(filepath.Join(bin, "docker-credential-fake"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestOpenCredentials(t *testing.T) {
	const want = "Basic dTpw" // u:p
	var requests int
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if gotAuth = r.Header.Get("Authorization"); gotAuth != want {
			w.Header().Set("WWW-Authenticate", `Basic realm="test"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "http://")
	ref, err := registry.ParseReference(host + "/repo:tag")
	if err != nil {
		t.Fatal(err)
	}
	auths := fmt.Sprintf(`{"auths":{%q:{"auth":"dTpw"}}}`, host)
	helped := fmt.Sprintf(`{"credHelpers":{%q:"fake"}}`, host)

	for name, tc := range map[string]struct {
		config  string
		helper  func(t *testing.T)
		wantErr string // "" sends credentials if any, else fails before any request
	}{
		"auths":     {config: auths},
		"helper":    {config: helped, helper: func(t *testing.T) { fakeHelper(t, `{"Username":"u","Secret":"p"}`, 0) }},
		"none":      {config: `{}`},
		"malformed": {config: `{"auths":`, wantErr: "config.json"},
		"helper fails": {config: helped, wantErr: "keyring is locked",
			helper: func(t *testing.T) { fakeHelper(t, "keyring is locked", 1) }},
	} {
		t.Run(name, func(t *testing.T) {
			requests, gotAuth = 0, ""
			configPath := writeDockerConfig(t, tc.config)
			if tc.helper != nil {
				tc.helper(t)
			}
			c := NewClient()
			// the method drops privileges, so the config must be read already
			if err := os.WriteFile(configPath, []byte(`{}`), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := c.Open(context.Background(), ref, time.Second)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) || requests != 0 {
					t.Errorf("expected an error with %q and no request, got %v after %d requests", tc.wantErr, err, requests)
				}
				return
			}
			if tc.config == `{}` {
				if gotAuth != "" {
					t.Errorf("expected no credentials, got %q", gotAuth)
				}
			} else if !errors.Is(err, errdef.ErrNotFound) || gotAuth != want {
				t.Errorf("expected %q to reach the registry, got %q and %v", want, gotAuth, err)
			}
		})
	}
}

// TestCredentialDockerHub finds the credentials `docker login` stores for docker.io refs.
func TestCredentialDockerHub(t *testing.T) {
	writeDockerConfig(t, `{"auths":{"https://index.docker.io/v1/":{"auth":"dTpw"}}}`)
	cred, err := NewClient().credential(context.Background(), "docker.io", time.Second)
	if err != nil || cred.Username != "u" || cred.Password != "p" {
		t.Errorf("expected u:p, got %+v, %v", cred, err)
	}
}

func TestDockerConfigPathWithoutHome(t *testing.T) {
	t.Setenv("DOCKER_CONFIG", "")
	t.Setenv("HOME", "")
	u, err := user.Current()
	if err != nil {
		t.Skip(err)
	}
	got, err := dockerConfigPath()
	if want := filepath.Join(u.HomeDir, ".docker", "config.json"); err != nil || got != want {
		t.Errorf("expected %q, got %q, %v", want, got, err)
	}
}

// TestOpenTimeout fails a registry that stops sending after the timeout given to Open.
func TestOpenTimeout(t *testing.T) {
	t.Setenv("DOCKER_CONFIG", t.TempDir())
	for name, stall := range map[string]func(w http.ResponseWriter){
		"headers": func(http.ResponseWriter) {},
		"body": func(w http.ResponseWriter) {
			w.Header().Set("Content-Type", ocispec.MediaTypeImageManifest)
			w.Header().Set("Content-Length", "100")
			w.Write([]byte("{"))
			w.(http.Flusher).Flush()
		},
	} {
		t.Run(name, func(t *testing.T) {
			done := make(chan struct{})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				stall(w)
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
			_, err = NewClient().Open(context.Background(), ref, 100*time.Millisecond)
			var netErr net.Error
			if !errors.As(err, &netErr) || !netErr.Timeout() || time.Since(start) > 5*time.Second {
				t.Errorf("expected a timeout soon after 100ms, got %v after %v", err, time.Since(start))
			}
		})
	}
}
