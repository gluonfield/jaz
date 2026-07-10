import { execFileSync } from "node:child_process";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname } from "node:path";

const out = process.argv[2] || "dist/acp-adapters.json";
const specPath = process.argv[3] || ".github/acp-adapter-assets.json";
const spec = JSON.parse(readFileSync(specPath, "utf8"));

const manifest = {
  adapters: {},
};
const archiveChecks = [];

for (const [name, adapter] of Object.entries(spec.adapters)) {
  assertConsistent(name, adapter);
  const adapterAssets = releaseAssetMap(adapter.repo, adapter.tag);
  manifest.adapters[name] = { version: adapter.version, assets: {} };
  for (const [platform, wanted] of Object.entries(adapter.assets)) {
    const asset = adapterAssets.get(wanted.name);
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
    archiveChecks.push(assertArchiveContains(
      asset.url,
      [wanted.binary, ...Object.values(wanted.env || {})],
      `${name} ${platform}`,
    ));
  }
}

await Promise.all(archiveChecks);

mkdirSync(dirname(out), { recursive: true });
writeFileSync(out, `${JSON.stringify(manifest, null, 2)}\n`);

// Guards the single source of truth against partial edits: the tag must match
// the version and every asset filename must embed that version. This is what
// catches "bumped tag/version but forgot an asset name" before release.
function assertConsistent(name, adapter) {
  const { tag, version } = adapter;
  if (tag !== version && tag !== `v${version}`) {
    throw new Error(`${name}: tag "${tag}" does not match version "${version}"`);
  }
  for (const [platform, wanted] of Object.entries(adapter.assets)) {
    if (!wanted.name.includes(version)) {
      throw new Error(`${name} ${platform}: asset "${wanted.name}" does not embed version "${version}"`);
    }
  }
}

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

async function assertArchiveContains(url, paths, label) {
  const response = await fetch(url);
  if (!response.ok || !response.body) {
    throw new Error(`${label}: cannot inspect published archive: ${response.status} ${response.statusText}`);
  }
  const missing = new Set(paths);
  const reader = response.body.pipeThrough(new DecompressionStream("gzip")).getReader();
  let buffered = Buffer.alloc(0);
  for (;;) {
    const header = await take(512);
    if (!header || header.every((byte) => byte === 0)) break;
    const name = tarString(header, 0, 100);
    const prefix = tarString(header, 345, 155);
    const type = header[156];
    if (type === 0 || type === 48) {
      missing.delete(prefix ? `${prefix}/${name}` : name);
      if (missing.size === 0) {
        await reader.cancel();
        return;
      }
    }
    const size = Number.parseInt(tarString(header, 124, 12).trim() || "0", 8);
    if (!Number.isSafeInteger(size) || !(await discard(Math.ceil(size / 512) * 512))) break;
  }
  await reader.cancel();
  throw new Error(`${label}: published archive is missing required file: ${[...missing].join(", ")}`);

  async function take(length) {
    while (buffered.length < length) {
      const { value, done } = await reader.read();
      if (done) return null;
      buffered = Buffer.concat([buffered, Buffer.from(value)]);
    }
    const value = buffered.subarray(0, length);
    buffered = buffered.subarray(length);
    return value;
  }

  async function discard(length) {
    while (length > 0) {
      if (buffered.length === 0) {
        const { value, done } = await reader.read();
        if (done) return false;
        buffered = Buffer.from(value);
      }
      const consumed = Math.min(length, buffered.length);
      buffered = buffered.subarray(consumed);
      length -= consumed;
    }
    return true;
  }
}

function tarString(header, start, length) {
  return header.subarray(start, start + length).toString("utf8").replace(/\0.*$/, "");
}
