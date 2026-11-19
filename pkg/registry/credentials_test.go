package registry

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"oras.land/oras-go/v2/errdef"
	"oras.land/oras-go/v2/registry"
)

// writeDockerConfig points DOCKER_CONFIG at config and returns its path.
func writeDockerConfig(t *testing.T, config string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", dir)
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	return configPath
}

// fakeHelper installs docker-credential-fake, which prints out and exits with code.
func fakeHelper(t *testing.T, out string, code int) {
	t.Helper()
	bin := t.TempDir()
	script := fmt.Sprintf("#!/bin/sh\ncat >/dev/null\nprintf '%%s' '%s'\nexit %d\n", out, code)
	if err := os.WriteFile(filepath.Join(bin, "docker-credential-fake"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
}

func TestOpenCredentials(t *testing.T) {
	const want = "Basic dTpw" // u:p
	var requests int
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if gotAuth = r.Header.Get("Authorization"); gotAuth != want {
			w.Header().Set("WWW-Authenticate", `Basic realm="test"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "http://")
	ref, err := registry.ParseReference(host + "/repo:tag")
	if err != nil {
		t.Fatal(err)
	}
	auths := fmt.Sprintf(`{"auths":{%q:{"auth":"dTpw"}}}`, host)
	helped := fmt.Sprintf(`{"credHelpers":{%q:"fake"}}`, host)

	for name, tc := range map[string]struct {
		config  string
		helper  func(t *testing.T)
		wantErr string // "" sends credentials if any, else fails before any request
	}{
		"auths":     {config: auths},
		"helper":    {config: helped, helper: func(t *testing.T) { fakeHelper(t, `{"Username":"u","Secret":"p"}`, 0) }},
		"none":      {config: `{}`},
		"malformed": {config: `{"auths":`, wantErr: "config.json"},
		"helper fails": {config: helped, wantErr: "keyring is locked",
			helper: func(t *testing.T) { fakeHelper(t, "keyring is locked", 1) }},
	} {
		t.Run(name, func(t *testing.T) {
			requests, gotAuth = 0, ""
			configPath := writeDockerConfig(t, tc.config)
			if tc.helper != nil {
				tc.helper(t)
			}
			c := NewClient()
			// the method drops privileges, so the config must be read already
			if err := os.WriteFile(configPath, []byte(`{}`), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := c.Open(context.Background(), ref, time.Second)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) || requests != 0 {
					t.Errorf("expected an error with %q and no request, got %v after %d requests", tc.wantErr, err, requests)
				}
				return
			}
			if tc.config == `{}` {
				if gotAuth != "" {
					t.Errorf("expected no credentials, got %q", gotAuth)
				}
			} else if !errors.Is(err, errdef.ErrNotFound) || gotAuth != want {
				t.Errorf("expected %q to reach the registry, got %q and %v", want, gotAuth, err)
			}
		})
	}
}

// TestCredentialDockerHub finds the credentials `docker login` stores for docker.io refs.
func TestCredentialDockerHub(t *testing.T) {
	writeDockerConfig(t, `{"auths":{"https://index.docker.io/v1/":{"auth":"dTpw"}}}`)
	cred, err := NewClient().credential(context.Background(), "docker.io", time.Second)
	if err != nil || cred.Username != "u" || cred.Password != "p" {
		t.Errorf("expected u:p, got %+v, %v", cred, err)
	}
}

func TestDockerConfigPathWithoutHome(t *testing.T) {
	t.Setenv("DOCKER_CONFIG", "")
	t.Setenv("HOME", "")
	u, err := user.Current()
	if err != nil {
		t.Skip(err)
	}
	got, err := dockerConfigPath()
	if want := filepath.Join(u.HomeDir, ".docker", "config.json"); err != nil || got != want {
		t.Errorf("expected %q, got %q, %v", want, got, err)
	}
}
