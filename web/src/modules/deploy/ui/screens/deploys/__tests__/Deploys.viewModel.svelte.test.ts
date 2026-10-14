import { beforeEach, describe, expect, it, vi } from 'vitest'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { UnavailableError } from '@/shared/domain'
import {
	asDeployId,
	asLogOffset,
	DeployNotFound,
	type DeployLogEvent,
	type FollowDeployLogInput,
} from '../../../../domain'
import { makeDeploy, makeStatus } from '../../../../domain/repositories/__fakes__/deployFixtures'
import { mountViewModel } from '../../../__tests__/mountViewModel'
import { DeploysViewModel } from '../Deploys.viewModel.svelte.ts'

const mocks = vi.hoisted(() => ({
	listDeploys: vi.fn(),
	getAppStatus: vi.fn(),
	getDeploy: vi.fn(),
	followDeployLog: vi.fn(),
	goto: vi.fn(),
}))

vi.mock('@/modules/deploy/container', () => ({
	deployUseCases: {
		getAppStatus: mocks.getAppStatus,
		listDeploys: mocks.listDeploys,
		getDeploy: mocks.getDeploy,
		createDeploy: vi.fn(),
		rollback: vi.fn(),
		followDeployLog: mocks.followDeployLog,
		streamAppLogs: vi.fn(),
	},
}))

vi.mock('$app/navigation', () => ({ goto: mocks.goto }))

const PROPS = { project: 'auditfy', stage: 'staging', app: 'api' }

const HISTORY = [
	makeDeploy(144, { status: 'running', finishedAt: null }),
	makeDeploy(143),
	makeDeploy(142, { status: 'failed', error: 'la versión nueva no pasó el healthcheck' }),
]

/** Un log guionado: los eventos y, si se pide, quedarse abierto hasta abortar. */
function scripted(events: DeployLogEvent[], { hold = false } = {}) {
	return async function* (
		_input: FollowDeployLogInput,
		signal: AbortSignal,
	): AsyncGenerator<DeployLogEvent> {
		for (const event of events) yield event
		if (hold) {
			await new Promise<void>((resolve) => signal.addEventListener('abort', () => resolve()))
		}
	}
}

const chunk = (text: string, next: number): DeployLogEvent => ({
	kind: 'chunk',
	text,
	nextOffset: asLogOffset(next),
})

