# Changelog

Notes for each release. `.github/release-notes.sh` reads the section matching the
version being tagged and puts it at the top of the GitHub release, so this file
is the source of the patch notes rather than a copy of them. A release with no
section here fails rather than publishing without notes.

Newest first. Use `## <version>` with no `v` prefix, matching the tag minus the
`v`.

## 0.5.0

**Groups.** Gather the people you often encrypt for into a named group, then pick
the whole group in one click instead of ticking everyone each time. Manage groups
on the new **Groups** tab of the Contacts screen. A group is only a shortcut for
choosing people — the file is still encrypted to each person individually, and
removing a contact quietly removes them from any groups too.

**A clearer "who can open this?"** The Encrypt screen's recipient list is now
searchable, shows how many people you've chosen, and lets you **save the current
selection as a group** on the spot. Clicking a group ticks all its members; you
can still add or remove individuals afterward.

**"Let me open this file."** A new checkbox, on by default, encrypts the file to
your own key as well — so you can always open what you encrypted. Encrypting only
to yourself, for your own storage, is fine too.

**Clearer key-mixing message.** age can't put a quantum-resistant key and a
classic key in the same file. If your selection mixes the two, Encryptor now says
so plainly and tells you how to fix it, instead of failing with a cryptic error.

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
