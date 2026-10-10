class Beam < Formula
  desc "Local-network file and clipboard transfer"
  homepage "https://github.com/SoorajSundar1505/beam-cli"

  livecheck do
    url :homepage
    strategy :github_latest
  end

  depends_on :macos

  on_macos do
    on_arm do
      url "https://github.com/SoorajSundar1505/beam-cli/releases/download/v0.1.13/beam-darwin-arm64"
      sha256 "d693a9a9c7fa62c5a7c9dce48a7e6a780fbd99492c64e447ade66da9d0cbd5aa"
    end

    on_intel do
      url "https://github.com/SoorajSundar1505/beam-cli/releases/download/v0.1.13/beam-darwin-amd64"
      sha256 "8a4db8aec0db8c7e79ad500601cadd7103821bcecb1f0b4915111256458a0e67"
    end
  end

  def install
    bin.install Dir["beam-darwin-*"].first => "beam"
  end

  test do
    assert_match "clipboard", shell_output("#{bin}/beam --help")
  end
end
