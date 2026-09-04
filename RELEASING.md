# Releasing sheen

A single tag-driven GitHub Actions workflow (`.github/workflows/release.yml`)
builds and publishes everything. No GoReleaser, no secrets beyond the
built-in `GITHUB_TOKEN`.

## Cut a release

```
git tag v0.1.2 && git push origin v0.1.2
```

The workflow then:

1. builds one artifact per target — darwin/amd64, darwin/arm64,
   linux/amd64, linux/arm64, windows/amd64 — as
   `sheen_<version>_<os>_<arch>.tar.gz` (`.zip` for windows), each with
   README and LICENSE inside, binaries stamped with the version
2. generates `checksums.txt` over all artifacts
3. creates the GitHub Release with `gh release create --generate-notes`

Users install with `install.sh` (see README) or by grabbing a tarball from
the releases page.

## Notes

- Tag from a green master; the workflow builds whatever the tag points to.
- If a release fails, delete the release and tag, then re-tag. The
  `gh release create` step is not idempotent.
- Binaries are built with CGO disabled and `-trimpath`, so any runner
  cross-compiles every target.
