package registry

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const testTimeout = 200 * time.Millisecond

func isTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func TestTimeoutStalledBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Send data past the timeout, then stall.
		for range 4 {
			w.Write([]byte("x"))
			w.(http.Flusher).Flush()
			time.Sleep(testTimeout / 2)
		}
		<-r.Context().Done()
	}))
	defer srv.Close()

	resp, err := newClient(testTimeout).Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if !isTimeout(err) {
		t.Fatalf("expected a timeout, got %v", err)
	}
	if len(b) != 4 {
		t.Fatalf("expected 4 bytes before the timeout, got %d", len(b))
	}
}

// TestTimeoutStalledStreamHTTP2 stalls one request while another keeps the
// shared HTTP/2 connection busy.
func TestTimeoutStalledStreamHTTP2(t *testing.T) {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("x"))
		w.(http.Flusher).Flush()
		if r.URL.Path == "/stall" {
			<-r.Context().Done()
			return
		}
		for range 12 {
			time.Sleep(testTimeout / 4)
			w.Write([]byte("x"))
			w.(http.Flusher).Flush()
		}
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()

	client := newClient(testTimeout)
	client.Transport.(*stallTransport).next.(*http.Transport).TLSClientConfig = srv.Client().Transport.(*http.Transport).TLSClientConfig
	busy, err := client.Get(srv.URL + "/busy")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Body.Close()
	go io.Copy(io.Discard, busy.Body)

	start := time.Now()
	stall, err := client.Get(srv.URL + "/stall")
	if err != nil {
		t.Fatal(err)
	}
	defer stall.Body.Close()
	if stall.ProtoMajor != 2 {
		t.Fatalf("expected HTTP/2, got %s", stall.Proto)
	}
	if _, err := io.ReadAll(stall.Body); !isTimeout(err) {
		t.Fatalf("expected a timeout, got %v", err)
	}
	if elapsed := time.Since(start); elapsed >= 2*testTimeout {
		t.Fatalf("stalled stream took %v to time out", elapsed)
	}
}

func TestTimeoutNoResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	if _, err := newClient(testTimeout).Get(srv.URL); !isTimeout(err) {
		t.Fatalf("expected a timeout, got %v", err)
	}
}
