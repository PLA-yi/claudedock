#!/usr/bin/env bash
set -euo pipefail

# Usage: generate-homebrew-formula.sh <version> <darwin-amd64-sha256> <darwin-arm64-sha256> <linux-amd64-sha256> <linux-arm64-sha256>
# Outputs Formula Ruby file to stdout.

VERSION="${1:?version required}"
SHA_DARWIN_AMD64="${2:?darwin-amd64 sha256 required}"
SHA_DARWIN_ARM64="${3:?darwin-arm64 sha256 required}"
SHA_LINUX_AMD64="${4:?linux-amd64 sha256 required}"
SHA_LINUX_ARM64="${5:?linux-arm64 sha256 required}"

REPO="ZaneL1u/cloud-cli-proxy"
BASE_URL="https://github.com/${REPO}/releases/download/v${VERSION}"

cat <<FORMULA
class CloudClaude < Formula
  desc "Transparent remote Claude Code CLI — one command to connect your cloud host"
  homepage "https://github.com/${REPO}"
  version "${VERSION}"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "${BASE_URL}/cloud-claude-darwin-arm64.tar.gz"
      sha256 "${SHA_DARWIN_ARM64}"

      def install
        bin.install "cloud-claude-darwin-arm64" => "cloud-claude"
      end
    else
      url "${BASE_URL}/cloud-claude-darwin-amd64.tar.gz"
      sha256 "${SHA_DARWIN_AMD64}"

      def install
        bin.install "cloud-claude-darwin-amd64" => "cloud-claude"
      end
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "${BASE_URL}/cloud-claude-linux-arm64.tar.gz"
      sha256 "${SHA_LINUX_ARM64}"

      def install
        bin.install "cloud-claude-linux-arm64" => "cloud-claude"
      end
    end
    if Hardware::CPU.intel?
      url "${BASE_URL}/cloud-claude-linux-amd64.tar.gz"
      sha256 "${SHA_LINUX_AMD64}"

      def install
        bin.install "cloud-claude-linux-amd64" => "cloud-claude"
      end
    end
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/cloud-claude --version")
  end
end
FORMULA
