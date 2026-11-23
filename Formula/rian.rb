class Rian < Formula
  desc "Terminal app for managing development projects, commands, and Git"
  homepage "https://github.com/FatalMistake02/rian"
  url "https://github.com/FatalMistake02/rian/archive/refs/tags/v0.2.2.tar.gz"
  sha256 "78893292f6b1ebc519cf39809bef290a34b9c645ce18447f15cdf252e609df9f"
  license "MIT"

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w -X main.version=#{version}"), "./cmd/rian"
    prefix.install "LICENSE", "THIRD_PARTY_NOTICES.txt"
  end

  test do
    assert_equal "rian #{version}", shell_output("#{bin}/rian --version").strip
  end
end
