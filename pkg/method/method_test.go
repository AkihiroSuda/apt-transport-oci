package method

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AkihiroSuda/apt-transport-oci/pkg/apt"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/memory"
)

// TestTimeout looks up Acquire::oci::<host>::Timeout, then Acquire::oci::Timeout.
func TestTimeout(t *testing.T) {
	for name, tc := range map[string]struct {
		config   apt.Config
		registry string
		want     time.Duration
	}{
		"unset":             {apt.Config{}, "ghcr.io", defaultTimeout},
		"oci":               {apt.Config{"acquire::oci::timeout": "60"}, "ghcr.io", time.Minute},
		"host wins":         {apt.Config{"acquire::oci::timeout": "60", "acquire::oci::ghcr.io::timeout": "5"}, "ghcr.io", 5 * time.Second},
		"other host":        {apt.Config{"acquire::oci::quay.io::timeout": "5"}, "ghcr.io", defaultTimeout},
		"port ignored":      {apt.Config{"acquire::oci::localhost::timeout": "5"}, "localhost:5000", 5 * time.Second},
		"hex, as strtol":    {apt.Config{"acquire::oci::timeout": "0x1e"}, "ghcr.io", 30 * time.Second},
		"http ignored":      {apt.Config{"acquire::http::timeout": "5", "acquire::https::timeout": "5"}, "ghcr.io", defaultTimeout},
		"leading integer":   {apt.Config{"acquire::oci::timeout": "60s"}, "ghcr.io", time.Minute},
		"invalid host wins": {apt.Config{"acquire::oci::ghcr.io::timeout": "soon", "acquire::oci::timeout": "5"}, "ghcr.io", defaultTimeout},
	} {
		if got := timeout(tc.config, tc.registry); got != tc.want {
			t.Errorf("%s: expected %v, got %v", name, tc.want, got)
		}
	}
}

// fakeRegistry serves a memory.Store over the distribution API, and records
// the requests it gets.
type fakeRegistry struct {
	*memory.Store
	mu       sync.Mutex
	requests []string
}

// push stores b, tagged by its digest so that manifests and blobs resolve by
// digest as well as by tag.
func (f *fakeRegistry) push(t *testing.T, mediaType string, b []byte) ocispec.Descriptor {
	t.Helper()
	desc, err := oras.TagBytes(context.Background(), f.Store, mediaType, b, digest.FromBytes(b).String())
	if err != nil {
		t.Fatal(err)
	}
	return desc
}

func (f *fakeRegistry) manifestGETs() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, r := range f.requests {
		if strings.HasPrefix(r, "GET /v2/repo/manifests/") {
			n++
		}
	}
	return n
}

func (f *fakeRegistry) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, r.Method+" "+r.URL.Path)
	f.mu.Unlock()
	ref, ok := strings.CutPrefix(r.URL.Path, "/v2/repo/manifests/")
	if !ok {
		ref, _ = strings.CutPrefix(r.URL.Path, "/v2/repo/blobs/")
	}
	desc, err := f.Resolve(r.Context(), ref)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	b, err := content.FetchAll(r.Context(), f, desc)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", desc.MediaType)
	w.Header().Set("Docker-Content-Digest", desc.Digest.String())
	w.Header().Set("Content-Length", strconv.Itoa(len(b)))
	if r.Method != http.MethodHead {
		w.Write(b)
	}
}

// TestFetchByHashFirst fetches by hash, else maps the files once, by title.
func TestFetchByHashFirst(t *testing.T) {
	t.Setenv("DOCKER_CONFIG", t.TempDir())
	f := &fakeRegistry{Store: memory.New()}
	files := map[string][]byte{"dists/sid/InRelease": []byte("in-release"), "pool/main/h/hello_1_amd64.deb": []byte("deb")}
	var manifests []ocispec.Descriptor
	config := f.push(t, ocispec.MediaTypeEmptyJSON, []byte("{}"))
	for title, data := range files {
		layer := f.push(t, MediaTypeApplicationXBinary, data)
		layer.Annotations = map[string]string{ocispec.AnnotationTitle: title}
		b, _ := json.Marshal(ocispec.Manifest{Versioned: specs.Versioned{SchemaVersion: 2}, MediaType: ocispec.MediaTypeImageManifest, Config: config, Layers: []ocispec.Descriptor{layer}})
		manifests = append(manifests, f.push(t, ocispec.MediaTypeImageManifest, b))
	}
	b, _ := json.Marshal(ocispec.Index{Versioned: specs.Versioned{SchemaVersion: 2}, MediaType: ocispec.MediaTypeImageIndex, Manifests: manifests})
	if err := f.Tag(context.Background(), f.push(t, ocispec.MediaTypeImageIndex, b), "sid"); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(f)
	defer srv.Close()

	var out bytes.Buffer
	m := New(&out, strings.NewReader(""))
	if err := m.configure(&apt.Message{StatusCode: apt.CodeConfiguration}); err != nil {
		t.Fatal(err)
	}
	repoURI := "oci://" + strings.TrimPrefix(srv.URL, "http://") + "/repo:sid/"
	acquire := func(title string, hash []byte) string {
		t.Helper()
		out.Reset()
		fields := make(textproto.MIMEHeader)
		fields.Set(FieldURI, repoURI+title)
		fields.Set(FieldTargetRepoURI, repoURI)
		fields.Set(FieldFilename, filepath.Join(t.TempDir(), "file"))
		if hash != nil {
			fields.Set(FieldExpectedSHA256, digest.FromBytes(hash).Encoded())
			fields.Set(FieldExpectedFileSize, strconv.Itoa(len(hash)))
		}
		m.handleURIAcquire(context.Background(), &apt.Message{StatusCode: apt.CodeURIAcquire, Fields: fields})
		return out.String()
	}

	deb := files["pool/main/h/hello_1_amd64.deb"]
	if got := acquire("pool/main/h/hello_1_amd64.deb", deb); !strings.Contains(got, "201 URI Done") || f.manifestGETs() != 0 {
		t.Fatalf("by hash: expected URI Done and no manifest GET, got %d GETs and:\n%s", f.manifestGETs(), got)
	} else {
		// SHA-256 from the verified digest, SHA-512 computed
		for _, want := range []string{"SHA256-Hash: " + digest.SHA256.FromBytes(deb).Encoded(), "SHA512-Hash: " + digest.SHA512.FromBytes(deb).Encoded()} {
			if !strings.Contains(got, want) {
				t.Errorf("by hash: expected %q in:\n%s", want, got)
			}
		}
	}

	if got := acquire("dists/sid/InRelease", nil); !strings.Contains(got, "201 URI Done") {
		t.Fatalf("by title: expected URI Done, got:\n%s", got)
	}
	built := f.manifestGETs()
	if got := acquire("pool/main/h/hello_1_amd64.deb", nil); !strings.Contains(got, "201 URI Done") || f.manifestGETs() != built {
		t.Fatalf("mapped once: expected no more manifest GETs than %d, got %d and:\n%s", built, f.manifestGETs(), got)
	}
	// no blob has the hash, so the title is used
	if got := acquire("pool/main/h/hello_1_amd64.deb", []byte("other")); !strings.Contains(got, "201 URI Done") || f.manifestGETs() != built {
		t.Fatalf("hash miss: expected URI Done by title and no more manifest GETs than %d, got %d and:\n%s", built, f.manifestGETs(), got)
	}
	if got := acquire("pool/main/m/missing_1_amd64.deb", []byte("missing")); !strings.Contains(got, "FailReason: HttpError404") {
		t.Fatalf("missing: expected HttpError404, got:\n%s", got)
	}
}
