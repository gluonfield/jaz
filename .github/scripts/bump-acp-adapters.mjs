import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";

// Bumps .github/acp-adapter-assets.json to the latest release of each adapter
// repo. Asset filenames embed the version, so a bump rewrites tag, version, and
// every asset name in lockstep. Writes the spec in place and prints a JSON
// summary ({changed, bumps}) for the workflow to act on. Validation that the new
// assets actually exist is left to create-acp-adapters-manifest.mjs.
const specPath = process.argv[2] || ".github/acp-adapter-assets.json";
const spec = JSON.parse(readFileSync(specPath, "utf8"));

const bumps = [];
for (const [name, adapter] of Object.entries(spec.adapters)) {
  if (adapter.runtime?.length) {
    continue;
  }
  const latest = latestTag(adapter.repo);
  if (!latest || latest === adapter.tag) {
    continue;
  }
  const oldVersion = adapter.version;
  const newVersion = latest.replace(/^v/, "");
  for (const [platform, wanted] of Object.entries(adapter.assets)) {
    if (!wanted.name.includes(oldVersion)) {
      throw new Error(`${name} ${platform}: asset "${wanted.name}" does not embed current version "${oldVersion}", cannot bump safely`);
    }
    wanted.name = wanted.name.split(oldVersion).join(newVersion);
  }
  adapter.tag = latest;
  adapter.version = newVersion;
  bumps.push(`${name} ${oldVersion} -> ${newVersion}`);
}

if (bumps.length > 0) {
  writeFileSync(specPath, `${JSON.stringify(spec, null, 2)}\n`);
}
process.stdout.write(`${JSON.stringify({ changed: bumps.length > 0, bumps })}\n`);

function latestTag(repo) {
  const raw = execFileSync("gh", ["release", "view", "--repo", repo, "--json", "tagName"], {
    encoding: "utf8",
  });
  return JSON.parse(raw).tagName;
}
