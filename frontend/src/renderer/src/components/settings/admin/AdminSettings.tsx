import { OrgAPIKeysSection } from './OrgAPIKeysSection'
import { OrgIdentitySection } from './OrgIdentitySection'
import { OrgPluginsSection } from './OrgPluginsSection'

// Org-wide settings, shown only to org admins of the active Cloud workspace.
export function AdminSettings() {
  return (
    <section className="flex flex-col gap-7 py-4">
      <div>
        <h1 className="text-lg font-semibold text-ink">Admin</h1>
        <p className="mt-0.5 text-[13px] text-ink-2">Settings here apply to every member of this organization.</p>
      </div>
      <OrgIdentitySection />
      <OrgAPIKeysSection />
      <OrgPluginsSection />
    </section>
  )
}
