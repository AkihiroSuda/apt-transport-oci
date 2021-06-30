# Creating an apt repo on an OCI registry

## Step 1: Create dpkg
Create `hello-apt-transport-oci_0.0_amd64.deb` from [`helloapt-transport-oci` directory](./hello-apt-transport-oci).
The easiest way is to use [fpm](https://fpm.readthedocs.io/).

```bash
sudo gem install fpm
```

```bash
fpm -s dir -t deb -m example@example.com -n hello-apt-transport-oci -v 0.0 ./hello-apt-transport-oci/usr/bin/hello-apt-transport-oci=/usr/bin/hello-apt-transport-oci
```

## Step 2: Create metadata
Create `Packages` file
```
dpkg-scanpackages -m . > Packages
```

Hint: to create GPG-signed repo, use [`apt-ftparchive`](https://www.google.com/search?q=apt-ftparchive+gpg) .

## Step 3: Push to an OCI registry
Push the deb file and the `Packages` file to an OCI registry (e.g., `ghcr.io`).

The easiest way is to use [ORAS](https://github.com/oras-project/oras) CLI.

```bash
curl -fsSL https://github.com/oras-project/oras/releases/download/v0.12.0/oras_0.12.0_linux_amd64.tar.gz | sudo tar Cxzv /usr/local/bin/ oras
```

```bash
echo YOUR_GITHUB_PERSONAL_ACCESS_TOKEN | docker login ghcr.io -u USERNAME --password-stdin
```

```bash
oras push ghcr.io/USERNAME/hello-apt-transport-oci:latest hello-apt-transport-oci_0.0_amd64.deb:application/octet-stream Packages:application/octet-stream`
```

- See [GHCR documentation](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry) to learn
how to create a GitHub Personal Access Token (PAT) for `ghcr.io`.
- Use GitHub Web UI to make the image public / private.
