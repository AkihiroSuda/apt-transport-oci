package method

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/AkihiroSuda/apt-transport-oci/pkg/apt"
	"github.com/AkihiroSuda/apt-transport-oci/pkg/registry"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/errdef"
)

// acquire downloads the file msg asks for to its Filename.
func (m *Method) acquire(ctx context.Context, msg *apt.Message) error {
	uri := msg.Fields.Get(FieldURI)
	ref, title, err := parseURIFields(msg)
	if err != nil {
		return err
	}
	repo, err := m.open(ctx, uri, ref)
	if err != nil {
		return err
	}
	blob, body, err := m.fetch(ctx, repo, msg, title)
	if err != nil {
		return fmt.Errorf("%q in %q: %w", title, ref, err)
	}
	defer body.Close()
	if limit := maximumSize(msg); limit > 0 && blob.Size > limit {
		return fmt.Errorf("%q: %w (%d > %d)", title, errdef.ErrSizeExceedsLimit, blob.Size, limit)
	}

	sizeField := apt.Field{Key: FieldSize, Value: strconv.FormatInt(blob.Size, 10)}
	m.w.WriteMessage(apt.CodeURIStart, apt.Field{Key: FieldURI, Value: uri}, sizeField)
	filename := msg.Fields.Get(FieldFilename)
	sha256, sha512, err := download(body, blob, filename)
	if err != nil {
		return fmt.Errorf("blob of %q: %w", title, err)
	}
	m.w.WriteMessage(apt.CodeURIDone,
		apt.Field{Key: FieldURI, Value: uri},
		apt.Field{Key: FieldFilename, Value: filename},
		sizeField,
		apt.Field{Key: FieldSHA256Hash, Value: sha256.Encoded()},
		apt.Field{Key: FieldSHA512Hash, Value: sha512.Encoded()},
	)
	return nil
}

// fetch opens the file by apt's hash, else, or if no blob has it, by title.
func (m *Method) fetch(ctx context.Context, repo *registry.Repo, msg *apt.Message, title string) (ocispec.Descriptor, io.ReadCloser, error) {
	if expected, ok := expectedDigest(msg, repo.Algorithm()); ok {
		// oras-go describes the blob from the response
		blob, body, err := repo.FetchReference(ctx, expected.String())
		if !errors.Is(err, errdef.ErrNotFound) {
			return blob, body, err
		}
	}
	blob, ok, err := repo.Lookup(ctx, title)
	if err != nil {
		return blob, nil, err
	}
	if !ok {
		return blob, nil, errdef.ErrNotFound
	}
	switch blob.MediaType {
	case MediaTypeApplicationOctetStream, MediaTypeApplicationXBinary:
	default:
		m.w.WriteMessage(apt.CodeWarning, apt.Field{Key: FieldMessage,
			Value: fmt.Sprintf("expected media type of %q to be %q, got %q", title, MediaTypeApplicationXBinary, blob.MediaType)})
	}
	body, err := repo.Fetch(ctx, blob)
	return blob, body, err
}

// download writes body to filename, verified against blob, and returns its
// SHA-256 and SHA-512.
func download(body io.Reader, blob ocispec.Descriptor, filename string) (sha256, sha512 digest.Digest, err error) {
	file, err := os.Create(filename)
	if err != nil {
		return "", "", err
	}
	defer file.Close()
	sha256Hash, sha512Hash := digest.SHA256.Digester(), digest.SHA512.Digester()
	verified := content.NewVerifyReader(body, blob)
	if _, err := io.Copy(io.MultiWriter(file, sha256Hash.Hash(), sha512Hash.Hash()), verified); err != nil {
		return "", "", err
	}
	if err := verified.Verify(); err != nil {
		return "", "", err
	}
	if err := file.Close(); err != nil {
		return "", "", err
	}
	return sha256Hash.Digest(), sha512Hash.Digest(), nil
}
