// Each list ends with the name the tool had in transcripts saved before the rename.
const named = (name: string | undefined, tools: string[]) =>
  !!name && tools.some((tool) => name === tool || name === `mcp_jaztools_${tool}`)

export const isArtifactToolName = (name?: string) => named(name, ['show_artifact', 'visualise_show_widget'])

export const isHiddenToolName = (name?: string) => named(name, ['artifact_guide', 'visualise_read_me'])
