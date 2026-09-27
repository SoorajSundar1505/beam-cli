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
      url "https://github.com/SoorajSundar1505/beam-cli/releases/download/v0.1.12/beam-darwin-arm64"
      sha256 "3f8ea2bc9627d51fb5b309e8da161edec58f59478076e141c73811700f82c01e"
    end

    on_intel do
      url "https://github.com/SoorajSundar1505/beam-cli/releases/download/v0.1.12/beam-darwin-amd64"
      sha256 "48c824c172bb117fc3e232ce49cf434dcb4cad519d7f2a067ce00ff83656ac61"
    end
  end

  def install
    bin.install Dir["beam-darwin-*"].first => "beam"
  end

  test do
    assert_match "clipboard", shell_output("#{bin}/beam --help")
  end
end
