# Creating an apt repo on an OCI registry

## Step 1: Create dpkg
Create `hello-apt-transport-oci_0.1_all.deb` from [`helloapt-transport-oci` directory](./hello-apt-transport-oci).

```bash
dpkg-deb --build --root-owner-group hello-apt-transport-oci hello-apt-transport-oci_0.1_all.deb
```

## Step 2: Create repository data

Use [aptly](https://www.aptly.info) to create the repository data.

```bash
sudo apt-get install aptly
```

```bash
aptly repo create hello-apt-transport-oci
aptly repo add hello-apt-transport-oci hello-apt-transport-oci_0.1_all.deb
aptly publish repo -distribution=stable -architectures=all,amd64,arm64 hello-apt-transport-oci
```

The repository data will be locally published on `~/.aptly/public`.

```console
$ tree ~/.aptly/public
/home/USER/.aptly/public
├── dists
│   └── stable
│       ├── Contents-all.gz
│       ├── Contents-amd64.gz
│       ├── Contents-arm64.gz
│       ├── InRelease
│       ├── main
│       │   ├── binary-all
│       │   │   ├── Packages
│       │   │   ├── Packages.bz2
│       │   │   ├── Packages.gz
│       │   │   └── Release
│       │   ├── binary-amd64
│       │   │   ├── Packages
│       │   │   ├── Packages.bz2
│       │   │   ├── Packages.gz
│       │   │   └── Release
│       │   ├── binary-arm64
│       │   │   ├── Packages
│       │   │   ├── Packages.bz2
│       │   │   ├── Packages.gz
│       │   │   └── Release
│       │   ├── Contents-all.gz
│       │   ├── Contents-amd64.gz
│       │   └── Contents-arm64.gz
│       ├── Release
│       └── Release.gpg
└── pool
    └── main
        └── h
            └── hello-apt-transport-oci
                └── hello-apt-transport-oci_0.1_all.deb

11 directories, 22 files
```

## Step 3: Push to an OCI registry
Push the deb file and the `Packages` file to an OCI registry (e.g., `ghcr.io`).

The easiest way is to use [ORAS](https://github.com/oras-project/oras) CLI.

```bash
sudo apt-get install oras
```

```bash
echo YOUR_GITHUB_PERSONAL_ACCESS_TOKEN | docker login ghcr.io -u USERNAME --password-stdin
```

```bash
cd ~/.aptly/public
find . -type f -printf "%p:application/octet-stream\n" | xargs oras push --image-spec=v1.0 ghcr.io/USERNAME/hello-apt-transport:latest
```

- See [GHCR documentation](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry) to learn
how to create a [GitHub Personal Access Token](https://github.com/settings/personal-access-tokens) (PAT) for `ghcr.io`.
- Use GitHub Web UI to make the image public / private.

## Step 4: Tell users the instruction

Tell your users how to install your package:

> - Create `/etc/apt/sources.list.d/oci.sources` with the following content:
> ```
> Types: deb
> URIs: oci://ghcr.io/USERNAME/hello-apt-transport-oci:latest
> Suites: stable
> Components: main
> Signed-By: /etc/apt/keyrings/USERNAME.gpg
> ```
>
> - Register a GPG key:
> ```bash
> curl -fsSL https://github.com/USERNAME.gpg | sudo gpg --dearmor -o /etc/apt/keyrings/USERNAME.gpg
> ```
