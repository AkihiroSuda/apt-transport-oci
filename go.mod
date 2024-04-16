module github.com/AkihiroSuda/apt-transport-oci

go 1.16

require (
	github.com/cloudflare/apt-transport-cloudflared v0.0.0-20210629160405-bbcb96fd4852
	github.com/containerd/containerd v1.5.2
	github.com/docker/cli v20.10.7+incompatible
	github.com/docker/docker v20.10.7+incompatible // indirect
	github.com/docker/docker-credential-helpers v0.6.4 // indirect
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.0.1
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.7.0
)

// LICENSE:
// - cloudflare/apt-transport-cloudflared: BSD 3-clause https://github.com/cloudflare/apt-transport-cloudflared/blob/96e1417f9c542a53d41b619cd17d3ccd9786fd49/LICENSE
// - containerd/{containerd, nerdctl}: Apache License 2.0 https://github.com/containerd/containerd/blob/main/LICENSE
// - opencontainers/{go-digest, image-spec}: Apache License 2.0 https://github.com/opencontainers/go-digest/blob/master/LICENSE
