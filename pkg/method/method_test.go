package method

import (
	"testing"

	"github.com/AkihiroSuda/apt-transport-oci/pkg/apt"
)

func TestExpectedBlob(t *testing.T) {
	// Digests of the empty file
	const (
		sha256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
		sha512 = "cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e"
	)

	type testCase struct {
		v       map[string]string
		xDigest string // empty if no blob is expected
	}

	for name, tc := range map[string]testCase{
		"sha256": {
			v:       map[string]string{FieldExpectedSHA256: sha256, FieldExpectedFileSize: "0"},
			xDigest: "sha256:" + sha256,
		},
		"sha512": {
			v:       map[string]string{FieldExpectedSHA512: sha512, FieldExpectedFileSize: "0"},
			xDigest: "sha512:" + sha512,
		},
		"sha256 preferred over sha512": {
			v:       map[string]string{FieldExpectedSHA256: sha256, FieldExpectedSHA512: sha512, FieldExpectedFileSize: "0"},
			xDigest: "sha256:" + sha256,
		},
		"missing hash": {
			v: map[string]string{FieldExpectedFileSize: "0"},
		},
		"missing size": {
			v: map[string]string{FieldExpectedSHA256: sha256},
		},
		"malformed hash": {
			v: map[string]string{FieldExpectedSHA256: "../../x", FieldExpectedFileSize: "0"},
		},
		"negative size": {
			v: map[string]string{FieldExpectedSHA256: sha256, FieldExpectedFileSize: "-1"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			desc, ok := expectedBlob(&apt.Message{Fields: tc.v})
			if ok != (tc.xDigest != "") {
				t.Fatalf("expected ok=%v, got %v", tc.xDigest != "", ok)
			}

			if desc.Digest.String() != tc.xDigest {
				t.Fatalf("expected digest %q, got %q", tc.xDigest, desc.Digest)
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
