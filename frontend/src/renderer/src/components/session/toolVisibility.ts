export function isArtifactToolName(name?: string): boolean {
  return (
    name === 'visualise_show_widget' ||
    name === 'mcp_jaztools_visualise_show_widget'
  )
}

export function isHiddenToolName(name?: string): boolean {
  return (
    name === 'update_plan' ||
    name === 'visualise_read_me' ||
    name === 'mcp_jaztools_visualise_read_me'
  )
}
