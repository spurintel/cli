#!/bin/sh
# Generates the bash/zsh/fish completion scripts GoReleaser bundles into the
# release archives and the Homebrew cask. Run automatically by the goreleaser
# `before` hook; safe to run by hand for local inspection.
set -e
rm -rf completions && mkdir completions
for sh in bash zsh fish; do
	go run ./cmd/spur completion "$sh" > "completions/spur.$sh"
done
