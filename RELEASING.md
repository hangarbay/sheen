# Releasing sheen

1. Bump and tag:

   ```
   git tag v0.1.1 && git push origin v0.1.1
   ```

2. The `Update Homebrew tap` workflow fires on `v*` tags. It downloads the
   release tarball, computes its sha256, rewrites `Formula/sheen.rb` in
   `hangarbay/homebrew-tap`, and pushes a `sheen <version>` commit.

3. Users get the new version on their next `brew upgrade sheen`
   (Homebrew auto-updates taps).

## One-time setup

The workflow needs push access to the tap. Create a fine-grained personal
access token:

- Repository access: only `hangarbay/homebrew-tap`
- Permissions: Contents -> Read and write

Add it as a repository secret named `TAP_TOKEN` on
`github.com/hangarbay/sheen` (Settings -> Secrets and variables -> Actions).

## Notes

- The formula builds from the release tarball, so every tag must point at
  a commit that compiles (CI runs tests on push; tag from a green master).
- If installs later feel slow (source builds pull the Go toolchain), the
  next step is GoReleaser: prebuilt darwin/arm64 + amd64 tarballs attached
  to the release, and the formula switches to pouring binaries.
