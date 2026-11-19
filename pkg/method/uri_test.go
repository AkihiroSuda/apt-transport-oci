package method

import (
	"net/textproto"
	"testing"

	"github.com/AkihiroSuda/apt-transport-oci/pkg/apt"
	"github.com/opencontainers/go-digest"
)

func TestExpectedDigest(t *testing.T) {
	// Digests of the empty file
	const (
		sha256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
		sha512 = "cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e"
	)

	type testCase struct {
		v       map[string]string
		alg     digest.Algorithm // the registry's
		xDigest string           // empty if apt sent no usable hash
	}

	both := map[string]string{FieldExpectedSHA256: sha256, FieldExpectedSHA512: sha512}
	for name, tc := range map[string]testCase{
		"sha256":          {map[string]string{FieldExpectedSHA256: sha256}, digest.SHA256, "sha256:" + sha256},
		"sha512":          {map[string]string{FieldExpectedSHA512: sha512}, digest.SHA512, "sha512:" + sha512},
		"both, sha256":    {both, digest.SHA256, "sha256:" + sha256},
		"both, sha512":    {both, digest.SHA512, "sha512:" + sha512},
		"sha512 only":     {map[string]string{FieldExpectedSHA512: sha512}, digest.SHA256, ""},
		"other algorithm": {both, digest.Algorithm("blake3"), ""},
		"missing hash":    {map[string]string{FieldExpectedFileSize: "0"}, digest.SHA256, ""},
		"malformed hash":  {map[string]string{FieldExpectedSHA256: "../../x"}, digest.SHA256, ""},
	} {
		t.Run(name, func(t *testing.T) {
			fields := textproto.MIMEHeader{}
			for key, value := range tc.v {
				fields.Set(key, value)
			}
			dgst, ok := expectedDigest(&apt.Message{Fields: fields}, tc.alg)
			if ok != (tc.xDigest != "") || (ok && dgst.String() != tc.xDigest) {
				t.Fatalf("expected %q, got %q (ok=%v)", tc.xDigest, dgst, ok)
			}
		})
	}
}

func TestParseURI(t *testing.T) {
	type testCase struct {
		v     string
		xRepo string
		xPath string
		err   bool
	}

	for name, uri := range map[string]testCase{
		"no proto just host": {
			v:   "foo.bar",
			err: true,
		},
		"no proto host and path": {
			v:   "foo.bar/baz",
			err: true,
		},
		"no proto host and tag": {
			v:   "foo.bar:latest",
			err: true,
		},
		"with proto just host": {
			v:   "oci://foo.bar",
			err: true,
		},
		"with proto host and path": {
			v:   "oci://foo.bar/baz",
			err: true,
		},
		"with proto host and tag": {
			v:     "oci://foo.bar:latest/",
			xRepo: "oci://foo.bar:latest/",
		},
		"with proto host, tag, and path": {
			v:     "oci://foo.bar:latest/File",
			xRepo: "oci://foo.bar:latest/",
			xPath: "File",
		},
		"with proto host, tag, and nested path": {
			v:     "oci://foo.bar:latest/Nested/File",
			xRepo: "oci://foo.bar:latest/",
			xPath: "Nested/File",
		},
		"with proto host, namespace, tag, and path": {
			v:     "oci://foo.bar/namespace:latest/File",
			xRepo: "oci://foo.bar/namespace:latest/",
			xPath: "File",
		},
		"with proto host, namespace, tag, and nested path": {
			v:     "oci://foo.bar/namespace:latest/Nested/File",
			xRepo: "oci://foo.bar/namespace:latest/",
			xPath: "Nested/File",
		},
		"with proto host, port, namespace, tag, and nested path": {
			v:     "oci://localhost:5000/namespace:latest/Nested/File",
			xRepo: "oci://localhost:5000/namespace:latest/",
			xPath: "Nested/File",
		},
	} {
		t.Run(name, func(t *testing.T) {
			uri := uri

			repo, path, err := parseURI(uri.v)
			if uri.err {
				if err == nil {
					t.Fatal("expected error")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
			}

			if repo != uri.xRepo {
				t.Fatalf("expected repo %q, got %q", uri.xRepo, repo)
			}

			if path != uri.xPath {
				t.Fatalf("expected path %q, got %q", uri.xPath, path)
			}
		})
	}
}

func TestMaximumSize(t *testing.T) {
	for name, tc := range map[string]struct {
		v     map[string]string
		xSize int64
	}{
		"none":          {map[string]string{}, 0},
		"maximum":       {map[string]string{FieldMaximumSize: "10000000"}, 10000000},
		"expected":      {map[string]string{FieldExpectedFileSize: "42"}, 42},
		"expected wins": {map[string]string{FieldExpectedFileSize: "42", FieldMaximumSize: "10000000"}, 42},
		"zero expected": {map[string]string{FieldExpectedFileSize: "0", FieldMaximumSize: "10000000"}, 10000000},
		"invalid":       {map[string]string{FieldMaximumSize: "big"}, 0},
	} {
		t.Run(name, func(t *testing.T) {
			fields := textproto.MIMEHeader{}
			for key, value := range tc.v {
				fields.Set(key, value)
			}
			if size := maximumSize(&apt.Message{Fields: fields}); size != tc.xSize {
				t.Fatalf("expected %d, got %d", tc.xSize, size)
			}
		})
	}
}

// TestParseURIFieldsReference checks references are taken verbatim.
func TestParseURIFieldsReference(t *testing.T) {
	for repo, want := range map[string]string{
		"oci://ghcr.io/user/repo:tag/":                    "ghcr.io/user/repo:tag",
		"oci://localhost:5000/repo:tag/":                  "localhost:5000/repo:tag",
		"oci://docker.io/library/ubuntu:noble/":           "docker.io/library/ubuntu:noble",
		"oci://docker.io/ubuntu:noble/":                   "docker.io/ubuntu:noble",
		"oci://ubuntu:noble/":                             "",
		"oci://ghcr.io/user/repo@sha256:" + zeros64 + "/": "ghcr.io/user/repo@sha256:" + zeros64,
	} {
		fields := textproto.MIMEHeader{}
		fields.Set(FieldURI, repo+"dists/noble/InRelease")
		fields.Set(FieldTargetRepoURI, repo)
		ref, title, err := parseURIFields(&apt.Message{Fields: fields})
		if want == "" {
			if err == nil {
				t.Errorf("%s: expected an error, got %q", repo, ref)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: %v", repo, err)
		}
		if got := ref.String(); got != want {
			t.Errorf("%s: expected %q, got %q", repo, want, got)
		}
		if title != "dists/noble/InRelease" {
			t.Errorf("%s: expected title %q, got %q", repo, "dists/noble/InRelease", title)
		}
	}
}

const zeros64 = "0000000000000000000000000000000000000000000000000000000000000000"
