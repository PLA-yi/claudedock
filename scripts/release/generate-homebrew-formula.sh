#!/usr/bin/env bash
set -euo pipefail

# Usage: generate-homebrew-formula.sh <version> <darwin-amd64-sha256> <darwin-arm64-sha256> <linux-amd64-sha256> <linux-arm64-sha256>
# Outputs Formula Ruby file to stdout.

VERSION="${1:?version required}"
SHA_DARWIN_AMD64="${2:?darwin-amd64 sha256 required}"
SHA_DARWIN_ARM64="${3:?darwin-arm64 sha256 required}"
SHA_LINUX_AMD64="${4:?linux-amd64 sha256 required}"
SHA_LINUX_ARM64="${5:?linux-arm64 sha256 required}"

REPO="claudedock/claudedock"
BASE_URL="https://github.com/${REPO}/releases/download/v${VERSION}"

cat <<FORMULA
class Claudedock < Formula
  desc "Transparent remote Claude Code CLI — one command to connect your cloud host"
  homepage "https://github.com/${REPO}"
  version "${VERSION}"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "${BASE_URL}/claudedock-darwin-arm64.tar.gz"
      sha256 "${SHA_DARWIN_ARM64}"

      def install
        bin.install "claudedock-darwin-arm64" => "claudedock"
      end
    else
      url "${BASE_URL}/claudedock-darwin-amd64.tar.gz"
      sha256 "${SHA_DARWIN_AMD64}"

      def install
        bin.install "claudedock-darwin-amd64" => "claudedock"
      end
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "${BASE_URL}/claudedock-linux-arm64.tar.gz"
      sha256 "${SHA_LINUX_ARM64}"

      def install
        bin.install "claudedock-linux-arm64" => "claudedock"
      end
    end
    if Hardware::CPU.intel?
      url "${BASE_URL}/claudedock-linux-amd64.tar.gz"
      sha256 "${SHA_LINUX_AMD64}"

      def install
        bin.install "claudedock-linux-amd64" => "claudedock"
      end
    end
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/claudedock --version")
  end
end
FORMULA
