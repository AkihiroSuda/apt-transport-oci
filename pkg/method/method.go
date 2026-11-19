package method

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/AkihiroSuda/apt-transport-oci/pkg/apt"
	"github.com/AkihiroSuda/apt-transport-oci/pkg/registry"
	"github.com/AkihiroSuda/apt-transport-oci/pkg/version"
	"golang.org/x/sync/singleflight"
	orasregistry "oras.land/oras-go/v2/registry"
)

// Protocol: see https://justi.cz/security/2019/01/22/apt-rce.html
// See also the output of `apt-get -o Debug::pkgAcquire::Worker=1 update`
const (
	FieldURI              = "URI"
	FieldMessage          = "Message"
	FieldFailReason       = "FailReason"
	FieldTransientFailure = "Transient-Failure"
	FieldTargetRepoURI    = "Target-Repo-URI"
	FieldFilename         = "Filename"
	FieldSize             = "Size"
	FieldSHA256Hash       = "SHA256-Hash"
	FieldExpectedSHA256   = "Expected-SHA256"
	FieldSHA512Hash       = "SHA512-Hash"
	FieldExpectedSHA512   = "Expected-SHA512"
	FieldExpectedFileSize = "Expected-Checksum-FileSize"
	FieldMaximumSize      = "Maximum-Size"
)

// defaultTimeout is apt's default for Acquire::<method>::Timeout.
const defaultTimeout = 30 * time.Second

const (
	MediaTypeApplicationXBinary     = "application/x-binary"
	MediaTypeApplicationOctetStream = "application/octet-stream"
)

func New(out io.Writer, in io.Reader) *Method {
	return &Method{
		w: apt.NewMessageWriter(out),
		r: apt.NewMessageReader(in),
	}
}

type Method struct {
	w *apt.MessageWriter
	r *apt.MessageReader

	// set by configure
	config apt.Config
	client *registry.Client

	// repos caches opened refs, so a run sees one snapshot of each.
	repos sync.Map // string -> *registry.Repo
	// opening merges concurrent opens; failures are not cached, so apt can retry.
	opening singleflight.Group
}

// Run is based on https://github.com/cloudflare/apt-transport-cloudflared/blob/96e1417f9c54/apt/method.go#L77-L108
func (m *Method) Run(ctx context.Context) {
	version := fmt.Sprintf("%d.%d", version.Major, version.Minor)
	m.w.WriteMessage(apt.CodeCapabilities,
		apt.Field{Key: "Version", Value: version},
		apt.Field{Key: "Pipeline", Value: "true"},
		apt.Field{Key: "Send-Config", Value: "true"},
	)
	var requests sync.WaitGroup
	defer requests.Wait()
	for {
		msg, err := m.r.ReadMessage()
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) {
			return
		}
		if err != nil {
			m.w.WriteMessage(apt.CodeGeneralFailure,
				apt.Field{Key: FieldMessage, Value: fmt.Sprintf("Error reading message: %v", err)})
			return
		}
		switch msg.StatusCode {
		case apt.CodeConfiguration:
			// Sent once, before any URI Acquire.
			if err := m.configure(msg); err != nil {
				m.w.WriteMessage(apt.CodeGeneralFailure, apt.Field{Key: FieldMessage, Value: err.Error()})
				return
			}
		case apt.CodeURIAcquire:
			requests.Go(func() { m.handleURIAcquire(ctx, msg) })
		default:
			m.w.WriteMessage(apt.CodeLog,
				apt.Field{Key: FieldMessage, Value: fmt.Sprintf("Unknown message: %d %s", msg.StatusCode, msg.Description)})
		}
	}
}

func (m *Method) handleURIAcquire(ctx context.Context, msg *apt.Message) {
	err := m.acquire(ctx, msg)
	if err == nil {
		return
	}
	reason, transient := failure(err)
	fields := []apt.Field{
		{Key: FieldURI, Value: msg.Fields.Get(FieldURI)},
		{Key: FieldMessage, Value: err.Error()},
		{Key: FieldFailReason, Value: reason},
	}
	if transient {
		fields = append(fields, apt.Field{Key: FieldTransientFailure, Value: "true"})
	}
	m.w.WriteMessage(apt.CodeURIFailure, fields...)
}

// configure reads the configuration and credentials, then drops privileges.
func (m *Method) configure(msg *apt.Message) error {
	m.config = apt.ParseConfig(msg)
	m.client = registry.NewClient()
	return apt.DropPrivileges(m.config)
}

// timeout is Acquire::oci::<host>::Timeout if set, else Acquire::oci::Timeout,
// in seconds, as apt's ConfigFindI. Unset or not positive is defaultTimeout.
func timeout(config apt.Config, registry string) time.Duration {
	host := registry
	if h, _, err := net.SplitHostPort(registry); err == nil {
		host = h
	}
	key := "Acquire::oci::" + host + "::Timeout"
	if config.Find(key, "") == "" {
		key = "Acquire::oci::Timeout"
	}
	if seconds := config.FindI(key, 0); seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return defaultTimeout
}

// open returns the repo of ref, opening it on first use.
func (m *Method) open(ctx context.Context, uri string, ref orasregistry.Reference) (*registry.Repo, error) {
	key := ref.String()
	if repo, ok := m.repos.Load(key); ok {
		return repo.(*registry.Repo), nil
	}
	if m.client == nil {
		return nil, errors.New("no configuration from apt")
	}
	repo, err, _ := m.opening.Do(key, func() (any, error) {
		if repo, ok := m.repos.Load(key); ok {
			return repo, nil
		}
		m.w.WriteMessage(apt.CodeStatus,
			apt.Field{Key: FieldURI, Value: uri},
			apt.Field{Key: FieldMessage, Value: "Opening " + key},
		)
		repo, err := m.client.Open(ctx, ref, timeout(m.config, ref.Registry))
		if err != nil {
			return nil, err
		}
		m.repos.Store(key, repo)
		return repo, nil
	})
	if err != nil {
		return nil, err
	}
	return repo.(*registry.Repo), nil
}
