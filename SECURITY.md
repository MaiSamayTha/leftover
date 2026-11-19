# Security

Leftover moves folders on people's disks, so safety bugs are taken as seriously as security bugs.

## Reporting

Please report anything that could lead to data loss or to Leftover acting outside what it promises through GitHub's private vulnerability reporting (the **Report a vulnerability** button on the Security tab), not a public issue. Include the OS, the Leftover version (`leftover version`), what you did, and what happened. You'll get a reply within a few days.

## What counts

- Leftover moving, changing or deleting anything you didn't confirm, or anything outside the folders a rule describes
- A rule marking something `rebuild` or `redownload` that doesn't actually come back
- Following a symlink or junction, crossing onto another drive, or writing outside its data folder while scanning
- A vault entry that can't be restored when it should be
- The desktop window's local server being reachable from another site, another user or another machine
- Any network request made by the app other than the update check you start, or an update installed without a valid signature
- Tampering possible in the install scripts or release checksums

## Design

- **Network only on request.** The only code that talks to the internet is `internal/update`, and it runs only when someone clicks Check for updates or runs `leftover update`. It asks GitHub's releases API for the latest version and sends nothing but a `Leftover/<version>` user agent. The only server is the window's, bound to `127.0.0.1` on a random port.
- **Signed updates.** Each release's `checksums.txt` is signed with an Ed25519 key (`tools/sign`; the private half is a release-workflow secret, the public half is in `internal/update/keys.go`). Update now installs nothing unless the signature verifies and the download matches its checksum. On macOS the new app must also pass `codesign --verify`.
- **Window server.** Every API call needs a random 192-bit key generated at launch and passed in the URL fragment, so it never appears in server logs or referrers. Requests with a foreign `Host` or `Origin` header are rejected, which blocks other websites and DNS rebinding. Pages are served with a strict Content Security Policy and no inline scripts or styles. The window can only ask to reveal folders the scan itself reported.
- **Read-only scanning.** The scanner uses `lstat` and directory listings, never follows links, and never crosses device boundaries.
- **Reversible cleaning.** Items move by rename into a vault on the same drive. A manifest is written before anything moves. Multi-folder items roll back if any part fails. Home, its parents, drive roots, the data folder and the vaults are refused whatever a rule says.
- **Checked catalogue.** Rules load with unknown fields rejected, and a rule that could clean something must cite an `https` source or the program refuses to start.
- **Supply chain.** No third-party Go modules; CI fails if one appears. Releases are built by GitHub Actions from a tag, and each lists SHA-256 checksums that the install scripts verify before installing.
