package method

import "testing"

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
