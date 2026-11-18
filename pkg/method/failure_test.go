package method

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"syscall"
	"testing"

	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/errdef"
	"oras.land/oras-go/v2/registry/remote/errcode"
)

func TestFailure(t *testing.T) {
	for name, tc := range map[string]struct {
		err        error
		xReason    string
		xTransient bool
	}{
		"mismatched digest": {fmt.Errorf("blob: %w", content.ErrMismatchedDigest), "HashSumMismatch", false},
		"too large":         {fmt.Errorf("%w (2 > 1)", errdef.ErrSizeExceedsLimit), "MaximumSizeExceeded", false},
		"large manifest":    {fmt.Errorf("manifest: %w", errdef.ErrSizeExceedsLimit), "MaximumSizeExceeded", false},
		"not found":         {fmt.Errorf("ref: %w", errdef.ErrNotFound), "HttpError404", false},
		"401":               {&errcode.ErrorResponse{StatusCode: 401}, "HttpError401", false},
		"429":               {&errcode.ErrorResponse{StatusCode: 429}, "HttpError429", true},
		"503":               {fmt.Errorf("get: %w", &errcode.ErrorResponse{StatusCode: 503}), "HttpError503", true},
		"no such host":      {&net.DNSError{Err: "no such host", IsNotFound: true}, "ResolveFailure", true},
		"temporary dns":     {&net.DNSError{Err: "try again", IsTemporary: true}, "TmpResolveFailure", true},
		"refused":           {&net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}, "ConnectionRefused", true},
		"connect timed out": {&net.OpError{Op: "dial", Err: syscall.ETIMEDOUT}, "ConnectionTimedOut", true},
		"connect timeout":   {fmt.Errorf("get: %w", &net.OpError{Op: "dial", Err: os.ErrDeadlineExceeded}), "Timeout", true},
		"stalled":           {fmt.Errorf("blob: %w", os.ErrDeadlineExceeded), "", true},
		"reset":             {fmt.Errorf("blob: %w", &net.OpError{Op: "read", Err: syscall.ECONNRESET}), "", true},
		"unreachable":       {fmt.Errorf("get: %w", &net.OpError{Op: "dial", Err: syscall.ENETUNREACH}), "", true},
		"truncated":         {fmt.Errorf("blob: %w", io.ErrUnexpectedEOF), "", true},
		"bad certificate":   {&url.Error{Op: "Get", Err: &tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}}}, "", false},
		"other":             {fmt.Errorf("other"), "", false},
	} {
		t.Run(name, func(t *testing.T) {
			reason, transient := failure(tc.err)
			if reason != tc.xReason || transient != tc.xTransient {
				t.Fatalf("expected (%q, %v), got (%q, %v)", tc.xReason, tc.xTransient, reason, transient)
			}
		})
	}
}
