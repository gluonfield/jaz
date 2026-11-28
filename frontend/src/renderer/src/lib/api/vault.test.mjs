import { afterEach, expect, test } from 'bun:test'

if (typeof globalThis.window === 'undefined') {
  globalThis.window = {
    location: { origin: 'http://127.0.0.1:4096', protocol: 'http:', href: 'http://127.0.0.1:4096/' },
    localStorage: {
      getItem: () => null,
      setItem: () => {},
      removeItem: () => {},
    },
    addEventListener: () => {},
    removeEventListener: () => {},
  }
}

const {
  createVaultItem,
  deleteVaultItem,
  listVaultItems,
  updateVaultItem,
} = await import('./vault')
const { hasPermissionSurface, isPlanApprovalPermission } = await import('../sessionPermissions')

const originalFetch = globalThis.fetch

afterEach(() => {
  globalThis.fetch = originalFetch
})

test('vault API client forwards Cloud workspace headers and handles CRUD + CSV import', async () => {
  const seen = []
  globalThis.fetch = async (url, init) => {
    const parsed = new URL(String(url))
    const method = init?.method || 'GET'
    expect(new Headers(init?.headers).get('X-Jaz-Cloud-Workspace')).toBe('ws_cloud_1')
    seen.push(`${method} ${parsed.pathname}${parsed.search}`)

    if (method === 'GET' && parsed.pathname === '/v1/vault/items') {
      return Response.json({
        items: [
          {
            id: 'vlt_1',
            kind: 'login',
            label: 'GitHub',
            origins: ['https://github.com'],
            username: 'octocat',
            created_at: '2026-11-27T00:00:00Z',
            updated_at: '2026-11-27T00:00:00Z',
          },
        ],
      })
    }
    if (method === 'POST' && parsed.pathname === '/v1/vault/items') {
      const body = JSON.parse(init.body)
      expect(body.pan).toBe('4242424242424242')
      expect(body.cvv).toBe('987')
      return Response.json({
        item: {
          id: 'vlt_card_1',
          kind: 'card',
          label: 'Visa',
          card_brand: 'Visa',
          card_last4: '4242',
          created_at: '2026-11-27T00:00:00Z',
          updated_at: '2026-11-27T00:00:00Z',
        },
      })
    }
    if (method === 'PATCH' && parsed.pathname === '/v1/vault/items/vlt_card_1') {
      return Response.json({
        item: {
          id: 'vlt_card_1',
          kind: 'card',
          label: 'Personal Visa',
          created_at: '2026-11-27T00:00:00Z',
          updated_at: '2026-11-27T00:00:00Z',
        },
      })
    }
    if (method === 'DELETE' && parsed.pathname === '/v1/vault/items/vlt_card_1') {
      return Response.json({ deleted: true })
    }
    return new Response('not found', { status: 404 })
  }

  const items = await listVaultItems({ kind: 'login', domain: 'github.com', workspaceID: 'ws_cloud_1' })
  expect(items).toHaveLength(1)
  expect(items[0].id).toBe('vlt_1')

  const created = await createVaultItem(
    {
      kind: 'card',
      label: 'Visa',
      pan: '4242424242424242',
      card_exp_month: 12,
      card_exp_year: 2030,
      cvv: '987',
    },
    'ws_cloud_1',
  )
  expect(created.card_last4).toBe('4242')

  const updated = await updateVaultItem('vlt_card_1', { label: 'Personal Visa' }, 'ws_cloud_1')
  expect(updated.label).toBe('Personal Visa')

  await deleteVaultItem('vlt_card_1', 'ws_cloud_1')
  expect(seen).toEqual([
    'GET /v1/vault/items?kind=login&domain=github.com',
    'POST /v1/vault/items',
    'PATCH /v1/vault/items/vlt_card_1',
    'DELETE /v1/vault/items/vlt_card_1',
  ])
})

test('listVaultItems treats 404 from older Cloud backends as empty list', async () => {
  globalThis.fetch = async () => new Response('not found', { status: 404 })
  const items = await listVaultItems({ workspaceID: 'ws_old' })
  expect(items).toEqual([])
})

test('sessionPermissions keeps vault approval and vault request cards out of plan approval suppression', () => {
  const approval = {
    id: 'perm-vault-1',
    content: 'Checkout summary',
    vault_approval: {
      item_id: 'vlt_card_1',
      kind: 'card',
      label: 'Personal Visa',
      card_brand: 'Visa',
      card_last4: '4242',
      merchant_origin: 'https://checkout.example.com',
    },
    options: [
      { id: 'approve_once', name: 'Approve once', kind: 'allow_once' },
      { id: 'deny', name: 'Deny', kind: 'reject_once' },
    ],
  }
  const request = {
    id: 'perm-vault-2',
    vault_request: {
      kind: 'login',
      domain: 'https://github.com',
      label: 'GitHub',
    },
    options: [{ id: 'deny', name: 'Cancel', kind: 'reject_once' }],
  }

  expect(isPlanApprovalPermission(approval)).toBe(false)
  expect(isPlanApprovalPermission(request)).toBe(false)
  expect(hasPermissionSurface(approval)).toBe(true)
  expect(hasPermissionSurface(request)).toBe(true)
})
