class Hum < Formula
  desc "Auditory display daemon that renders work sessions as ambient music"
  homepage "https://github.com/mach4-braai/hum"
  url "https://github.com/mach4-braai/hum/releases/download/v0.1.0/hum_0.1.0_source.tar.gz"
  sha256 "0000000000000000000000000000000000000000000000000000000000000000"
  license "MIT"
  head "https://github.com/mach4-braai/hum.git", branch: "master"

  depends_on "go" => :build

  on_linux do
    depends_on "alsa-lib"
    depends_on "pkg-config" => :build
  end

  def install
    commit = Utils.git_head(buildpath, safe: false) || "none"
    ldflags = %W[
      -X main.version=#{version}
      -X main.commit=#{commit}
      -X main.date=#{time.iso8601}
    ]

    system "go", "build", *std_go_args(output: bin/"hum", ldflags:), "./cmd/hum"
    system "go", "build", *std_go_args(output: bin/"humd", ldflags:), "./cmd/humd"

    (var/"log/hum").mkpath
  end

  service do
    run [opt_bin/"humd"]
    run_type :immediate
    keep_alive crashed: true
    log_path var/"log/hum/humd.log"
    error_log_path var/"log/hum/humd.error.log"
    working_dir Dir.home
  end

  def caveats
    <<~EOS
      Start the daemon under launchd or systemd:
        brew services start hum

      Logs are written to:
        #{var}/log/hum/humd.log
        #{var}/log/hum/humd.error.log
    EOS
  end

  test do
    assert_match "hum", shell_output("#{bin}/hum version")
    assert_match version.to_s, shell_output("#{bin}/hum version") unless build.head?

    require "json"
    reported = JSON.parse(shell_output("#{bin}/hum version --json"))
    assert_equal "hum", reported["program"]

    assert_match "humd", shell_output("#{bin}/humd --version")

    assert_match "daemon", shell_output("#{bin}/hum doctor 2>&1", 1)
  end
end
