import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-svelte'
import { createQueryClient } from '@/shared/data/query'
import type { AuditEvent, AuditEventId, AuditPage } from '../../../../domain'
import AuditLogView from './AuditLogView.svelte'

const mocks = vi.hoisted(() => ({ listAudit: vi.fn() }))

vi.mock('@/modules/admin/container', () => ({
	adminUseCases: {
		getSettings: vi.fn(),
		updateSecurity: vi.fn(),
		updatePublicIp: vi.fn(),
		updateAcmeEmail: vi.fn(),
		updateProxyPorts: vi.fn(),
		replaceCloudflareToken: vi.fn(),
		removeCloudflareToken: vi.fn(),
		listAudit: mocks.listAudit,
		listRegistries: vi.fn(),
		addRegistry: vi.fn(),
		updateRegistry: vi.fn(),
		removeRegistry: vi.fn(),
	},
}))

function event(id: number, overrides: Partial<AuditEvent> = {}): AuditEvent {
	return {
		id: id as AuditEventId,
		actor: { kind: 'token', id: 2, name: 'claude-staging' },
		action: 'deploy.create',
		target: 'app:auditfy/staging/api',
		viaMcp: false,
		at: new Date('2026-10-10T14:27:00Z'),
		...overrides,
	}
}

const PAGE: AuditPage = {
	items: [
		// Un deploy que hizo Claude Code con el servidor MCP (contrato §8.6).
		event(2, { viaMcp: true }),
		event(1, { actor: { kind: 'token', id: 1, name: 'github-actions' } }),
	],
	nextCursor: null,
}

describe('AuditLog', () => {
	beforeEach(() => {
		vi.clearAllMocks()
		mocks.listAudit.mockResolvedValue(PAGE)
	})

	it('en "Quién", lo que hizo un token con el servidor MCP dice "vía MCP"', async () => {
		const screen = await render(AuditLogView, { client: createQueryClient({ retry: false }) })

		const rows = screen.getByRole('row')
		await expect.element(rows.nth(1)).toHaveTextContent('claude-staging')
		const who = (row: number) => rows.nth(row).getByRole('cell').nth(1)

		await expect.element(who(1)).toHaveTextContent(/token\s*claude-staging\s*vía MCP/)
		await expect.element(who(2)).toHaveTextContent('github-actions')
		await expect.element(who(2)).not.toHaveTextContent('MCP')
	})
})
