# BEAM

CLI-first, local-network file and clipboard transfer between **your** devices.
No cloud, no accounts, no GUI.

Phase 1 targets **macOS** and **Windows**. The protocol is ready for a future
iPhone client; this tree does not fake iOS support.

```text
beam init
beam pair
beam devices
beam send photo.jpg --to windows
beam receive
beam clipboard --list
beam clipboard --search "meeting"
beam clipboard --copy 1 --to windows
```

## Architecture

```text
cmd/beam                CLI entry
internal/cli            cobra commands and transfer UX
internal/device         identity, paired peers, device table
internal/discovery      mDNS/Bonjour (_beam._tcp)
internal/pairing        6-digit local-network pairing
internal/crypto         Ed25519 identity, X25519, ChaCha20-Poly1305
internal/protocol       frames, offers, filename safety
internal/transport      handshake + encrypted session
internal/transfer       streaming send/receive + SHA-256
internal/server         inbound file + clipboard (same session)
internal/client         outbound file + clipboard (same session)
internal/clipboard      OS clipboard (macOS / Windows)
internal/history        local SQLite clipboard history
internal/storage        config/data paths
```

File transfer and clipboard use **one** TCP connection, handshake, and
Offer/Accept/Data/Done framing. Details: [PROTOCOL.md](PROTOCOL.md).

## Install

Download `checksums.txt` and the binary for your platform from the latest
[GitHub Release](../../releases/latest).

### macOS

Apple Silicon (M1, M2, M3, M4, and newer) uses `beam-darwin-arm64`. Intel Macs
use `beam-darwin-amd64`.

```bash
chmod +x beam-darwin-arm64
sudo mv beam-darwin-arm64 /usr/local/bin/beam
beam --help
```

Verify the Apple Silicon download before installing:

```bash
grep ' beam-darwin-arm64$' checksums.txt | shasum -a 256 -c -
```

For an Intel Mac, replace `beam-darwin-arm64` with `beam-darwin-amd64`.

### Windows

Download `beam-windows-amd64.exe`. In PowerShell, verify it with:

```powershell
$expected = ((Select-String .\checksums.txt 'beam-windows-amd64\.exe$').Line -split '\s+')[0]
$actual = (Get-FileHash .\beam-windows-amd64.exe -Algorithm SHA256).Hash.ToLower()
if ($actual -ne $expected) { throw "Checksum verification failed" }
Write-Host "Checksum verified"
```

Move the executable into a directory on your `PATH`, optionally renaming it to
`beam.exe`.

### Linux

Download `beam-linux-amd64`, then verify and install it:

```bash
grep ' beam-linux-amd64$' checksums.txt | sha256sum -c -
chmod +x beam-linux-amd64
sudo mv beam-linux-amd64 /usr/local/bin/beam
beam --help
```

### Build from source

If Go 1.22 or newer is installed:

```bash
go install ./cmd/beam
```

Or from this repo:

```bash
go test ./...
go build -o beam ./cmd/beam
```

## Setup

On each computer:

```bash
beam init --name MacBook
```

This writes a device ID, display name, Ed25519 key (`0600`), and empty peer
list under your OS config directory (`BEAM_CONFIG_DIR` overrides it).

### Pair

Device A:

```bash
beam pair
# Pairing code: 482917
```

Device B:

```bash
beam pair --code 482917
```

The code is mixed into session key derivation. It is not sent in plaintext.

### Stay reachable

The other device must advertise on the LAN:

```bash
beam receive
```

That command listens for **files and clipboard**, snapshots local clipboard
into history, and advertises via mDNS.

## Commands

| Command | Purpose |
| --- | --- |
| `beam init` | Create identity and config |
| `beam pair` | Host pairing (prints code) |
| `beam pair --code NNNNNN` | Join pairing |
| `beam devices` | This device + paired peers (online/offline) |
| `beam send FILE [--to NAME]` | Stream a file (any type) |
| `beam receive` | Accept inbound transfers |
| `beam clipboard --list` | Local history only |
| `beam clipboard --search Q` | FTS + substring search |
| `beam clipboard --copy N --to NAME` | Send history item N (1 = newest) |
| `beam clipboard --copy "text" --to NAME` | Send literal text |

If `--to` is omitted, BEAM asks for a device number.

Default save location: `~/Downloads` (`BEAM_DOWNLOADS_DIR` overrides).

## Security properties (Phase 1)

- Device identity is an Ed25519 keypair; ID is derived from the public key.
- Data sessions require a paired public key match.
- Payloads are ChaCha20-Poly1305 after ECDH.
- Filenames are sanitized (`filepath.Base`); writes stay in the downloads dir.
- SHA-256 is checked at the end of every payload; failures delete `*.beam.partial`.
- Clipboard bodies and private keys are not written to application logs.
- Config and keys are created with mode `0600`.

## Tests

```bash
go test ./...
```

Covers identity, pairing handshake, encrypted sessions, streaming transfers,
checksums, interrupted transfers, clipboard history/search, clipboard payload
transfer, and offline peers.

## Not in Phase 1

GUI, cloud, accounts, QR pairing, folder sync, automation/BEAMFLOW, iPhone app.
