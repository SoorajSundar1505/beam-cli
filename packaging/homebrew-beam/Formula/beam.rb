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
      url "https://github.com/SoorajSundar1505/beam-cli/releases/download/v0.1.14/beam-darwin-arm64"
      sha256 "262676b548e02fb5233b4d3025409f59baed28c06f178dced2e0e3ccf8a51da3"
    end

    on_intel do
      url "https://github.com/SoorajSundar1505/beam-cli/releases/download/v0.1.14/beam-darwin-amd64"
      sha256 "c808f179c33119375531dc17d7beab33043d73b356b35ef581fc74cf300ad2bc"
    end
  end

  def install
    bin.install Dir["beam-darwin-*"].first => "beam"
  end

  test do
    assert_match "clipboard", shell_output("#{bin}/beam --help")
  end
end
