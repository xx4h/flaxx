class Flaxx < Formula
  desc "Generic scaffolding and maintenance tool for FluxCD GitOps repositories"
  homepage "https://github.com/xx4h/flaxx"
  url "https://github.com/xx4h/flaxx.git",
      tag:      "v0.1.0",
      revision: "ab0631bb511f7141f121c86970765a9f109734c5"
  license "Apache-2.0"
  head "https://github.com/xx4h/flaxx.git", branch: "main"

  livecheck do
    url :stable
    regex(/^v?(\d+(?:\.\d+)+)$/i)
  end

  depends_on "go" => :build

  def install
    ldflags = %W[
      -s -w
      -X github.com/xx4h/flaxx/cmd.version=v#{version}
      -X github.com/xx4h/flaxx/cmd.commit=#{Utils.git_head}
      -X github.com/xx4h/flaxx/cmd.date=#{time.iso8601}
    ]
    system "go", "build", *std_go_args(ldflags:)

    generate_completions_from_executable(bin/"flaxx", "completion")
  end

  test do
    assert_match "flaxx", shell_output("#{bin}/flaxx --help")

    # Verify generate requires args
    output = shell_output("#{bin}/flaxx generate 2>&1", 1)
    assert_match "accepts 2 arg(s)", output
  end
end
