package method

import (
	"errors"
	"fmt"
	"net"
	"slices"
	"syscall"

	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/errdef"
	"oras.land/oras-go/v2/registry/remote/errcode"
	"oras.land/oras-go/v2/registry/remote/retry"
)

// transientCodes are the HTTP statuses apt retries.
var transientCodes = []int{408, 429, 500, 502, 503, 504, 599}

// failure returns the FailReason of err and whether apt should retry, as
// apt's basehttp.cc and connect.cc.
func failure(err error) (reason string, transient bool) {
	var resp *errcode.ErrorResponse
	var dnsErr *net.DNSError
	var netErr net.Error
	var opErr *net.OpError
	switch {
	case errors.Is(err, content.ErrMismatchedDigest):
		return "HashSumMismatch", false
	case errors.Is(err, errdef.ErrSizeExceedsLimit):
		return "MaximumSizeExceeded", false
	case errors.Is(err, errdef.ErrNotFound):
		return "HttpError404", false
	case errors.As(err, &resp):
		return fmt.Sprintf("HttpError%d", resp.StatusCode), slices.Contains(transientCodes, resp.StatusCode)
	case errors.As(err, &dnsErr):
		if dnsErr.IsTemporary || dnsErr.IsTimeout {
			return "TmpResolveFailure", true
		}
		return "ResolveFailure", true
	case errors.Is(err, syscall.ECONNREFUSED):
		return "ConnectionRefused", true
	case errors.Is(err, syscall.ETIMEDOUT):
		return "ConnectionTimedOut", true
	case errors.As(err, &opErr) && opErr.Op == "dial" && opErr.Timeout():
		return "Timeout", true
	// apt retries any connection error; DefaultPredicate misses a wrapped timeout
	case errors.As(err, &netErr) && netErr.Timeout(), errors.As(err, &opErr):
		return "", true
	}
	retryable, _ := retry.DefaultPredicate(nil, err)
	return "", retryable
}
