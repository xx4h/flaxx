class Flaxx < Formula
  desc "Generic scaffolding and maintenance tool for FluxCD GitOps repositories"
  homepage "https://github.com/xx4h/flaxx"
  license "MIT"
  head "https://github.com/xx4h/flaxx.git", branch: "main"
  # Uncomment and fill in when a release is tagged:
  # url "https://github.com/xx4h/flaxx.git",
  #     tag:      "v0.1.0",
  #     revision: "COMMIT_SHA"

  depends_on "go" => :build

  def install
    ldflags = "-s -w -X main.version=#{version}"
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
