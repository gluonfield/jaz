import { execFileSync } from "node:child_process";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname } from "node:path";

const out = process.argv[2] || "dist/acp-adapters.json";
const specPath = process.argv[3] || ".github/acp-adapter-assets.json";
const spec = JSON.parse(readFileSync(specPath, "utf8"));

const manifest = {
  adapters: {},
};

for (const [name, adapter] of Object.entries(spec.adapters)) {
  const releaseAssets = releaseAssetMap(adapter.repo, adapter.tag);
  manifest.adapters[name] = { version: adapter.version, assets: {} };
  for (const [platform, wanted] of Object.entries(adapter.assets)) {
    const asset = releaseAssets.get(wanted.name);
    if (!asset) {
      throw new Error(`${adapter.repo}@${adapter.tag} is missing ${wanted.name}`);
    }
    const sha256 = digestSHA256(asset);
    manifest.adapters[name].assets[platform] = {
      url: asset.url,
      sha256,
      binary: wanted.binary,
      ...(wanted.env ? { env: wanted.env } : {}),
    };
  }
}

mkdirSync(dirname(out), { recursive: true });
writeFileSync(out, `${JSON.stringify(manifest, null, 2)}\n`);

function releaseAssetMap(repo, tag) {
  const raw = execFileSync("gh", ["release", "view", tag, "--repo", repo, "--json", "assets"], {
    encoding: "utf8",
  });
  const release = JSON.parse(raw);
  return new Map(release.assets.map((asset) => [asset.name, asset]));
}

function digestSHA256(asset) {
  if (!asset.digest?.startsWith("sha256:")) {
    throw new Error(`${asset.name} is missing a GitHub SHA-256 digest`);
  }
  return asset.digest.slice("sha256:".length);
}
