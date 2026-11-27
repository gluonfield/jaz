import { OrgAPIKeysSection } from './OrgAPIKeysSection'
import { OrgIdentitySection } from './OrgIdentitySection'
import { OrgPluginsSection } from './OrgPluginsSection'

// Org-wide settings, shown only to org admins of the active Cloud workspace.
export function AdminSettings() {
  return (
    <section className="flex flex-col gap-7 py-4">
      <h1 className="text-lg font-semibold text-ink">Admin</h1>
      <OrgIdentitySection />
      <OrgAPIKeysSection />
      <OrgPluginsSection />
    </section>
  )
}
