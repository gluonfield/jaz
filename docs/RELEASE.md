# Releasing Jaz

Bump the version, update [`RELEASE-NOTES.md`](RELEASE-NOTES.md), commit both and push the matching `v` tag. That triggers
[`Release`](../.github/workflows/release-desktop.yml), which creates a draft,
builds and uploads all artifacts, then publishes it as the latest release
only after every build succeeds. The previous complete release stays available
to updaters while the new build runs.

The tag must equal `frontend/package.json` version with a `v` prefix
(`0.0.11` → `v0.0.11`), or the build fails.

```sh
# Bump frontend/package.json and update docs/RELEASE-NOTES.md first.
git add frontend/package.json docs/RELEASE-NOTES.md
git commit -m "Release v0.0.11" -- frontend/package.json docs/RELEASE-NOTES.md
git push origin main

# Start the release build from that exact commit.
git tag v0.0.11
git push origin v0.0.11

# Watch the build.
gh run watch "$(gh run list --workflow=release-desktop.yml -L1 --json databaseId -q '.[0].databaseId')"
```

The workflow uploads macOS `*.dmg`, macOS/Windows `*.zip`, Windows `*.exe`,
`latest-mac.yml`, `latest.yml`, Linux/macOS `jaz-backend-<os>-<arch>.tar.gz`
backend assets, and matching `.sha256` files to the draft release. Retry failed
jobs with `gh run rerun <run-id> --failed`; uploads replace their earlier assets.
Never publish the draft manually before its builds finish.

Standalone backend release assets are built with version metadata. After
installing one, `/opt/jaz/bin/jaz --version` should print the release tag.

Desktop telemetry is enabled only when the release build receives
`POSTHOG_PROJECT_TOKEN` as a repository variable or secret. The token is a
public PostHog project token. The capture host is hardcoded to
`https://us.i.posthog.com`.
