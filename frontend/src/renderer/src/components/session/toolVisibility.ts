export function isArtifactToolName(name?: string): boolean {
  return (
    name === 'visualize_show_widget' ||
    name === 'visualize:show_widget' ||
    name === 'mcp_jaztools_visualize_show_widget'
  )
}

export function isHiddenToolName(name?: string): boolean {
  return (
    name === 'update_plan' ||
    name === 'visualize_read_me' ||
    name === 'visualize:read_me' ||
    name === 'mcp_jaztools_visualize_read_me'
  )
}
