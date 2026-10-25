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
      url "https://github.com/SoorajSundar1505/beam-cli/releases/download/v0.1.15/beam-darwin-arm64"
      sha256 "6df09eb145c706e0650e4b45d0145bf18fd42318e6c85b225da0ea1b08e64210"
    end

    on_intel do
      url "https://github.com/SoorajSundar1505/beam-cli/releases/download/v0.1.15/beam-darwin-amd64"
      sha256 "b455d433d5934133ddd5aba166821f46cb938292b3bc0c0a6456de1c0d8aae35"
    end
  end

  def install
    bin.install Dir["beam-darwin-*"].first => "beam"
  end

  test do
    assert_match "clipboard", shell_output("#{bin}/beam --help")
  end
end
