module github.com/AkihiroSuda/apt-transport-oci

go 1.16

require (
	github.com/cloudflare/apt-transport-cloudflared v0.0.0-20190717195953-96e1417f9c54
	github.com/containerd/containerd v1.5.2
	github.com/containerd/nerdctl v0.9.0
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.0.1
)

// https://github.com/cloudflare/apt-transport-cloudflared/pull/1
replace github.com/cloudflare/apt-transport-cloudflared => github.com/AkihiroSuda/apt-transport-cloudflared v0.0.0-20210629160405-bbcb96fd4852

// LICENSE:
// - cloudflare/apt-transport-cloudflared: BSD 3-clause https://github.com/cloudflare/apt-transport-cloudflared/blob/96e1417f9c542a53d41b619cd17d3ccd9786fd49/LICENSE
// - containerd/{containerd, nerdctl}: Apache License 2.0 https://github.com/containerd/containerd/blob/main/LICENSE
// - opencontainers/{go-digest, image-spec}: Apache License 2.0 https://github.com/opencontainers/go-digest/blob/master/LICENSE
