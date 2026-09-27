#!/usr/bin/env python3
"""Rewrite Homebrew, Scoop, and WinGet files for one published BEAM release.

Usage:
  python3 packaging/refresh.py v0.1.12
  python3 packaging/refresh.py v0.1.12 --checksums dist/checksums.txt

The script downloads checksums.txt from the GitHub Release when a local file
is not given. It does not commit, tag, or push.
"""

import argparse
import json
import pathlib
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parents[1]
REPO = "SoorajSundar1505/beam-cli"
OWNER = "SoorajSundar1505"
VERSION_SCHEMA = "1.12.0"


def release_version(tag: str) -> str:
    version = tag[1:] if tag.startswith("v") else tag
    parts = version.split(".")
    if len(parts) < 2 or not all(part.isdigit() for part in parts):
        raise SystemExit(f"expected a version tag such as v0.1.12, got {tag}")
    return version


def load_checksums(version: str, path: pathlib.Path | None) -> dict[str, str]:
    if path is None:
        url = f"https://github.com/{REPO}/releases/download/v{version}/checksums.txt"
        text = urllib.request.urlopen(url, timeout=60).read().decode()
    else:
        text = path.read_text()
    checksums = {}
    for line in text.splitlines():
        if not line.strip():
            continue
        digest, name = line.split()
        checksums[name] = digest.lower()
    required = ("beam-darwin-arm64", "beam-darwin-amd64", "beam-windows-amd64.exe")
    missing = [name for name in required if name not in checksums]
    if missing:
        raise SystemExit(f"checksums.txt is missing {', '.join(missing)}")
    return checksums


def asset_url(version: str, name: str) -> str:
    return f"https://github.com/{REPO}/releases/download/v{version}/{name}"


def write_homebrew(version: str, checksums: dict[str, str]) -> None:
    formula = f"""class Beam < Formula
  desc "Local-network file and clipboard transfer"
  homepage "https://github.com/{REPO}"

  livecheck do
    url :homepage
    strategy :github_latest
  end

  depends_on :macos

  on_macos do
    on_arm do
      url "{asset_url(version, "beam-darwin-arm64")}"
      sha256 "{checksums["beam-darwin-arm64"]}"
    end

    on_intel do
      url "{asset_url(version, "beam-darwin-amd64")}"
      sha256 "{checksums["beam-darwin-amd64"]}"
    end
  end

  def install
    bin.install Dir["beam-darwin-*"].first => "beam"
  end

  test do
    assert_match "clipboard", shell_output("#{{bin}}/beam --help")
  end
end
"""
    path = ROOT / "packaging" / "homebrew-beam" / "Formula" / "beam.rb"
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(formula)
    readme = ROOT / "packaging" / "homebrew-beam" / "README.md"
    readme.write_text(
        "# BEAM\n\n"
        "Homebrew tap for the BEAM local-network file and clipboard CLI.\n\n"
        "```bash\n"
        f"brew tap {OWNER}/beam\n"
        "brew install beam\n"
        "```\n"
    )


def write_scoop(version: str, checksums: dict[str, str]) -> None:
    manifest = {
        "version": version,
        "description": "Local-network file and clipboard transfer",
        "homepage": f"https://github.com/{REPO}",
        "license": "Unknown",
        "architecture": {
            "64bit": {
                "url": asset_url(version, "beam-windows-amd64.exe"),
                "hash": checksums["beam-windows-amd64.exe"],
            }
        },
        "pre_install": "Rename-Item \"$dir\\beam-windows-amd64.exe\" 'beam.exe'",
        "bin": "beam.exe",
        "checkver": {"github": f"https://github.com/{REPO}"},
        "autoupdate": {
            "architecture": {
                "64bit": {
                    "url": f"https://github.com/{REPO}/releases/download/v$version/beam-windows-amd64.exe"
                }
            },
            "hash": {
                "url": f"https://github.com/{REPO}/releases/download/v$version/checksums.txt",
                "regex": "$sha256\\s+beam-windows-amd64\\.exe",
            },
        },
    }
    path = ROOT / "packaging" / "scoop-beam" / "bucket" / "beam.json"
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(manifest, indent=4) + "\n")
    readme = ROOT / "packaging" / "scoop-beam" / "README.md"
    readme.write_text(
        "# BEAM\n\n"
        "Scoop bucket for the BEAM local-network file and clipboard CLI.\n\n"
        "```powershell\n"
        f"scoop bucket add beam https://github.com/{OWNER}/scoop-beam\n"
        "scoop install beam\n"
        "```\n"
    )


def write_winget(version: str, checksums: dict[str, str]) -> None:
    folder = (
        ROOT
        / "packaging"
        / "winget-pkgs"
        / "manifests"
        / "s"
        / OWNER
        / "Beam"
        / version
    )
    folder.mkdir(parents=True, exist_ok=True)
    identifier = f"{OWNER}.Beam"
    header = (
        "# yaml-language-server: $schema="
        "https://aka.ms/winget-manifest.{kind}."
        f"{VERSION_SCHEMA}.schema.json\n"
    )
    (folder / f"{identifier}.yaml").write_text(
        header.format(kind="version")
        + f"""
PackageIdentifier: {identifier}
PackageVersion: {version}
DefaultLocale: en-US
ManifestType: version
ManifestVersion: {VERSION_SCHEMA}
"""
    )
    (folder / f"{identifier}.installer.yaml").write_text(
        header.format(kind="installer")
        + f"""
PackageIdentifier: {identifier}
PackageVersion: {version}
InstallerType: portable
Commands:
  - beam
Installers:
  - Architecture: x64
    InstallerUrl: {asset_url(version, "beam-windows-amd64.exe")}
    InstallerSha256: {checksums["beam-windows-amd64.exe"].upper()}
ManifestType: installer
ManifestVersion: {VERSION_SCHEMA}
"""
    )
    (folder / f"{identifier}.locale.en-US.yaml").write_text(
        header.format(kind="defaultLocale")
        + f"""
PackageIdentifier: {identifier}
PackageVersion: {version}
PackageLocale: en-US
Publisher: {OWNER}
PublisherUrl: https://github.com/{OWNER}
PackageName: BEAM
PackageUrl: https://github.com/{REPO}
License: Proprietary
ShortDescription: Local-network file and clipboard transfer
Description: BEAM sends files and clipboard contents directly between your own devices on the local network. There is no cloud service and no account.
Tags:
  - cli
  - clipboard
  - file-transfer
  - local-network
ReleaseNotesUrl: https://github.com/{REPO}/releases/tag/v{version}
ManifestType: defaultLocale
ManifestVersion: {VERSION_SCHEMA}
"""
    )


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("tag", help="published release tag, for example v0.1.12")
    parser.add_argument("--checksums", type=pathlib.Path)
    args = parser.parse_args()
    version = release_version(args.tag)
    checksums = load_checksums(version, args.checksums)
    write_homebrew(version, checksums)
    write_scoop(version, checksums)
    write_winget(version, checksums)
    print(f"updated packaging files for {version}")


if __name__ == "__main__":
    main()
