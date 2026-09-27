# BEAM Protocol v1

BEAM is a local-network, device-to-device protocol. There is no cloud, no
account, and no third-party relay. File transfer and clipboard transfer share
one TCP session, one handshake, and one framing layer. An iPhone client can
speak this protocol later without changing Mac/Windows peers.

## Roles

| Role | Responsibility |
| --- | --- |
| Identity | Ed25519 keypair. Device ID is the first 16 bytes of SHA-256(public key), hex-encoded. |
| Advertiser | mDNS/Bonjour service `_beam._tcp` with TXT `id`, `name`, `type`, `proto=1`, optional `pair=1`. |
| Listener | TCP on the advertised port (default `47821`). Accepts pairing and data sessions. |
| Initiator | `beam send` / `beam clipboard --copy` / `beam pair --code` opens a TCP connection. |

Device `type` is `mac`, `windows`, or `ios`. Phase 1 implements Mac and Windows
endpoints. `ios` is reserved so discovery tables and pairing records stay stable.

## Discovery

Service type: `_beam._tcp.local.`

TXT records (short keys for DNS-SD limits):

- `id` — device ID
- `name` — UTF-8 display name
- `type` — `mac` \| `windows` \| `ios`
- `proto` — `1`
- `pair` — `1` while `beam pair` is waiting for a joiner

Peers that are paired but not advertising are listed as `offline`.

## Pairing

Local-network pairing with a short numeric code (PAKE-style binding):

1. Host runs `beam pair`, prints a 6-digit code, advertises `pair=1`.
2. Joiner runs `beam pair --code NNNNNN` (or is prompted), browses for `pair=1`.
3. Joiner connects and both complete the handshake **with the code mixed into HKDF**.
4. If AEAD confirmation decrypts, each side stores the other’s Ed25519 public key.

The code never travels in plaintext. A wrong code fails decryption. Pairing does
not require a prior trust relationship; data sessions do.

## Handshake (Noise-like, custom but small)

Over raw TCP:

1. **Hello** (cleartext, signed): protocol version, device ID, name, type, mode
   (`pair` or `data`), Ed25519 public key, X25519 ephemeral public key, Unix
   timestamp, Ed25519 signature over those fields.
2. **Peer Hello** — same.
3. **ECDH** on the ephemerals → `shared`.
4. **HKDF-SHA256**  
   `info = "beam-v1" || transcript || optional_pairing_code`  
   produces two 32-byte keys. Initiator send key is key 0; responder is swapped.
5. **Confirm**: each side sends an AEAD frame of `ok`. Failure ⇒ abort.

Data mode additionally requires that the peer identity is already in the local
paired-device store. Pairing mode skips that check and **writes** the store on
success.

Session AEAD is ChaCha20-Poly1305. Nonces are a 12-byte buffer with a 64-bit
counter in the last 8 bytes (separate counters per direction).

## Framing

After the handshake, every message is one encrypted frame:

```
uint32be length_of_ciphertext
ciphertext = AEAD(nonce, type_byte || payload)
```

`type_byte` values:

| Type | Name | Payload |
| --- | --- | --- |
| 1 | Offer | JSON (file or clipboard) |
| 2 | Accept | JSON `{ "id": "..." }` |
| 3 | Reject | JSON `{ "id": "...", "reason": "..." }` |
| 4 | Data | `uint64be seq` + raw bytes (64 KiB max) |
| 5 | Done | JSON `{ "id": "...", "sha256": "hex" }` |
| 6 | Cancel | JSON `{ "id": "..." }` |
| 7 | Error | JSON `{ "code": "...", "message": "..." }` |
| 8 | Ping | empty |
| 9 | Pong | empty |

JSON offers:

```json
{
  "id": "uuid",
  "kind": "file" | "clipboard",
  "clip_format": "text" | "image" | "file",
  "name": "photo.jpg",
  "mime": "image/jpeg",
  "size": 12345
}
```

SHA-256 is computed while streaming. It is sent only in `Done`. The receiver
hashes the written bytes and compares. Mismatch or cancel deletes the partial
file (`*.beam.partial`) and does not replace the destination.

## Path safety

Offer `name` is reduced with `filepath.Base`, must be a non-empty single path
element, and must not contain `..`, separators, or NUL. The save directory is
always the configured downloads directory (default `~/Downloads`).

## Clipboard

Clipboard payloads use the same Offer/Accept/Data/Done sequence. Text is UTF-8.
Images and files are raw bytes plus a filename. History is SQLite on disk only.
Application logs must never include clipboard bodies or private keys.
