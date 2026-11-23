class Rian < Formula
  desc "Terminal app for managing development projects, commands, and Git"
  homepage "https://github.com/FatalMistake02/rian"
  version "0.2.4"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/FatalMistake02/rian/releases/download/v0.2.4/rian_0.2.4_darwin_arm64.tar.gz"
      sha256 "1818f38d899f9544bbc91a2ac3f2a06ea3b42f4c200dd1489f3a4002a2080a1d"
    end
    on_intel do
      url "https://github.com/FatalMistake02/rian/releases/download/v0.2.4/rian_0.2.4_darwin_amd64.tar.gz"
      sha256 "3a8e11138bd9461e4bd9431805968b7b740f5a6329b93b6d2b588017580eed62"
    end
  end
  on_linux do
    on_arm do
      url "https://github.com/FatalMistake02/rian/releases/download/v0.2.4/rian_0.2.4_linux_arm64.tar.gz"
      sha256 "1e20e7bbb5b80f73ee60af83fa1df3ac44fa8e83d11ab9c91a82e6e4f00a72ed"
    end
    on_intel do
      url "https://github.com/FatalMistake02/rian/releases/download/v0.2.4/rian_0.2.4_linux_amd64.tar.gz"
      sha256 "027145785193ac114f0316ba9f89b7c374d8ed4422a29b37026e83fc64c0d296"
    end
  end

  def install
    bin.install "rian"
    prefix.install "LICENSE", "THIRD_PARTY_NOTICES.txt"
  end

  test do
    assert_equal "rian #{version}", shell_output("#{bin}/rian --version").strip
  end
end
