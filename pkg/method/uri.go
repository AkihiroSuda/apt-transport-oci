package method

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/AkihiroSuda/apt-transport-oci/pkg/apt"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
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

	// Combine everything up to but not including the tag
	// We don't quite have the tag yet because it (should) have a file path added
	// to the end that we need to split off first.
	repo = "oci://" + trimmed[:i+1]
	tagAndFile := strings.SplitN(trimmed[i+1:], "/", 2)

	// We add "/" here, otherwise we'll end up with an extra preceding "/" on the
	// file name later which we don't want and will cause the file to not be found.
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

func expectedBlob(msg *apt.Message) (ocispec.Descriptor, bool) {
	size, err := strconv.ParseInt(msg.Fields.Get(FieldExpectedFileSize), 10, 64)
	if err != nil || size < 0 {
		return ocispec.Descriptor{}, false
	}
	for _, dgst := range []digest.Digest{
		digest.NewDigestFromEncoded(digest.SHA256, msg.Fields.Get(FieldExpectedSHA256)),
		digest.NewDigestFromEncoded(digest.SHA512, msg.Fields.Get(FieldExpectedSHA512)),
	} {
		if dgst.Validate() == nil {
			return ocispec.Descriptor{
				MediaType: MediaTypeApplicationOctetStream,
				Digest:    dgst,
				Size:      size,
			}, true
		}
	}
	return ocispec.Descriptor{}, false
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
