package method

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/AkihiroSuda/apt-transport-oci/pkg/apt"
	"github.com/opencontainers/go-digest"
	orasregistry "oras.land/oras-go/v2/registry"
)

const scheme = "oci://"

func parseURI(uri string) (repo, path string, _ error) {
	// The format here would be something like: registry.somehost.com/some/repo:tag/SomeFile

	trimmed := strings.TrimPrefix(uri, scheme)
	if trimmed == uri {
		return "", "", fmt.Errorf("missing %s protocol in uri", scheme)
	}

	// The tag follows the last colon; a port precedes it.
	i := strings.LastIndex(trimmed, ":")
	if i < 0 {
		return "", "", errors.New("uri is missing repo tag")
	}

	// The path follows the tag; the repo keeps the "/" between them.
	tagAndFile := strings.SplitN(trimmed[i+1:], "/", 2)
	repo = scheme + trimmed[:i+1] + tagAndFile[0] + "/"
	if len(tagAndFile) > 1 {
		path = tagAndFile[1]
	}

	return repo, path, nil
}

// parseURIFields returns the reference of msg and the title of its file.
func parseURIFields(msg *apt.Message) (ociRef orasregistry.Reference, title string, err error) {
	repoURI := msg.Fields.Get(FieldTargetRepoURI)
	if repoURI == "" {
		uri := msg.Fields.Get(FieldURI)
		if uri == "" {
			return ociRef, "", fmt.Errorf("missing fields %q and %q", FieldTargetRepoURI, FieldURI)
		}
		repoURI, _, err = parseURI(uri)
		if err != nil {
			return ociRef, "", err
		}
	}
	if !strings.HasPrefix(repoURI, scheme) {
		return ociRef, "", fmt.Errorf("field %s lacks %q prefix: %q", FieldTargetRepoURI, scheme, repoURI)
	}
	refTmp := strings.TrimPrefix(repoURI, scheme)
	refTmp = strings.TrimSuffix(refTmp, "/")
	ociRef, err = orasregistry.ParseReference(refTmp)
	if err != nil {
		return ociRef, "", fmt.Errorf("failed to parse %q (%s=%q) as OCI reference: %w", refTmp, FieldTargetRepoURI, repoURI, err)
	}
	title = strings.TrimPrefix(msg.Fields.Get(FieldURI), repoURI)
	// not robust, but no security issue (cuz not referring to the actual filesystem)
	title = strings.TrimPrefix(title, "./")
	return ociRef, title, nil
}

// expectedFields are the fields of the hashes apt expects, by algorithm.
var expectedFields = map[digest.Algorithm]string{
	digest.SHA256: FieldExpectedSHA256,
	digest.SHA512: FieldExpectedSHA512,
}

// expectedDigest is apt's hash in alg, the registry's algorithm.
func expectedDigest(msg *apt.Message, alg digest.Algorithm) (digest.Digest, bool) {
	field, ok := expectedFields[alg]
	if !ok {
		return "", false
	}
	expected := digest.NewDigestFromEncoded(alg, msg.Fields.Get(field))
	return expected, expected.Validate() == nil
}

// maximumSize is the size apt allows for the file, or 0 for none, as apt's
// acquire-method.cc.
func maximumSize(msg *apt.Message) int64 {
	for _, field := range []string{FieldExpectedFileSize, FieldMaximumSize} {
		if size, err := strconv.ParseInt(msg.Fields.Get(field), 10, 64); err == nil && size > 0 {
			return size
		}
	}
	return 0
}
