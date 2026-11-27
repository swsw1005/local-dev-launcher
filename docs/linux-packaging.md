# Linux packages

The Linux release workflow builds amd64 and arm64 `.deb` and `.rpm` packages
from a stable version tag, tests the project, signs RPM packages and APT/RPM
repository metadata, and publishes the repositories on the `gh-pages` branch.
This workflow requires repository setup before the first release; it does not
make the packages available until a successful run is published.

## Maintainer setup

1. Create a dedicated OpenPGP signing key with a passphrase and securely back
   it up. Record the full public-key fingerprint through an independent channel.
2. Add the ASCII-armored private key as the GitHub Actions secret
   `LDR_REPOSITORY_SIGNING_KEY` and its passphrase as
   `LDR_REPOSITORY_SIGNING_PASSPHRASE`. Enable Actions write access to the
   repository contents.
3. Run the workflow for a stable release tag whose version matches `ldr
   --version`, then configure GitHub Pages to publish the root of the `gh-pages`
   branch. The first successful workflow run creates the branch.
4. Verify package signatures, installation, upgrade, and removal on the
   supported Ubuntu and Fedora releases before announcing the repository.

The workflow refuses to replace an existing public signing key with a different
one. Key rotation requires a separate migration plan so existing clients can
trust the new key. The repository accumulates prior package versions.

## Installation after publication

The expected repository base URL is
`https://swsw1005.github.io/local-dev-launcher`. Verify the fingerprint of
`ldr-signing-key.asc` against a trusted independent source before installing
it as a trusted key.

For Ubuntu, install the verified key as `/etc/apt/keyrings/ldr.gpg`, then add
this entry to `/etc/apt/sources.list.d/ldr.list`:

```text
deb [signed-by=/etc/apt/keyrings/ldr.gpg] https://swsw1005.github.io/local-dev-launcher/apt/ ./
```

Run `sudo apt update` and `sudo apt install ldr`.

For Fedora, add a `/etc/yum.repos.d/ldr.repo` file:

```ini
[ldr]
name=Local Dev Runner
baseurl=https://swsw1005.github.io/local-dev-launcher/rpm/
enabled=1
gpgcheck=1
repo_gpgcheck=1
gpgkey=https://swsw1005.github.io/local-dev-launcher/ldr-signing-key.asc
```

After verifying the signing key, run `sudo dnf install ldr`. Updates are
available through the system package manager once a newer package is published.
