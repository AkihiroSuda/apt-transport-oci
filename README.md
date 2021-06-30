# apt-transport-oci: OCI transport plugin for `apt-get` (i.e., `apt-get` over `ghcr.io`)

`apt-transport-oci` is an `apt-get` plugin to support distributing `*.deb` packages over an OCI registry such as `ghcr.io` .

## Motivation
The motivation is to distribute `*.deb` packages without running a web server but using a popular fully-managed service such as `ghcr.io`.

If GitHub could offer fully-managed apt repo, this plugin wouldn't be needed.

## Usage

- Install the `apt-transport-oci` plugin onto `/usr/lib/apt/methods/oci`:
```
sudo go build -o /usr/lib/apt/methods/oci ./cmd/usr-lib-apt-methods-oci
```

- Create `/etc/apt/sources.list.d/oci.list` with the following content to enable [`oci://ghcr.io/akihirosuda/hello-apt-transport-oci`](https://ghcr.io/akihirosuda/hello-apt-transport-oci) repo:
```
deb [trusted=yes] oci://ghcr.io/akihirosuda/hello-apt-transport-oci:latest /
```

- Run `sudo apt-get update && sudo apt-get install hello-apt-transport-oci`

```console
$ sudo apt-get update
...
Get:7 oci://ghcr.io/akihirosuda/hello-apt-transport-oci:latest  Packages [478 B]
...
Reading package lists... Done
```

```console
$ sudo apt-get install hello-apt-transport-oci
Reading package lists... Done
Building dependency tree... Done
Reading state information... Done
The following NEW packages will be installed:
  hello-apt-transport-oci
0 upgraded, 1 newly installed, 0 to remove and 1 not upgraded.
Need to get 1,106 B of archives.
After this operation, 0 B of additional disk space will be used.
Get:1 oci://ghcr.io/akihirosuda/hello-apt-transport-oci:latest  hello-apt-transport-oci 0.0 [1,106 B]
Selecting previously unselected package hello-apt-transport-oci.
(Reading database ... 249739 files and directories currently installed.)
Preparing to unpack .../hello-apt-transport-oci_0.0_amd64.deb ...
Unpacking hello-apt-transport-oci (0.0) ...
Setting up hello-apt-transport-oci (0.0) ...
```

- Make sure `hello-apt-transport-oci` is installed
```console
$ hello-apt-transport-oci
Hello, apt-transport-oci
```

### Hints
- Create `/root/.docker/config.json` to enable authentication.
- Non-TLS registry is supported only for 127.0.0.1
- Troubleshooting: Run `apt-get -o Debug::pkgAcquire::Worker=1 update 2>&1` and grep `FailReason`

## Creating an apt repo
See [`./examples`](./examples) .

## Specification
The spec corresponds to the behavior of `oras push IMAGE FILE1:application/octet-stream FILE2:application/octet-stream ...` (ORAS v0.12).

- An image index MAY have multiple manifests, but all the manifests SHOULD refer to the same set of layers (because `apt-get` itself supports multi-arch repo).
- A layer MUST have `org.opencontainers.image.title` annotation that corresponds to the file name.
- A layer SHOULD have one of the following media types:
 - `application/octet-stream`
 - `application/x-binary`
