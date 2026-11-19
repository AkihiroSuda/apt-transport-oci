package registry

import (
	"context"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"time"

	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"
)

func (c *Client) credential(ctx context.Context, registry string, timeout time.Duration) (auth.Credential, error) {
	if c.err != nil {
		return auth.EmptyCredential, c.err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return c.store.Get(ctx, credentials.ServerAddressFromRegistry(registry))
}

// dockerConfigPath falls back to passwd if $HOME is unset.
// TODO: replace with NewStoreFromDocker after oras-project/oras-go#1785.
func dockerConfigPath() (string, error) {
	if dir := os.Getenv("DOCKER_CONFIG"); dir != "" {
		return filepath.Join(dir, "config.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		u, uerr := user.Current()
		if uerr != nil {
			return "", errors.Join(err, uerr)
		}
		home = u.HomeDir
	}
	return filepath.Join(home, ".docker", "config.json"), nil
}
