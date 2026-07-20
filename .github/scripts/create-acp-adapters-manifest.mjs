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

// Guards the single source of truth against partial edits. Most adapters put
// their version in each asset name; stable_asset_names marks upstreams that do
// not.
function assertConsistent(name, adapter) {
  const { tag, version } = adapter;
  if (tag !== version && tag !== `v${version}`) {
    throw new Error(`${name}: tag "${tag}" does not match version "${version}"`);
  }
  for (const [platform, wanted] of Object.entries(adapter.assets)) {
    if (!adapter.stable_asset_names && !wanted.name.includes(version)) {
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
  if (new URL(url).pathname.endsWith(".zip")) {
    const response = await fetch(url);
    if (!response.ok) {
      throw new Error(`${label}: cannot inspect published archive: ${response.status} ${response.statusText}`);
    }
    const entries = zipEntries(Buffer.from(await response.arrayBuffer()), label);
    const missing = paths.filter((path) => !entries.has(path));
    if (missing.length > 0) {
      throw new Error(`${label}: published archive is missing required file: ${missing.join(", ")}`);
    }
    return;
  }
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

function zipEntries(body, label) {
  let eocd = -1;
  for (let i = body.length - 22; i >= Math.max(0, body.length - 65557); i--) {
    if (body.readUInt32LE(i) === 0x06054b50 && i + 22 + body.readUInt16LE(i + 20) === body.length) {
      eocd = i;
      break;
    }
  }
  if (eocd < 0) {
    throw new Error(`${label}: ZIP end-of-central-directory record not found`);
  }
  const entries = body.readUInt16LE(eocd + 10);
  const directorySize = body.readUInt32LE(eocd + 12);
  const directoryOffset = body.readUInt32LE(eocd + 16);
  if (entries === 0xffff || directorySize === 0xffffffff || directoryOffset === 0xffffffff) {
    throw new Error(`${label}: ZIP64 archives are not supported by manifest validation`);
  }
  if (directoryOffset + directorySize > body.length) {
    throw new Error(`${label}: malformed ZIP central directory`);
  }
  const directory = body.subarray(directoryOffset, directoryOffset + directorySize);
  const names = new Set();
  let offset = 0;
  for (let i = 0; i < entries; i++) {
    if (offset + 46 > directory.length || directory.readUInt32LE(offset) !== 0x02014b50) {
      throw new Error(`${label}: malformed ZIP central directory`);
    }
    const nameLength = directory.readUInt16LE(offset + 28);
    const extraLength = directory.readUInt16LE(offset + 30);
    const commentLength = directory.readUInt16LE(offset + 32);
    const next = offset + 46 + nameLength + extraLength + commentLength;
    if (next > directory.length) {
      throw new Error(`${label}: malformed ZIP central directory entry`);
    }
    names.add(directory.subarray(offset + 46, offset + 46 + nameLength).toString("utf8"));
    offset = next;
  }
  return names;
}

function tarString(header, start, length) {
  return header.subarray(start, start + length).toString("utf8").replace(/\0.*$/, "");
}
