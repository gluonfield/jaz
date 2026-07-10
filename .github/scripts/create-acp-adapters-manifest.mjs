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
const releases = new Map();

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
    const files = (adapter.runtime || []).map((file) => manifestFile(file, platform));
    manifest.adapters[name].assets[platform] = {
      url: asset.url,
      sha256,
      binary: wanted.binary,
      ...(wanted.env ? { env: wanted.env } : {}),
      ...(files.length ? { files } : {}),
    };
    archiveChecks.push(assertArchiveContains(asset.url, wanted.binary, `${name} ${platform}`));
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
    const paths = new Set([wanted.binary]);
    for (const file of adapter.runtime || []) {
      if (!file.assets?.[platform]) {
        throw new Error(`${name} ${platform}: runtime file "${file.path}" has no asset`);
      }
      if (paths.has(file.path)) {
        throw new Error(`${name} ${platform}: runtime path "${file.path}" is duplicated`);
      }
      paths.add(file.path);
    }
  }
}

function manifestFile(file, platform) {
  const source = file.assets[platform];
  const asset = releaseAssetMap(file.repo, file.tag).get(source.name);
  if (!asset) {
    throw new Error(`${file.repo}@${file.tag} is missing ${source.name}`);
  }
  archiveChecks.push(assertArchiveContains(asset.url, source.source, source.name));
  return {
    url: asset.url,
    sha256: digestSHA256(asset),
    source: source.source,
    path: file.path,
  };
}

function releaseAssetMap(repo, tag) {
  const key = `${repo}@${tag}`;
  if (releases.has(key)) {
    return releases.get(key);
  }
  const raw = execFileSync("gh", ["release", "view", tag, "--repo", repo, "--json", "assets"], {
    encoding: "utf8",
  });
  const release = JSON.parse(raw);
  const assets = new Map(release.assets.map((asset) => [asset.name, asset]));
  releases.set(key, assets);
  return assets;
}

function digestSHA256(asset) {
  if (!asset.digest?.startsWith("sha256:")) {
    throw new Error(`${asset.name} is missing a GitHub SHA-256 digest`);
  }
  return asset.digest.slice("sha256:".length);
}

async function assertArchiveContains(url, want, label) {
  const response = await fetch(url);
  if (!response.ok || !response.body) {
    throw new Error(`${label}: cannot inspect published archive: ${response.status} ${response.statusText}`);
  }
  const reader = response.body.pipeThrough(new DecompressionStream("gzip")).getReader();
  let buffered = Buffer.alloc(0);
  for (;;) {
    const header = await take(512);
    if (!header || header.every((byte) => byte === 0)) break;
    const name = tarString(header, 0, 100);
    const prefix = tarString(header, 345, 155);
    const type = header[156];
    if ((type === 0 || type === 48) && (prefix ? `${prefix}/${name}` : name) === want) {
      await reader.cancel();
      return;
    }
    const size = Number.parseInt(tarString(header, 124, 12).trim() || "0", 8);
    if (!Number.isSafeInteger(size) || !(await discard(Math.ceil(size / 512) * 512))) break;
  }
  await reader.cancel();
  throw new Error(`${label}: published archive is missing required file "${want}"`);

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
