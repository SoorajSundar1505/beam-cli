# BEAM

Send files and clipboard text between your own devices on the local network.
No cloud, no accounts, no GUI. macOS and Windows.

```bash
beam devices
beam pair
beam send photo.jpg --to Suraj-Windows
beam clipboard --to Suraj-Windows
```

## Install

macOS:

```bash
brew tap SoorajSundar1505/beam
brew install beam
```

Windows:

```powershell
scoop bucket add beam https://github.com/SoorajSundar1505/scoop-beam
scoop install beam
```

```powershell
winget install SoorajSundar1505.Beam
```

Or download a binary from the latest [GitHub Release](https://github.com/SoorajSundar1505/beam-cli/releases/latest) and check it against `checksums.txt`.

| Platform | File |
| --- | --- |
| macOS Apple Silicon | `beam-darwin-arm64` |
| macOS Intel | `beam-darwin-amd64` |
| Windows | `beam-windows-amd64.exe` |
| Linux | `beam-linux-amd64` |

Build from this repo with Go 1.22 or newer: `go build -o beam ./cmd/beam`

## Use

The first command creates this device, starts one background receiver, and turns on login startup. You do not need to leave a terminal open.

```text
BEAM [ONLINE]  Suraj-Mac

Devices

1. Suraj-Mac       [THIS DEVICE]
2. Suraj-Windows   [ONLINE]
```

`[ONLINE]` means the other device was just seen. `[UNREACHABLE]` means the last address failed. BEAM does not print IP addresses.

Pair once. On the other device, enter the code this one prints:

```bash
beam pair
beam pair --code 482917
```

Rename this device without changing its identity:

```bash
beam init --name Suraj-Mac
```

If a send cannot reach the other device, BEAM tries the newest discovered address once, then prints `Transfer failed: Suraj-Windows is currently unreachable.` Add `--debug` to see the network error. An offline device can queue the file locally until it is back.

```bash
beam stop                  # stop the receiver
beam start                 # start it again
beam start --autostart     # also start at login
beam stop --disable-autostart
```

Received files go to `~/Downloads`.

## Commands

| Command | Purpose |
| --- | --- |
| `beam devices` | This device and paired devices |
| `beam status` | Receiver status and the device list |
| `beam send FILE [--to NAME]` | Send a file |
| `beam clipboard [--to NAME]` | Send the current clipboard |
| `beam clipboard --list` | Local clipboard history |
| `beam clipboard --search TEXT` | Search that history |
| `beam clipboard --copy N --to NAME` | Send history item N |
| `beam pair` | Show a pairing code |
| `beam pair --code NNNNNN` | Pair with that code |
| `beam init [--name NAME]` | Create or rename this device |
| `beam stop` | Stop the background receiver |

Without `--to`, BEAM asks you to pick a device.

## Security

Each device has an Ed25519 identity. Transfers use a paired key, X25519, and ChaCha20-Poly1305, and every file is checked with SHA-256. Clipboard text and private keys are not written to logs. Protocol details are in [PROTOCOL.md](PROTOCOL.md).
