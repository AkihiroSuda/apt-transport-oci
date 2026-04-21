# apt-transport-oci: OCI transport plugin for `apt-get` (i.e., `apt-get` over `ghcr.io`)

`apt-transport-oci` is an `apt-get` plugin to support distributing `*.deb` packages over an OCI registry such as `ghcr.io` .

> [!NOTE]
> "OCI" here refers to the "[Open Container Initiative](https://opencontainers.org/)", not to the "Oracle Cloud Infrastructure".

## Motivation
The motivation is to distribute `*.deb` packages without running a web server but using a popular fully-managed service such as `ghcr.io`.

If GitHub could offer fully-managed apt repo, this plugin wouldn't be needed.


## Install
The `apt-transport-oci` plugin is officially [packaged](https://repology.org/project/apt-transport-oci/versions) in Debian and Ubuntu,
since Debian 14 and Ubuntu 26.04.

```bash
sudo apt install apt-transport-oci
```

<details><summary>Build from source</summary>
<p>

```bash
sudo go build -o /usr/lib/apt/methods/oci ./cmd/usr-lib-apt-methods-oci
```

</p>
</details>

## Example
The following example installs the `hello-apt-transport-oci` package from the [`oci://ghcr.io/akihirosuda/apt-transport-oci-examples/hello-apt-transport-oci`](https://ghcr.io/akihirosuda/apt-transport-oci-examples/hello-apt-transport-oci) image.

> [!TIP]
> See <https://github.com/AkihiroSuda/apt-transport-oci-examples> for how to build and push this package to your own registry.

- Create `/etc/apt/sources.list.d/oci.sources` with the following content:
```
Types: deb
URIs: oci://ghcr.io/akihirosuda/apt-transport-oci-examples/hello-apt-transport-oci:latest
Suites: stable
Components: main
Signed-By: /etc/apt/keyrings/apt-transport-oci-examples.gpg
```

- Register a GPG key:
```bash
curl -fsSL https://raw.githubusercontent.com/AkihiroSuda/apt-transport-oci-examples/refs/heads/master/apt-transport-oci-examples.gpg \
  | sudo gpg --dearmor -o /etc/apt/keyrings/apt-transport-oci-examples.gpg
```

- Run:
```bash
sudo apt update
sudo apt install hello-apt-transport-oci
```

- Make sure `hello-apt-transport-oci` is installed
```console
$ hello-apt-transport-oci
Hello, apt-transport-oci
```

## Hints
- Create `/root/.docker/config.json` to enable authentication.
- Non-TLS registry is supported only for 127.0.0.1
- Troubleshooting: Run `apt-get -o Debug::pkgAcquire::Worker=1 update 2>&1` and grep `FailReason`

## Specification
The spec corresponds to the behavior of `oras push --image-spec=v1.0 IMAGE FILE1:application/octet-stream FILE2:application/octet-stream ...`.

- An image index MAY have multiple manifests, but all the manifests SHOULD refer to the same set of layers (because `apt-get` itself supports multi-arch repo).
- A layer MUST have `org.opencontainers.image.title` annotation that corresponds to the file name.
- A layer SHOULD have one of the following media types:
  - `application/octet-stream`
  - `application/x-binary`
