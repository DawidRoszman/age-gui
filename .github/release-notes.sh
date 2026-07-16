#!/usr/bin/env bash
# Generates the GitHub release notes. Called by .github/workflows/release.yml.
#
# The install instructions are most of this file on purpose. These builds are
# unsigned, so every user's first experience is an OS warning telling them the
# app might be malware. Saying nothing would leave a security tool looking
# exactly like the thing it protects against.
set -euo pipefail

VERSION="${1:?usage: release-notes.sh VERSION}"

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHANGELOG="$HERE/../CHANGELOG.md"

# Pull this version's section out of the changelog: everything under its
# "## <version>" heading, up to the next release heading.
#
# Refusing to publish without notes is deliberate. Release notes written after
# the fact do not get written, and a release that only says "Download" tells a
# user nothing about whether they want it.
notes="$(awk -v v="## $VERSION" '
  $0 == v          { found = 1; next }
  found && /^## /  { exit }
  found            { print }
' "$CHANGELOG")"

# Trim blank lines from both ends, so the heading below sits against the text.
notes="$(printf '%s' "$notes" | sed -e '/./,$!d' -e :a -e '/^\n*$/{$d;N;ba' -e '}')"

if [ -z "$notes" ]; then
  echo "release-notes.sh: CHANGELOG.md has no '## $VERSION' section." >&2
  echo "Add one before tagging; the release notes are generated from it." >&2
  exit 1
fi

cat <<EOF
Share secrets securely, without the command line.

## What's new in $VERSION

$notes

## Download

| Your system | File |
|---|---|
| **Windows** | \`encryptor-${VERSION}-windows-amd64-installer.exe\` |
| **macOS** (Apple Silicon or Intel) | \`encryptor-${VERSION}-macos-universal.zip\` |
| **Ubuntu / Debian** | \`encryptor_${VERSION}_amd64.deb\` |
| **Fedora / RHEL / openSUSE** | \`encryptor-${VERSION}-1.x86_64.rpm\` |
| **Other Linux** | \`encryptor-${VERSION}-linux-amd64.tar.gz\` |

Windows also has a portable \`.exe\` if you would rather not run an installer.

## Installing

### Windows

Run the installer. It brings the WebView2 runtime with it, so there is nothing
else to install.

Windows will show **"Windows protected your PC"**. This is because these builds
are not signed — see below. Click **More info**, then **Run anyway**.

### macOS

Unzip and drag **Encryptor** to your Applications folder.

The first launch will be refused, with a warning that Encryptor cannot be
verified or is from an unidentified developer. To allow it:

1. Open **System Settings → Privacy & Security**
2. Scroll down. There will be a line about Encryptor being blocked
3. Click **Open Anyway**

(On macOS 15 and later this is the only route — right-click → Open no longer
works.)

### Ubuntu / Debian

\`\`\`
sudo apt install ./encryptor_${VERSION}_amd64.deb
\`\`\`

Needs Ubuntu 22.04+ or Debian 12+. \`apt\` installs GTK and WebKit for you.

### Fedora / RHEL / openSUSE

\`\`\`
sudo dnf install ./encryptor-${VERSION}-1.x86_64.rpm
\`\`\`

### Other Linux

Unpack the tarball and run \`encryptor\`. You will need GTK 3 and WebKit2GTK 4.1
already installed.

## About those warnings

Windows and macOS warn about software that has not been signed with a paid
certificate (roughly \$100–400 a year). These builds are not signed, so both
will tell you the app is from an unidentified developer.

The warning means "nobody paid to vouch for this", not "this was found to be
harmful". If you would rather not take that on faith, everything here is built
in public: the workflow that produced these files is
[release.yml](https://github.com/DawidRoszman/age-gui/blob/${VERSION}/.github/workflows/release.yml),
you can read the source, and you can build it yourself.

Verify your download against \`SHA256SUMS\`:

\`\`\`
sha256sum -c SHA256SUMS --ignore-missing
\`\`\`

## Your key

Encryptor generates a **post-quantum** keypair (X25519 + ML-KEM-768) and stores it
encrypted with your passphrase.

**Back it up.** *Your key → Back up your key.* If you lose your key with no
backup, every file encrypted to you is unreadable forever — there is no reset
and nobody can recover it.

The backup is a standard age file, so \`age -d\` opens it with the stock CLI.
Your key is never locked inside this app.
EOF
