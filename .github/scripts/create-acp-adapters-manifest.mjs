import { execFileSync } from "node:child_process";
import { mkdirSync, writeFileSync } from "node:fs";
import { dirname } from "node:path";

const out = process.argv[2] || "dist/acp-adapters.json";

const codexVersion = "0.16.7";
const claudeVersion = "0.50.0-jaz.1";

const adapters = {
  codex: {
    repo: "gluonfield/codex-acp",
    tag: `v${codexVersion}`,
    version: codexVersion,
    assets: [
      ["darwin-arm64", `codex-acp-${codexVersion}-aarch64-apple-darwin.tar.gz`, "codex-acp"],
      ["darwin-x64", `codex-acp-${codexVersion}-x86_64-apple-darwin.tar.gz`, "codex-acp"],
      ["linux-arm64", `codex-acp-${codexVersion}-aarch64-unknown-linux-gnu.tar.gz`, "codex-acp"],
      ["linux-x64", `codex-acp-${codexVersion}-x86_64-unknown-linux-gnu.tar.gz`, "codex-acp"],
      ["win32-arm64", `codex-acp-${codexVersion}-aarch64-pc-windows-msvc.tar.gz`, "codex-acp.exe"],
      ["win32-x64", `codex-acp-${codexVersion}-x86_64-pc-windows-msvc.tar.gz`, "codex-acp.exe"],
    ],
  },
  claude: {
    repo: "gluonfield/claude-agent-acp",
    tag: `v${claudeVersion}`,
    version: claudeVersion,
    assets: [
      ["darwin-arm64", `claude-agent-acp-${claudeVersion}-darwin-arm64.tar.gz`, "claude-agent-acp", "claude"],
      ["darwin-x64", `claude-agent-acp-${claudeVersion}-darwin-x64.tar.gz`, "claude-agent-acp", "claude"],
      ["linux-arm64", `claude-agent-acp-${claudeVersion}-linux-arm64.tar.gz`, "claude-agent-acp", "claude"],
      ["linux-x64", `claude-agent-acp-${claudeVersion}-linux-x64.tar.gz`, "claude-agent-acp", "claude"],
      ["win32-x64", `claude-agent-acp-${claudeVersion}-win32-x64.tar.gz`, "claude-agent-acp.exe", "claude.exe"],
    ],
  },
};

const manifest = {
  adapters: {},
};

for (const [name, spec] of Object.entries(adapters)) {
  const assets = releaseAssetMap(spec.repo, spec.tag);
  manifest.adapters[name] = { version: spec.version, assets: {} };
  for (const [platform, filename, binary, envBinary] of spec.assets) {
    const asset = assets.get(filename);
    if (!asset) {
      throw new Error(`${spec.repo}@${spec.tag} is missing ${filename}`);
    }
    const sha256 = digestSHA256(asset);
    manifest.adapters[name].assets[platform] = {
      url: asset.url,
      sha256,
      binary,
      ...(envBinary ? { env: { CLAUDE_CODE_EXECUTABLE: envBinary } } : {}),
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
