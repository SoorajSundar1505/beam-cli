# BEAM

BEAM sends files and clipboard text between your own devices on the local network.
There is no cloud service and no account.

```bash
beam devices
beam pair
beam send photo.jpg --to Suraj-Windows
beam clipboard --to Suraj-Windows
beam portal --tray ~/Downloads
```

The current release is [v0.1.15](https://github.com/SoorajSundar1505/beam-cli/releases/tag/v0.1.15).

## Install

macOS, with [Homebrew](https://github.com/SoorajSundar1505/homebrew-beam-cli):

```bash
brew tap SoorajSundar1505/beam-cli
brew install beam
```

Windows, with [Scoop](https://github.com/SoorajSundar1505/scoop-beam-cli):

```powershell
scoop bucket add beam https://github.com/SoorajSundar1505/scoop-beam-cli
scoop install beam
```

Windows, with WinGet:

```powershell
winget install SoorajSundar1505.Beam
```

Or download a binary from the [latest release](https://github.com/SoorajSundar1505/beam-cli/releases/latest).

| Platform | File |
| --- | --- |
| macOS Apple Silicon | `beam-darwin-arm64` |
| macOS Intel | `beam-darwin-amd64` |
| Windows AMD64 | `beam-windows-amd64.exe` |
| Linux AMD64 | `beam-linux-amd64` |

Check the download against `checksums.txt` from the same release:

```bash
shasum -a 256 -c checksums.txt
```

On Windows:

```powershell
Get-FileHash .\beam-windows-amd64.exe -Algorithm SHA256
```

Compare that value with the `beam-windows-amd64.exe` line in `checksums.txt`.

Build from this repository with Go 1.22 or newer:

```bash
go build -o beam ./cmd/beam
```

`beam --version` prints `dev` for a local build. Release binaries print the tag, such as `v0.1.15`.

## Use

The first command creates this device, starts one background receiver, and turns on login startup on macOS and Windows. You do not need to leave a terminal open.

```text
BEAM [ONLINE]  Suraj-Mac

Devices

1. Suraj-Mac       [THIS DEVICE]
2. Suraj-Windows   [ONLINE]
```

`[ONLINE]` means the other device was just seen on the local network. `[UNREACHABLE]` means the last address failed. BEAM does not print IP addresses.

Pair once. On the other device, enter the code this one prints:

```bash
beam pair
beam pair --code 482917
```

Rename this device without changing its identity:

```bash
beam init --name Suraj-Mac
```

Send a file or the clipboard:

```bash
beam send photo.jpg --to Suraj-Windows
beam clipboard --to Suraj-Windows
beam clipboard --list
beam clipboard --search notes
beam clipboard --copy 2 --to Suraj-Windows
```

Without `--to`, BEAM asks you to pick a device.

If a send cannot reach the other device, BEAM tries the newest discovered address once, then prints `Transfer failed: Suraj-Windows is currently unreachable.` Add `--debug` to see the network error. A dropped file transfer keeps the chunks already received and continues from there the next time you send the same file. An offline device can queue the file locally until it is back.

Received files go to `~/Downloads`.

```bash
beam status
beam status --verbose
beam stop
beam start
beam start --autostart
beam stop --disable-autostart
```

## Portal

`beam portal` opens a local page in the browser. The page starts dark, with the instruction “Draw a circle to open the portal.” A camera preview sits in the bottom-right corner and shows your hand there. The main view keeps a fingertip cursor and does not draw a guide circle.

Draw one circle in the air with your index finger, in either direction. When that circle is complete, the ring opens and paired devices appear around it.

```bash
beam portal --tray ~/Downloads
beam portal --no-browser
```

Hold your hand near the top or bottom of the file tray to scroll the list. Pinch a device to select it, or click it. Pinch a file to pick it up, then open your hand over the portal or throw the file into it. That uses the same transfer as `beam send` and shows its real progress. You can import files on the page. You can also drag a circle on the stage with the mouse. Debug stays off unless you turn it on.

The portal needs a webcam and a browser. Gesture recognition stays on this computer.

## Commands

| Command | Purpose |
| --- | --- |
| `beam` | This device and paired devices |
| `beam devices` | Same device list |
| `beam status` | Receiver status and the device list |
| `beam send FILE [--to NAME]` | Send a file |
| `beam clipboard [--to NAME]` | Send the current clipboard |
| `beam clipboard --list` | Local clipboard history |
| `beam clipboard --search TEXT` | Search that history |
| `beam clipboard --copy N --to NAME` | Send history item N |
| `beam pair` | Show a pairing code |
| `beam pair --code NNNNNN` | Pair with that code |
| `beam init [--name NAME]` | Create or rename this device |
| `beam portal [--tray DIR]` | Open the hand-controlled portal |
| `beam start` | Start the background receiver |
| `beam stop` | Stop the background receiver |
| `beam --version` | Print the release tag |

## Supported systems

| System | Install | Notes |
| --- | --- | --- |
| macOS Apple Silicon and Intel | Homebrew or the release binary | Login startup is supported |
| Windows AMD64 | Scoop, WinGet, or the release binary | Login startup is supported. WinGet updates after Microsoft accepts the manifest |
| Linux AMD64 | Release binary | The CLI builds and runs. There is no distro package. The portal opens through `xdg-open` |

BEAM does not include an iOS or Android app, and it cannot relay a transfer through the internet. Both devices have to be on the same local network.

## Security

Each device has an Ed25519 identity. Transfers use a paired key, X25519, and ChaCha20-Poly1305, and every file is checked with SHA-256. Clipboard text and private keys are not written to logs. Protocol details are in [PROTOCOL.md](PROTOCOL.md).

## Troubleshooting

- `currently unreachable`: the paired device is off, on another network, or its receiver is stopped. Run `beam start` on that device, then try again with `--debug`.
- A repeated send of the same file continues from the chunks already received. If the source file changed, BEAM starts over.
- Portal camera: allow camera access in the browser and reload the page. Drawing still works with the mouse if the camera is unavailable.
- Receiver: `beam status` shows whether it is running. `beam stop` and `beam start` restart it.

Issues and releases: [github.com/SoorajSundar1505/beam-cli/issues](https://github.com/SoorajSundar1505/beam-cli/issues) and [github.com/SoorajSundar1505/beam-cli/releases](https://github.com/SoorajSundar1505/beam-cli/releases).
