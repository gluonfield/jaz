# Releasing Jaz

Bump the version, then publish a GitHub Release with a matching `v` tag. That
triggers [`Release Desktop`](../.github/workflows/release-desktop.yml), which
builds, signs, notarizes, and uploads the macOS app.

The tag must equal `frontend/package.json` version with a `v` prefix
(`0.0.11` → `v0.0.11`), or the build fails.

```sh
# 1. bump version in frontend/package.json (e.g. -> "0.0.11"), then:
git commit -am "feat: version update" && git push origin main

# 2. create + publish the release (this starts the build; don't attach assets)
gh release create v0.0.11 --target main --title v0.0.11 --generate-notes

# 3. watch it
gh run watch "$(gh run list --workflow=release-desktop.yml -L1 --json databaseId -q '.[0].databaseId')"
```

The workflow uploads `*.dmg`, `*.zip`, and `latest-mac.yml` to the release.
Re-runs overwrite assets (`--clobber`), so to redo a release just publish the
tag again.
