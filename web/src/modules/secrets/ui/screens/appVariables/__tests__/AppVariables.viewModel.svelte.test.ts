import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-svelte'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { UnavailableError } from '@/shared/domain'
import type { AppVariablesViewModel } from '..'
import { appOwner, type EnvOwner, makeGetEnv, makePatchEnv, stageOwner } from '../../../../domain'
import { InMemoryEnvRepository } from '../../../../domain/repositories/__fakes__/inMemoryEnvRepository'
import AppVariablesHarness from './AppVariablesHarness.svelte'

const mocks = vi.hoisted(() => ({
	getEnv: vi.fn(),
	patchEnv: vi.fn(),
}))

vi.mock('@/modules/secrets/container', () => ({
	secretsUseCases: { getEnv: mocks.getEnv, replaceEnv: vi.fn(), patchEnv: mocks.patchEnv },
}))

let fake: InMemoryEnvRepository
let getEnv: ReturnType<typeof makeGetEnv>

function seedFake(): InMemoryEnvRepository {
	return new InMemoryEnvRepository({
		owners: {
			'app:auditfy/staging/api': [
				{ key: 'LOG_LEVEL', value: 'info', secret: false },
				{ key: 'PORT', value: '3000', secret: false },
			],
			'stage:auditfy/staging': [
				{ key: 'LOG_LEVEL', value: 'debug', secret: false },
				{ key: 'DATABASE_URL', value: 'postgres://db', secret: true },
				{ key: 'TZ', value: 'UTC', secret: false },
			],
		},
		now: () => new Date('2026-10-05T12:00:00.000Z'),
	})
}

async function mount(): Promise<AppVariablesViewModel> {
	let vm: AppVariablesViewModel | undefined
	await render(AppVariablesHarness, {
		params: { project: 'auditfy', stage: 'staging', app: 'api' },
		onReady: (instance) => {
			vm = instance
		},
	})
	if (!vm) throw new Error('el harness no expuso el viewModel')
	return vm
}

describe('AppVariablesViewModel', () => {
	beforeEach(() => {
		vi.clearAllMocks()
		fake = seedFake()
		getEnv = makeGetEnv({ env: fake })
		mocks.getEnv.mockImplementation(getEnv)
		mocks.patchEnv.mockImplementation(makePatchEnv({ env: fake }))
	})

	it('carga las de la app y las del stage, y marca las que la app pisa', async () => {
		const vm = await mount()

		await vi.waitFor(() => expect(vm.stageVars).toHaveLength(3))
		await vi.waitFor(() => expect(vm.editor.count).toBe(2))
		expect(mocks.getEnv).toHaveBeenCalledWith(
			appOwner('auditfy', 'staging', 'api'),
			expect.anything(),
		)
		expect(mocks.getEnv).toHaveBeenCalledWith(
			stageOwner('auditfy', 'staging'),
			expect.anything(),
		)
		expect([...vm.overriddenKeys]).toEqual(['LOG_LEVEL'])
		expect(vm.editor.overridden.has('LOG_LEVEL')).toBe(true)
		expect(vm.stage).toBe('staging')
	})

	it('los links van al stage y al Resumen de la app', async () => {
		const vm = await mount()

		expect(vm.stageHref).toBe('/projects/auditfy/stages/staging/variables')
		expect(vm.summaryHref).toBe('/projects/auditfy/stages/staging/apps/api')
	})

	it('después de guardar, la app queda con cambios pendientes', async () => {
		const vm = await mount()
		await vi.waitFor(() => expect(vm.editor.hasLoaded).toBe(true))
		expect(vm.hasPendingChanges).toBe(false)

		vm.editor.setNewKey('NEW')
		await vm.editor.submitAdd()

		expect(vm.hasPendingChanges).toBe(true)
	})

	it('si fallan solo las del stage, las de la app siguen y el error es del stage', async () => {
		mocks.getEnv.mockImplementation((owner: EnvOwner) =>
			owner.kind === 'stage' ? Promise.reject(new UnavailableError('caído')) : getEnv(owner),
		)
		const vm = await mount()

		await vi.waitFor(() => expect(vm.stageError).toBe(m.common_unavailable()))
		expect(vm.editor.loadError).toBeNull()
		expect(vm.overriddenKeys.size).toBe(0)

		mocks.getEnv.mockImplementation(getEnv)
		vm.retryStage()
		// Sin datos, TanStack vuelve a `pending` al reintentar: se espera a que lleguen.
		await vi.waitFor(() => expect(vm.stageVars).toHaveLength(3))
		expect(vm.stageError).toBeNull()
	})
})
