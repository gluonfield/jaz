import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";

// Bumps .github/acp-adapter-assets.json to the latest release of each adapter
// repo. Versioned asset filenames are rewritten in lockstep; replacement is a
// no-op for stable names. Writes the spec in place and prints a JSON summary
// ({changed, bumps}) for the workflow to act on. Validation that the new assets
// actually exist is left to create-acp-adapters-manifest.mjs.
const specPath = process.argv[2] || ".github/acp-adapter-assets.json";
const spec = JSON.parse(readFileSync(specPath, "utf8"));

const bumps = [];
for (const [name, adapter] of Object.entries(spec.adapters)) {
  const latest = latestTag(adapter.repo);
  if (!latest || latest === adapter.tag) {
    continue;
  }
  const oldVersion = adapter.tag.replace(/^v/, "");
  const newVersion = latest.replace(/^v/, "");
  for (const wanted of Object.values(adapter.assets)) {
    wanted.name = wanted.name.replaceAll(oldVersion, newVersion);
  }
  adapter.tag = latest;
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
