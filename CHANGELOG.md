# Changelog

Notes for each release. `.github/release-notes.sh` reads the section matching the
version being tagged and puts it at the top of the GitHub release, so this file
is the source of the patch notes rather than a copy of them. A release with no
section here fails rather than publishing without notes.

Newest first. Use `## <version>` with no `v` prefix, matching the tag minus the
`v`.

## 0.4.0

**Files now save to your Downloads folder.**

Encrypted and decrypted files used to land next to the file you started with,
which scattered them wherever the original happened to live. They now go to your
Downloads folder, named after the file you started with.

*Settings* can point encrypted and decrypted files at different folders. They are
separate because the risk is different: an encrypted file is safe to leave in a
shared folder, a decrypted one usually is not.

If the name is already taken, a number is added — `report.pdf (2).age` — instead
of asking you where to put it. Nothing is ever overwritten.

**Show in folder.** The Encrypt and Decrypt screens now offer to open the result
in your file manager, so you do not have to go find it.

**A theme setting.** *Settings → Appearance* follows your desktop's light/dark
preference by default, and can be forced to light or dark if you prefer your
tools to disagree with your desktop.

On Linux the Downloads folder is read from your desktop's own configuration, so
a localized folder — `Pobrane`, `Téléchargements` — is used as-is rather than a
second `~/Downloads` being created next to it.

## 0.3.0

**The app is now called Encryptor**, with new artwork.

The binary, the Linux package and the settings folder are all renamed, so this is
a clean break rather than an upgrade: a key from an Age GUI build is not picked
up automatically. If you have one, move `age-gui` to `encryptor` inside your
config folder *before* first launch — see "Your key" below for where that is.

**Fixed: the macOS app was rejected as "damaged".** The licence files were copied
into the app bundle after it was built, which broke its signature seal, and
macOS refuses a quarantined bundle with a broken seal outright — without even
the "Open Anyway" button. The bundle is now signed after everything is in place.
The app is still not signed with a paid developer certificate, so the usual
unidentified-developer warning remains, but it can be dismissed as documented
below.
