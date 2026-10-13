import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { UnavailableError } from '@/shared/domain'
import { asDeployId } from '../../../../domain'
import {
	makeDeploy,
	makeService,
	makeStatus,
} from '../../../../domain/repositories/__fakes__/deployFixtures'
import { mountViewModel } from '../../../__tests__/mountViewModel'
import { SummaryViewModel } from '../Summary.viewModel.svelte.ts'

const mocks = vi.hoisted(() => ({
	getAppStatus: vi.fn(),
	listDeploys: vi.fn(),
	goto: vi.fn(),
}))

// Se mockea el CONTAINER, no la red: el test mira lo que hace el viewModel con lo que
// devuelven los casos de uso.
vi.mock('@/modules/deploy/container', () => ({
	deployUseCases: {
		getAppStatus: mocks.getAppStatus,
		listDeploys: mocks.listDeploys,
		getDeploy: vi.fn(),
		createDeploy: vi.fn(),
		rollback: vi.fn(),
		followDeployLog: vi.fn(),
		streamAppLogs: vi.fn(),
	},
}))

vi.mock('$app/navigation', () => ({ goto: mocks.goto }))

const PROPS = { project: 'auditfy', stage: 'staging', app: 'api' }

function setVisibility(state: 'visible' | 'hidden'): void {
	Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => state })
	document.dispatchEvent(new Event('visibilitychange'))
}

describe('SummaryViewModel', () => {
	beforeEach(() => {
		vi.clearAllMocks()
		mocks.getAppStatus.mockResolvedValue(makeStatus())
		mocks.listDeploys.mockResolvedValue([
			makeDeploy(143),
			makeDeploy(142, { status: 'failed' }),
			makeDeploy(140, {
				images: [{ service: 'api', ref: 'ghcr.io/auditfy/api:sha-e52b8f0', digest: null }],
			}),
		])
		mocks.goto.mockResolvedValue(undefined)
	})

	afterEach(() => {
		// Vuelve al getter del prototipo.
		Reflect.deleteProperty(document, 'visibilityState')
	})

	it('arma el estado, el deploy actual y el destino del rollback', async () => {
		const { vm } = await mountViewModel(() => new SummaryViewModel(() => PROPS))

		await vi.waitFor(() => expect(vm.health).toBe('running'))
		await vi.waitFor(() => expect(vm.current?.id).toBe(143))
		expect(vm.stack).toBe('auditfy_staging_api')
		expect(vm.network).toBe('auditfy_staging')
		expect(vm.swarmName('api')).toBe('auditfy_staging_api_api')
		expect(vm.last?.id).toBe(143)
		expect(vm.rollbackTarget?.id).toBe(140)
		expect(vm.historyComplete).toBe(true)
		expect(vm.imageOf('api')?.ref).toBe('ghcr.io/auditfy/api:sha-3f9c2e1')
		expect(vm.runningTasks).toBe(2)
		expect(vm.actionsEnabled).toBe(true)
	})

	it('se refresca cada 5 s solo con la pestaña visible, y al volver pide enseguida', async () => {
		setVisibility('visible')
		const { vm } = await mountViewModel(() => new SummaryViewModel(() => PROPS))
		await vi.waitFor(() => expect(vm.health).toBe('running'))
		expect(vm.refreshInterval).toBe(5000)

		setVisibility('hidden')
		expect(vm.visible).toBe(false)
		expect(vm.refreshInterval).toBe(false)

		const before = mocks.getAppStatus.mock.calls.length
		setVisibility('visible')
		expect(vm.refreshInterval).toBe(5000)
		await vi.waitFor(() => expect(mocks.getAppStatus.mock.calls.length).toBe(before + 1))
	})

	it('contra un backend sin la Fase 2 muestra la fase, no un error', async () => {
		mocks.getAppStatus.mockRejectedValue(
			new UnavailableError('Llega con la Fase 2', { phase: 2 }),
		)
		mocks.listDeploys.mockRejectedValue(
			new UnavailableError('Llega con la Fase 2', { phase: 2 }),
		)
		const { vm } = await mountViewModel(() => new SummaryViewModel(() => PROPS))

		await vi.waitFor(() => expect(vm.pendingPhase).toBe(2))
		expect(vm.loadError).toBeNull()
		expect(vm.actionsEnabled).toBe(false)
	})

	it('un backend caído es un error con reintento', async () => {
		mocks.getAppStatus.mockRejectedValue(new UnavailableError('caído'))
		const { vm } = await mountViewModel(() => new SummaryViewModel(() => PROPS))

		await vi.waitFor(() => expect(vm.loadError).toBeInstanceOf(UnavailableError))
		expect(vm.pendingPhase).toBeNull()

		mocks.getAppStatus.mockResolvedValue(makeStatus())
		vm.refresh()
		await vi.waitFor(() => expect(vm.health).toBe('running'))
	})

	it('una app sin servicios en Swarm es not_deployed', async () => {
		mocks.getAppStatus.mockResolvedValue(makeStatus([]))
		mocks.listDeploys.mockResolvedValue([])
		const { vm } = await mountViewModel(() => new SummaryViewModel(() => PROPS))

		await vi.waitFor(() => expect(vm.health).toBe('not_deployed'))
		expect(vm.current).toBeNull()
	})

	it('una réplica caída es degraded', async () => {
		mocks.getAppStatus.mockResolvedValue(makeStatus([makeService({ runningReplicas: 1 })]))
		const { vm } = await mountViewModel(() => new SummaryViewModel(() => PROPS))
		await vi.waitFor(() => expect(vm.health).toBe('degraded'))
	})

	it('los datos del catálogo llegan por props (F2)', async () => {
		const { vm } = await mountViewModel(
			() =>
				new SummaryViewModel(() => ({
					...PROPS,
					kind: 'service',
					pendingChanges: true,
					pendingChangesHref: '/projects/auditfy/stages/staging/apps/api/variables',
				})),
		)
		expect(vm.kind).toBe('service')
		expect(vm.pendingChanges).toBe(true)
		expect(vm.pendingChangesHref).toContain('/variables')
	})

	it('al encolar un deploy va a Deploys, que abre el más reciente (ese)', async () => {
		const { vm } = await mountViewModel(() => new SummaryViewModel(() => PROPS))
		vm.openDeploy()
		expect(vm.deployOpen).toBe(true)

		await vm.onQueued()

		expect(vm.deployOpen).toBe(false)
		expect(mocks.goto).toHaveBeenCalledWith('/projects/auditfy/stages/staging/apps/api/deploys')
		// El link al último deploy sí lleva el id.
		expect(vm.deployHref(asDeployId(143))).toBe(
			'/projects/auditfy/stages/staging/apps/api/deploys?deploy=143',
		)
	})

	it('el healthcheck del catálogo llega por props', async () => {
		const { vm } = await mountViewModel(
			() =>
				new SummaryViewModel(() => ({ ...PROPS, healthcheck: 'GET /health · cada 10 s' })),
		)
		expect(vm.healthcheck).toBe('GET /health · cada 10 s')
	})
})
