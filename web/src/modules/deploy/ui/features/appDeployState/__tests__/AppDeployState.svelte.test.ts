import { beforeEach, describe, expect, it, vi } from 'vitest'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { UnavailableError } from '@/shared/domain'
import {
	APP,
	makeDeploy,
	makeService,
	makeStatus,
} from '../../../../domain/repositories/__fakes__/deployFixtures'
import { mountViewModel } from '../../../__tests__/mountViewModel'
import { AppDeployState } from '../AppDeployState.svelte.ts'

const mocks = vi.hoisted(() => ({ getAppStatus: vi.fn(), listDeploys: vi.fn() }))

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

describe('AppDeployState (las columnas de Proyectos para F2)', () => {
	beforeEach(() => {
		vi.clearAllMocks()
		mocks.getAppStatus.mockResolvedValue(makeStatus())
		mocks.listDeploys.mockResolvedValue([makeDeploy(143)])
	})

	it('"Corriendo · 2/2", la imagen actual y "#143 · …"', async () => {
		const { vm } = await mountViewModel(() => new AppDeployState(() => APP))

		await vi.waitFor(() => expect(vm.isLoading).toBe(false))
		expect(vm.tone).toBe('ok')
		expect(vm.statusText).toBe(
			m.deploy_row_status({ health: m.deploy_health_running(), running: 2, desired: 2 }),
		)
		expect(vm.lastDeployText).toMatch(/^#143 · /)
		expect(vm.lastDeployFailed).toBe(false)
		expect(vm.image).toBe('ghcr.io/auditfy/api:sha-3f9c2e1')
		expect(vm.imageShort).toBe('api:sha-3f9c2e1')
	})

	it('una compose caída nombra el servicio que falta y lista sus servicios', async () => {
		mocks.getAppStatus.mockResolvedValue(
			makeStatus([
				makeService({ name: 'gateway', desiredReplicas: 1, runningReplicas: 0 }),
				makeService({ name: 'runner', desiredReplicas: 1, runningReplicas: 0 }),
			]),
		)
		mocks.listDeploys.mockResolvedValue([
			makeDeploy(23, {
				images: [
					{ service: 'gateway', ref: 'x/gateway:v1', digest: null },
					{ service: 'runner', ref: 'x/runner:v1', digest: null },
				],
			}),
		])
		const { vm } = await mountViewModel(() => new AppDeployState(() => APP))

		await vi.waitFor(() => expect(vm.health).toBe('down'))
		expect(vm.tone).toBe('bad')
		expect(vm.statusText).toBe(
			m.deploy_row_status_service({
				health: m.deploy_health_down(),
				service: 'gateway',
				running: 0,
				desired: 1,
			}),
		)
		expect(vm.image).toBe('gateway · runner')
	})

	it('si el último falló lo marca, y la imagen sigue siendo la que corre', async () => {
		mocks.listDeploys.mockResolvedValue([
			makeDeploy(58, {
				status: 'failed',
				images: [{ service: 'api', ref: 'x/api:rota', digest: null }],
			}),
			makeDeploy(57),
		])
		const { vm } = await mountViewModel(() => new AppDeployState(() => APP))

		await vi.waitFor(() => expect(vm.lastDeployFailed).toBe(true))
		expect(vm.lastDeployText).toMatch(/^#58 · /)
		expect(vm.lastDeployFailedText).toBe(m.deploy_row_failed())
		expect(vm.image).toBe('ghcr.io/auditfy/api:sha-3f9c2e1')
	})

	it('sin deploys dice "Sin deploys" y "Sin desplegar"', async () => {
		mocks.getAppStatus.mockResolvedValue(makeStatus([]))
		mocks.listDeploys.mockResolvedValue([])
		const { vm } = await mountViewModel(() => new AppDeployState(() => APP))

		await vi.waitFor(() => expect(vm.isLoading).toBe(false))
		expect(vm.statusText).toBe(m.deploy_health_not_deployed())
		expect(vm.lastDeployText).toBe(m.deploy_row_no_deploys())
		expect(vm.tone).toBe('idle')
		expect(vm.image).toBeNull()
	})

	it('sin la Fase 2 (o con un error) todo es "—"', async () => {
		const phase = new UnavailableError('Llega con la Fase 2', { phase: 2 })
		mocks.getAppStatus.mockRejectedValue(phase)
		mocks.listDeploys.mockRejectedValue(phase)
		const { vm } = await mountViewModel(() => new AppDeployState(() => APP))

		await vi.waitFor(() => expect(vm.statusUnavailable).toBe(true))
		expect(vm.statusText).toBe(m.deploy_none())
		expect(vm.lastDeployText).toBe(m.deploy_none())
		expect(vm.tone).toBe('idle')
		expect(vm.image).toBeNull()
	})

	it('sin historial (falló o no hay un exitoso), la imagen sale de lo que corre en Swarm', async () => {
		mocks.listDeploys.mockRejectedValue(new UnavailableError('caído'))
		const { vm } = await mountViewModel(() => new AppDeployState(() => APP))

		await vi.waitFor(() => expect(vm.tone).toBe('ok'))
		expect(vm.image).toBe('ghcr.io/auditfy/api:sha-3f9c2e1')
		expect(vm.imageShort).toBe('api:sha-3f9c2e1')
	})

	it('con enabled en false no pide nada y dice "—"; al prenderse carga', async () => {
		let enabled = $state(false)
		const { vm } = await mountViewModel(
			() =>
				new AppDeployState(
					() => APP,
					() => enabled,
				),
		)

		await new Promise((resolve) => setTimeout(resolve, 50))
		expect(mocks.getAppStatus).not.toHaveBeenCalled()
		expect(mocks.listDeploys).not.toHaveBeenCalled()
		expect(vm.isLoading).toBe(false)
		expect(vm.statusText).toBe(m.deploy_none())
		expect(vm.lastDeployText).toBe(m.deploy_none())

		enabled = true
		await vi.waitFor(() => expect(vm.isLoading).toBe(false))
		await vi.waitFor(() => expect(vm.tone).toBe('ok'))
		expect(mocks.getAppStatus).toHaveBeenCalledTimes(1)
	})
})