describe('DeploysViewModel', () => {
	beforeEach(() => {
		vi.clearAllMocks()
		mocks.listDeploys.mockResolvedValue(HISTORY)
		mocks.getAppStatus.mockResolvedValue(makeStatus())
		mocks.goto.mockResolvedValue(undefined)
		mocks.followDeployLog.mockImplementation(scripted([]))
	})

	it('abre el deploy de ?deploy= y lee su log entero si ya terminó', async () => {
		mocks.followDeployLog.mockImplementation(
			scripted([
				chunk('2026-10-10T13:18:02Z deploy #142 encolado\n', 40),
				chunk(
					'2026-10-10T13:19:06Z deploy fallido: la versión nueva no pasó el healthcheck\n',
					90,
				),
				{ kind: 'status', status: 'failed' },
			]),
		)
		const { vm } = await mountViewModel(
			() => new DeploysViewModel(() => ({ ...PROPS, deployParam: '142' })),
		)

		await vi.waitFor(() => expect(vm.log.lines).toHaveLength(2))
		expect(vm.selected?.id).toBe(142)
		expect(mocks.followDeployLog).toHaveBeenCalledWith(
			{ app: { project: 'auditfy', stage: 'staging', app: 'api' }, id: 142, live: false },
			expect.any(AbortSignal),
		)
		expect(vm.selectedStatus).toBe('failed')
		expect(vm.log.lines[1]?.tone).toBe('error')
		expect(vm.logText).toContain('deploy #142 encolado')
		expect(vm.logLoading).toBe(false)
	})

	it('sin ?deploy= abre el más reciente y lo sigue en vivo hasta que termina', async () => {
		mocks.followDeployLog.mockImplementation(
			scripted([
				{ kind: 'status', status: 'running' },
				chunk('uno\n', 4),
				{ kind: 'status', status: 'succeeded' },
			]),
		)
		const { vm } = await mountViewModel(() => new DeploysViewModel(() => PROPS))

		await vi.waitFor(() => expect(vm.liveStatus).toBe('succeeded'))
		expect(vm.selected?.id).toBe(144)
		expect(mocks.followDeployLog.mock.calls[0]?.[0]).toMatchObject({ id: 144, live: true })
		// El historial decía `running`: el stream es más nuevo.
		expect(vm.selectedStatus).toBe('succeeded')
		expect(vm.isActive).toBe(false)
		// Y el historial se vuelve a pedir para ponerse al día.
		await vi.waitFor(() => expect(mocks.listDeploys.mock.calls.length).toBeGreaterThan(1))
	})

	it('avisa cuando el log pasa a polling', async () => {
		mocks.followDeployLog.mockImplementation(scripted([{ kind: 'polling' }], { hold: true }))
		const { vm } = await mountViewModel(() => new DeploysViewModel(() => PROPS))

		await vi.waitFor(() => expect(vm.polling).toBe(true))
		expect(vm.isActive).toBe(true)
	})

	it('elegir otro deploy corta el seguimiento anterior (sin tocar la URL)', async () => {
		const signals: AbortSignal[] = []
		mocks.followDeployLog.mockImplementation(async function* (
			_input: FollowDeployLogInput,
			signal: AbortSignal,
		) {
			signals.push(signal)
			yield* []
			await new Promise<void>((resolve) => signal.addEventListener('abort', () => resolve()))
		})
		const { vm } = await mountViewModel(() => new DeploysViewModel(() => PROPS))
		await vi.waitFor(() => expect(signals).toHaveLength(1))

		vm.setFollow(false)
		vm.select(asDeployId(143))

		await vi.waitFor(() => expect(signals).toHaveLength(2))
		expect(signals[0]?.aborted).toBe(true)
		expect(vm.selected?.id).toBe(143)
		expect(vm.follow).toBe(true)
		expect(mocks.goto).not.toHaveBeenCalled()
	})

	it('un error del log se muestra y se puede reintentar', async () => {
		mocks.followDeployLog.mockImplementation(async function* () {
			yield* []
			throw new UnavailableError('caído')
		})
		const { vm } = await mountViewModel(() => new DeploysViewModel(() => PROPS))
		await vi.waitFor(() => expect(vm.logError).toBeInstanceOf(UnavailableError))

		mocks.followDeployLog.mockImplementation(scripted([chunk('ok\n', 3)]))
		vm.retryLog()

		await vi.waitFor(() => expect(vm.log.lines).toHaveLength(1))
		expect(vm.logError).toBeNull()
	})

	it('contra un backend sin la Fase 2 muestra la fase y no abre ningún log', async () => {
		mocks.listDeploys.mockRejectedValue(
			new UnavailableError('Llega con la Fase 2', { phase: 2 }),
		)
		const { vm } = await mountViewModel(() => new DeploysViewModel(() => PROPS))

		await vi.waitFor(() => expect(vm.pendingPhase).toBe(2))
		expect(vm.loadError).toBeNull()
		expect(mocks.followDeployLog).not.toHaveBeenCalled()
	})

	it('sin deploys queda vacío', async () => {
		mocks.listDeploys.mockResolvedValue([])
		const { vm } = await mountViewModel(() => new DeploysViewModel(() => PROPS))

		await vi.waitFor(() => expect(vm.isEmpty).toBe(true))
		expect(vm.selected).toBeNull()
	})

	it('al encolar uno nuevo desde el diálogo, lo abre', async () => {
		const { vm } = await mountViewModel(() => new DeploysViewModel(() => PROPS))
		await vi.waitFor(() => expect(vm.selected?.id).toBe(144))
		mocks.listDeploys.mockResolvedValue([makeDeploy(145, { status: 'queued' }), ...HISTORY])

		vm.openDeploy()
		vm.onDeployed(asDeployId(145))
		void vm.deploys.refetch()

		expect(vm.deployOpen).toBe(false)
		await vi.waitFor(() => expect(vm.selected?.id).toBe(145))
		expect(vm.services).toEqual(['api'])
	})

	it('un ?deploy= más viejo que el historial se pide aparte y se abre', async () => {
		mocks.getDeploy.mockResolvedValue(makeDeploy(100))
		const { vm } = await mountViewModel(
			() => new DeploysViewModel(() => ({ ...PROPS, deployParam: '100' })),
		)

		await vi.waitFor(() => expect(vm.selected?.id).toBe(100))
		expect(mocks.getDeploy.mock.calls[0]?.[1]).toBe(100)
		expect(vm.linkedMissing).toBeNull()
	})

	it('un ?deploy= que ya no existe avisa y muestra el más reciente', async () => {
		mocks.getDeploy.mockRejectedValue(new DeployNotFound())
		const { vm } = await mountViewModel(
			() => new DeploysViewModel(() => ({ ...PROPS, deployParam: '9999' })),
		)

		await vi.waitFor(() => expect(vm.linkedMissing).toBe(9999))
		await vi.waitFor(() => expect(vm.selected?.id).toBe(144))
	})

	it('anuncia cuando el deploy que se sigue en vivo termina', async () => {
		mocks.followDeployLog.mockImplementation(
			scripted([
				{ kind: 'status', status: 'running' },
				{ kind: 'status', status: 'succeeded' },
			]),
		)
		const { vm } = await mountViewModel(() => new DeploysViewModel(() => PROPS))

		await vi.waitFor(() =>
			expect(vm.announcement).toBe(m.deploy_announce_succeeded({ id: 144 })),
		)
	})

	it('los chunks que llegan juntos se pintan en un solo frame', async () => {
		const many = Array.from({ length: 200 }, (_, index) => chunk(`línea ${index}\n`, index + 1))
		mocks.followDeployLog.mockImplementation(scripted(many, { hold: true }))
		const { vm } = await mountViewModel(() => new DeploysViewModel(() => PROPS))

		await vi.waitFor(() => expect(vm.log.lines).toHaveLength(200))
		expect(vm.log.lines.at(-1)?.text).toBe('línea 199')
	})

	it('desmontar la pantalla corta el stream', async () => {
		const signals: AbortSignal[] = []
		mocks.followDeployLog.mockImplementation(async function* (
			_input: FollowDeployLogInput,
			signal: AbortSignal,
		) {
			signals.push(signal)
			yield* []
			await new Promise<void>((resolve) => signal.addEventListener('abort', () => resolve()))
		})
		const { unmount } = await mountViewModel(() => new DeploysViewModel(() => PROPS))
		await vi.waitFor(() => expect(signals).toHaveLength(1))

		unmount()

		expect(signals[0]?.aborted).toBe(true)
	})

	it('si el backend no reconoce el offset, el log se vuelve a leer de cero sin repetir', async () => {
		mocks.followDeployLog.mockImplementation(
			scripted([chunk('uno\n', 4), { kind: 'reset' }, chunk('uno\ndos\n', 8)]),
		)
		const { vm } = await mountViewModel(() => new DeploysViewModel(() => PROPS))

		await vi.waitFor(() =>
			expect(vm.log.lines.map((line) => line.text)).toEqual(['uno', 'dos']),
		)
	})
})
