# Packaging

These files install published GitHub Release binaries. They do not change BEAM.

`python3 packaging/refresh.py vX.Y.Z` rewrites them from that release's
`checksums.txt`. It does not commit or push.

Copy each directory to its own repository after a release:

- `packaging/homebrew-beam` → `SoorajSundar1505/homebrew-beam`
- `packaging/scoop-beam` → `SoorajSundar1505/scoop-beam`
- `packaging/winget-pkgs/manifests` → a pull request against `microsoft/winget-pkgs`

Do not push those updates from the BEAM release workflow.
