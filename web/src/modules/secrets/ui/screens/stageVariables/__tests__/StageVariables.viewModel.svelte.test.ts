import { QueryClient } from '@tanstack/svelte-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-svelte'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { toasts } from '@/shared/ui/stores'
import type { StageVariablesViewModel } from '..'
import { type EnvKey, type EnvVar, stageOwner } from '../../../../domain'
import StageVariablesHarness from './StageVariablesHarness.svelte'

const mocks = vi.hoisted(() => ({
	getEnv: vi.fn(),
	patchEnv: vi.fn(),
}))

vi.mock('@/modules/secrets/container', () => ({
	secretsUseCases: { getEnv: mocks.getEnv, replaceEnv: vi.fn(), patchEnv: mocks.patchEnv },
}))

const STAGE_VARS: EnvVar[] = [
	{
		key: 'TZ' as EnvKey,
		secret: false,
		value: 'UTC',
		updatedAt: new Date('2026-10-01T12:00:00Z'),
	},
]

async function mount(): Promise<StageVariablesViewModel> {
	const client = new QueryClient({
		defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
	})
	let vm: StageVariablesViewModel | undefined
	await render(StageVariablesHarness, {
		client,
		params: { project: 'auditfy', stage: 'staging' },
		onReady: (instance) => {
			vm = instance
		},
	})
	if (!vm) throw new Error('el harness no expuso el viewModel')
	const ready = vm
	await vi.waitFor(() => expect(ready.editor.hasLoaded).toBe(true))
	return ready
}

describe('StageVariablesViewModel', () => {
	beforeEach(() => {
		vi.clearAllMocks()
		toasts.items = []
		mocks.getEnv.mockResolvedValue(STAGE_VARS)
		mocks.patchEnv.mockResolvedValue(undefined)
	})

	it('el dueño es el stage, y un cambio deja pendientes a sus apps', async () => {
		const vm = await mount()
		const staging = stageOwner('auditfy', 'staging')
		expect(mocks.getEnv).toHaveBeenCalledWith(staging, expect.anything())
		expect(vm.editor.kind).toBe('stage')

		vm.editor.setNewKey('SENTRY_DSN')
		vm.editor.setNewValue('https://x')
		await vm.editor.submitAdd()

		expect(mocks.patchEnv).toHaveBeenCalledWith(
			expect.objectContaining({ owner: staging, set: [['SENTRY_DSN', 'https://x']] }),
		)
		expect(vm.hasPendingChanges).toBe(true)
		expect(toasts.items.at(-1)).toMatchObject({
			description: m.secrets_saved_key_stage({ key: 'SENTRY_DSN' }),
		})
	})
})
