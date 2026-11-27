#!/usr/bin/env bash
set -euo pipefail

: "${TAG:?release tag required}"
: "${SIGNING_KEY:?repository signing key required}"
: "${SIGNING_PASSPHRASE:?repository signing passphrase required}"
[[ "$TAG" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo 'Expected a stable vX.Y.Z tag' >&2; exit 1; }
export VERSION="${TAG#v}"
test "$(git describe --tags --exact-match HEAD)" = "$TAG"
test "$(go run ./cmd/ldr --version | tail -n 1)" = "ldr $VERSION"

export GNUPGHOME
GNUPGHOME="$(mktemp -d)"
chmod 700 "$GNUPGHOME"
trap 'rm -rf "$GNUPGHOME"' EXIT
export SIGNING_KEY_FILE="$GNUPGHOME/signing-key.asc"
printf '%s\n' "$SIGNING_KEY" > "$SIGNING_KEY_FILE"
chmod 600 "$SIGNING_KEY_FILE"
gpg --batch --import "$SIGNING_KEY_FILE"
key_id="$(gpg --batch --with-colons --list-secret-keys | awk -F: '$1 == "fpr" { print $10; exit }')"
test -n "$key_id"
export NFPM_RPM_PASSPHRASE="$SIGNING_PASSPHRASE"

mkdir -p dist
bash packaging/third-party-licenses.sh > dist/THIRD_PARTY_LICENSES
for ARCH in amd64 arm64; do
  export ARCH
  CGO_ENABLED=0 GOOS=linux GOARCH="$ARCH" go build -trimpath -ldflags='-s -w' -o dist/ldr-linux ./cmd/ldr
  nfpm package --config packaging/nfpm.yaml --packager deb --target "dist/ldr_${VERSION}_${ARCH}.deb"
  nfpm package --config packaging/nfpm.yaml --packager rpm --target "dist/ldr_${VERSION}_${ARCH}.rpm"
done
(cd dist && sha256sum ./*.deb ./*.rpm > "ldr_${VERSION}_linux_SHA256SUMS.txt")

if git ls-remote --exit-code --heads origin gh-pages >/dev/null; then
  git clone --single-branch --branch gh-pages "$(git remote get-url origin)" repository
else
  git clone --no-checkout "$(git remote get-url origin)" repository
  git -C repository switch --orphan gh-pages
fi
if [ -s repository/ldr-signing-key.asc ]; then
  old_key_id="$(gpg --batch --with-colons --show-keys repository/ldr-signing-key.asc | awk -F: '$1 == "fpr" { print $10; exit }')"
  [ "$old_key_id" = "$key_id" ] || { echo 'Repository signing key changed; perform a key rotation first' >&2; exit 1; }
fi
mkdir -p repository/apt repository/rpm
cp dist/*.deb repository/apt/
cp dist/*.rpm repository/rpm/
gpg --batch --armor --export "$key_id" > repository/ldr-signing-key.asc
touch repository/.nojekyll
(
  cd repository/apt
  apt-ftparchive packages . > Packages
  gzip -n -9 -c Packages > Packages.gz
  apt-ftparchive release . > Release
  gpg --batch --yes --pinentry-mode loopback --passphrase "$SIGNING_PASSPHRASE" --local-user "$key_id" --clearsign --output InRelease Release
  gpg --batch --yes --pinentry-mode loopback --passphrase "$SIGNING_PASSPHRASE" --local-user "$key_id" --armor --detach-sign --output Release.gpg Release
)
createrepo_c --update repository/rpm
gpg --batch --yes --pinentry-mode loopback --passphrase "$SIGNING_PASSPHRASE" --local-user "$key_id" --armor --detach-sign --output repository/rpm/repodata/repomd.xml.asc repository/rpm/repodata/repomd.xml
