package method

import (
	"bytes"
	"context"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AkihiroSuda/apt-transport-oci/pkg/apt"
)

// TestMalformedDockerConfig fails the request with the config's error.
func TestMalformedDockerConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", dir)
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"auths":`), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	m := New(&out, strings.NewReader(""))
	if err := m.configure(&apt.Message{StatusCode: apt.CodeConfiguration}); err != nil {
		t.Fatal(err)
	}
	repoURI := "oci://127.0.0.1:1/repo:tag/"
	fields := make(textproto.MIMEHeader)
	fields.Set(FieldURI, repoURI+"InRelease")
	fields.Set(FieldTargetRepoURI, repoURI)
	fields.Set(FieldFilename, filepath.Join(t.TempDir(), "InRelease"))
	m.handleURIAcquire(context.Background(), &apt.Message{StatusCode: apt.CodeURIAcquire, Fields: fields})
	got := out.String()
	if !strings.Contains(got, "400 URI Failure") || !strings.Contains(got, configPath) || strings.Contains(got, "104 Warning") {
		t.Errorf("expected a URI Failure naming %s and no warning, got:\n%s", configPath, got)
	}
}

// TestTimeout looks up Acquire::oci::<host>::Timeout, then Acquire::oci::Timeout.
func TestTimeout(t *testing.T) {
	for name, tc := range map[string]struct {
		config   apt.Config
		registry string
		want     time.Duration
	}{
		"unset":             {apt.Config{}, "ghcr.io", defaultTimeout},
		"oci":               {apt.Config{"acquire::oci::timeout": "60"}, "ghcr.io", time.Minute},
		"host wins":         {apt.Config{"acquire::oci::timeout": "60", "acquire::oci::ghcr.io::timeout": "5"}, "ghcr.io", 5 * time.Second},
		"other host":        {apt.Config{"acquire::oci::quay.io::timeout": "5"}, "ghcr.io", defaultTimeout},
		"port ignored":      {apt.Config{"acquire::oci::localhost::timeout": "5"}, "localhost:5000", 5 * time.Second},
		"hex, as strtol":    {apt.Config{"acquire::oci::timeout": "0x1e"}, "ghcr.io", 30 * time.Second},
		"http ignored":      {apt.Config{"acquire::http::timeout": "5", "acquire::https::timeout": "5"}, "ghcr.io", defaultTimeout},
		"leading integer":   {apt.Config{"acquire::oci::timeout": "60s"}, "ghcr.io", time.Minute},
		"invalid host wins": {apt.Config{"acquire::oci::ghcr.io::timeout": "soon", "acquire::oci::timeout": "5"}, "ghcr.io", defaultTimeout},
	} {
		if got := timeout(tc.config, tc.registry); got != tc.want {
			t.Errorf("%s: expected %v, got %v", name, tc.want, got)
		}
	}
}
