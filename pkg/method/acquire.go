package method

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/AkihiroSuda/apt-transport-oci/pkg/apt"
	"github.com/opencontainers/go-digest"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/errdef"
)

func (m *Method) acquire(ctx context.Context, msg *apt.Message) error {
	uri := msg.Fields.Get(FieldURI)
	filename := msg.Fields.Get(FieldFilename)

	ociRef, title, err := parseURIFields(msg)
	if err != nil {
		return err
	}
	repo, err := m.open(ctx, uri, ociRef)
	if err != nil {
		return err
	}
	desc, ok := repo.Files[title]
	if !ok {
		desc, ok = expectedBlob(msg)
	}
	if !ok {
		return fmt.Errorf("%q in %q: %w", title, ociRef, errdef.ErrNotFound)
	}
	if limit := maximumSize(msg); limit > 0 && desc.Size > limit {
		return fmt.Errorf("%q: %w (%d > %d)", title, errdef.ErrSizeExceedsLimit, desc.Size, limit)
	}
	switch desc.MediaType {
	case MediaTypeApplicationOctetStream, MediaTypeApplicationXBinary:
	default:
		m.w.WriteMessage(apt.CodeWarning, apt.Field{Key: FieldMessage,
			Value: fmt.Sprintf("expected media type of %q to be %q, got %q", title, MediaTypeApplicationXBinary, desc.MediaType)})
	}

	m.w.WriteMessage(apt.CodeURIStart,
		apt.Field{Key: FieldURI, Value: uri},
		apt.Field{Key: FieldSize, Value: strconv.FormatInt(desc.Size, 10)},
	)
	r, err := repo.Fetch(ctx, desc)
	if err != nil {
		return err
	}
	defer r.Close()
	w, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer w.Close()

	sha256, sha512 := digest.SHA256.Digester(), digest.SHA512.Digester()
	vr := content.NewVerifyReader(r, desc)
	if _, err := io.Copy(io.MultiWriter(w, sha256.Hash(), sha512.Hash()), vr); err != nil {
		return fmt.Errorf("blob of %q: %w", title, err)
	}
	if err := vr.Verify(); err != nil {
		return fmt.Errorf("blob of %q: %w", title, err)
	}
	if err := w.Close(); err != nil {
		return err
	}

	m.w.WriteMessage(apt.CodeURIDone,
		apt.Field{Key: FieldURI, Value: uri},
		apt.Field{Key: FieldFilename, Value: filename},
		apt.Field{Key: FieldSize, Value: strconv.FormatInt(desc.Size, 10)},
		apt.Field{Key: FieldSHA256Hash, Value: sha256.Digest().Encoded()},
		apt.Field{Key: FieldSHA512Hash, Value: sha512.Digest().Encoded()},
	)
	return nil
}
