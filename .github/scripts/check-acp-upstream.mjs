import { execFileSync } from "node:child_process"
import { appendFileSync, readFileSync } from "node:fs"

const spec = JSON.parse(readFileSync(".github/acp-adapter-assets.json", "utf8"))
const verified = JSON.parse(readFileSync(".github/acp-verified-native-versions.json", "utf8"))
const rows = []
const adapters = {
  codex: ["@agentclientprotocol/codex-acp", "package.json", "@openai/codex"],
  claude: ["@agentclientprotocol/claude-agent-acp", "package.json", "@anthropic-ai/claude-agent-sdk"],
  kimi: ["@moonshot-ai/kimi-code", "apps/kimi-code/package.json"],
}

await Promise.all(Object.entries(adapters).map(async ([name, [upstream, manifestPath, runtime]]) => {
  await check(name, async () => {
    const adapter = spec.adapters[name]
    const pkg = JSON.parse(execFileSync("gh", ["api", `repos/${adapter.repo}/contents/${manifestPath}?ref=${adapter.tag}`, "-H", "Accept: application/vnd.github.raw+json"], { encoding: "utf8", timeout: 30_000 }))
    compare(name, pkg.version.replace(/-jaz\..*$/, ""), await npmVersion(upstream))
    if (runtime) {
      compare(`${name} runtime`, pkg.dependencies[runtime], await npmVersion(runtime))
    }
  })
}))
await Promise.all([
  ...["antigravity", "muse"].map((name) => check(`${name} adapter`, async () => {
    const { repo, tag } = spec.adapters[name]
    const latest = execFileSync("gh", ["api", `repos/${repo}/releases/latest`, "--jq", ".tag_name"], { encoding: "utf8", timeout: 30_000 }).trim()
    compare(`${name} adapter`, tag.replace(/^v/, ""), latest.replace(/^v/, ""))
  })),
  check("muse runtime", async () => compare("muse runtime (last verified)", verified.muse, (await json("https://api.meta.ai/muse-code/channels/muse-stable")).version)),
  check("opencode", async () => compare("opencode", spec.adapters.opencode.tag.replace(/^v/, ""), await npmVersion("@opencode/cli"))),
  check("copilot", async () => compare("copilot", spec.adapters.copilot.tag.replace(/^v/, ""), await npmVersion("@github/copilot"))),
  check("grok", async () => compare("grok (last verified)", verified.grok, await npmVersion("@xai-official/grok"))),
])

rows.sort((a, b) => a.name.localeCompare(b.name))
const report = [
  "| Component | Bundled / verified | Latest upstream | Status |",
  "| --- | --- | --- | --- |",
  ...rows.map((row) => `| ${row.name} | ${row.current ?? "unknown"} | ${row.latest ?? "unknown"} | ${row.status} |`),
  "",
  "Grok and Muse use native updaters; their baseline is the last native version verified. Runtime changes require native-parity checks before release.",
].join("\n")
console.log(report)
if (process.env.GITHUB_STEP_SUMMARY) {
  appendFileSync(process.env.GITHUB_STEP_SUMMARY, `${report}\n`)
}
if (rows.some((row) => row.status !== "current")) {
  process.exitCode = 1
}

function compare(name, current, latest) {
  rows.push({ name, current, latest, status: current && current === latest ? "current" : "review required" })
}

async function check(name, action) {
  try {
    await action()
  } catch (error) {
    rows.push({ name, status: "lookup failed" })
    console.error(`${name}: ${error.message}`)
  }
}

async function npmVersion(name) {
  return (await json(`https://registry.npmjs.org/${encodeURIComponent(name)}/latest`)).version
}

async function json(url) {
  const response = await fetch(url, { headers: { "User-Agent": "Mozilla/5.0" }, signal: AbortSignal.timeout(30_000) })
  if (!response.ok) {
    throw new Error(`${url}: HTTP ${response.status}`)
  }
  return response.json()
}
