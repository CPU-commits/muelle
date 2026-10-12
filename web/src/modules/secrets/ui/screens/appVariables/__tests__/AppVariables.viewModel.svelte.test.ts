import { QueryClient } from '@tanstack/svelte-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-svelte'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { UnavailableError } from '@/shared/domain'
import type { AppVariablesViewModel } from '..'
import { appOwner, type EnvKey, type EnvOwner, type EnvVar, stageOwner } from '../../../../domain'
import AppVariablesHarness from './AppVariablesHarness.svelte'

const mocks = vi.hoisted(() => ({
	getEnv: vi.fn(),
	patchEnv: vi.fn(),
}))

vi.mock('@/modules/secrets/container', () => ({
	secretsUseCases: { getEnv: mocks.getEnv, replaceEnv: vi.fn(), patchEnv: mocks.patchEnv },
}))

const at = new Date('2026-10-01T12:00:00Z')
const plain = (key: string, value: string): EnvVar => ({
	key: key as EnvKey,
	secret: false,
	value,
	updatedAt: at,
})
const secret = (key: string): EnvVar => ({
	key: key as EnvKey,
	secret: true,
	value: null,
	updatedAt: at,
})

const APP_VARS = [plain('LOG_LEVEL', 'info'), plain('PORT', '3000')]
const STAGE_VARS = [plain('LOG_LEVEL', 'debug'), secret('DATABASE_URL'), plain('TZ', 'UTC')]

function envFor(owner: EnvOwner): Promise<EnvVar[]> {
	return Promise.resolve(owner.kind === 'app' ? APP_VARS : STAGE_VARS)
}

async function mount(): Promise<AppVariablesViewModel> {
	const client = new QueryClient({
		defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
	})
	let vm: AppVariablesViewModel | undefined
	await render(AppVariablesHarness, {
		client,
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
		mocks.getEnv.mockImplementation(envFor)
		mocks.patchEnv.mockResolvedValue(undefined)
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
			owner.kind === 'stage'
				? Promise.reject(new UnavailableError('caído'))
				: Promise.resolve(APP_VARS),
		)
		const vm = await mount()

		await vi.waitFor(() => expect(vm.stageError).toBe(m.common_unavailable()))
		expect(vm.editor.loadError).toBeNull()
		expect(vm.overriddenKeys.size).toBe(0)

		mocks.getEnv.mockImplementation(envFor)
		vm.retryStage()
		await vi.waitFor(() => expect(vm.stageError).toBeNull())
		expect(vm.stageVars).toHaveLength(3)
	})
})
