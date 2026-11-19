package registry

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/memory"
	"oras.land/oras-go/v2/errdef"
)

const mediaTypeOctetStream = "application/octet-stream"

func push(t *testing.T, store *memory.Store, mediaType string, b []byte) ocispec.Descriptor {
	t.Helper()
	desc, err := oras.PushBytes(context.Background(), store, mediaType, b)
	if err != nil {
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

			files, err := buildFileMap(context.Background(), store, root)
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
	root := pushJSON(t, store, ocispec.MediaTypeImageManifest, ocispec.Manifest{})
	if _, err := buildFileMap(context.Background(), tamperedTarget{store}, root); err == nil {
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
	for name, root := range map[string]ocispec.Descriptor{"root": big, "child": index} {
		t.Run(name, func(t *testing.T) {
			_, err := buildFileMap(context.Background(), store, root)
			if !errors.Is(err, errdef.ErrSizeExceedsLimit) {
				t.Fatalf("expected %v, got %v", errdef.ErrSizeExceedsLimit, err)
			}
		})
	}
}
