import { beforeEach, describe, expect, it, vi } from 'vitest'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { UnavailableError } from '@/shared/domain'
import { appRefFrom, catalogKeys } from '@/modules/catalog'
import type { ProxyStatus } from '@/modules/ingress'
import {
	asDeployId,
	asLogOffset,
	DeployNotFound,
	type DeployLogEvent,
	type FollowDeployLogInput,
} from '../../../../domain'
import { makeDeploy, makeStatus } from '../../../../domain/repositories/__fakes__/deployFixtures'
import { mountViewModel, testClient } from '../../../__tests__/mountViewModel'
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

// El motivo de la advertencia del paso 7 sale de GET /proxy (ingress, contrato §2.11).
const ingress = vi.hoisted(() => ({ getProxyStatus: vi.fn() }))
vi.mock('@/modules/ingress/container', () => ({ ingressUseCases: ingress }))

const CADDY = {
	image: 'ghcr.io/cpu-commits/muelle-caddy:v0.1.0',
	running: true,
	httpPort: 80,
	httpsPort: 443,
}

function proxyStatus(overrides: Partial<ProxyStatus> = {}): ProxyStatus {
	return {
		status: 'synced',
		error: null,
		syncedAt: new Date(),
		caddy: CADDY,
		portsRollback: null,
		...overrides,
	}
}

const CADDY_WARNING =
	'2026-10-16T12:00:08.800Z advertencia: no se pudo actualizar Caddy; el motivo está en el estado del proxy (GET /api/v1/proxy)\n'

// El kind y los servicios de la configuración salen del catálogo (contrato §2.12): se mockea
// su container, no la red. Por defecto no carga, y todo sale de Swarm como antes.
const catalog = vi.hoisted(() => ({ getApp: vi.fn() }))
vi.mock('@/modules/catalog/container', () => ({ catalogUseCases: catalog }))

const PROPS = { project: 'auditfy', stage: 'staging', app: 'api' }

const HISTORY = [
	makeDeploy(144, { status: 'running', finishedAt: null }),
	makeDeploy(143),
	makeDeploy(142, { status: 'failed', error: 'la versión nueva no pasó el healthcheck' }),
]

/** El historial con #143 marcado: su paso 7 no pudo actualizar Caddy (`routes_warning`). */
const WARNED = HISTORY.map((deploy) =>
	deploy.id === 143 ? { ...deploy, routesWarning: true } : deploy,
)

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
		catalog.getApp.mockRejectedValue(new UnavailableError('sin catálogo'))
		ingress.getProxyStatus.mockResolvedValue(proxyStatus())
	})

	it('la advertencia del paso 7 sale del deploy (routes_warning); su motivo, de GET /proxy', async () => {
		ingress.getProxyStatus.mockResolvedValue(
			proxyStatus({
				status: 'out_of_sync',
				error: 'Caddy rechazó la configuración: la ruta de x.tudominio.cl no es válida',
			}),
		)
		mocks.listDeploys.mockResolvedValue(WARNED)
		mocks.followDeployLog.mockImplementation(
			scripted([chunk(CADDY_WARNING, 120), { kind: 'status', status: 'succeeded' }]),
		)
		const { vm } = await mountViewModel(
			() => new DeploysViewModel(() => ({ ...PROPS, deployParam: '143' })),
		)

		await vi.waitFor(() =>
			expect(vm.proxyWarning).toEqual({
				kind: 'failing',
				error: 'Caddy rechazó la configuración: la ruta de x.tudominio.cl no es válida',
			}),
		)
		// La línea se pinta en ámbar como cualquier advertencia.
		await vi.waitFor(() => expect(vm.log.lines[0]?.tone).toBe('warn'))
	})

	it('si el proxy ya se sincronizó, lo dice; si GET /proxy no contesta, avisa sin el motivo', async () => {
		mocks.listDeploys.mockResolvedValue(WARNED)
		const { vm } = await mountViewModel(
			() => new DeploysViewModel(() => ({ ...PROPS, deployParam: '143' })),
		)
		await vi.waitFor(() => expect(vm.proxyWarning).toEqual({ kind: 'synced' }))

		ingress.getProxyStatus.mockRejectedValue(new UnavailableError('caído'))
		const other = await mountViewModel(
			() => new DeploysViewModel(() => ({ ...PROPS, deployParam: '143' })),
		)
		await vi.waitFor(() => expect(other.vm.selected?.id).toBe(143))
		await vi.waitFor(() => expect(ingress.getProxyStatus).toHaveBeenCalledTimes(2))
		expect(other.vm.proxyWarning).toEqual({ kind: 'failing', error: null })
	})

	it('sin la marca no avisa ni pide el estado del proxy, aunque el log diga lo mismo', async () => {
		// El texto del log no es un contrato (§2.11): una tarea de la app puede escribirlo.
		mocks.followDeployLog.mockImplementation(
			scripted([chunk(CADDY_WARNING, 120), { kind: 'status', status: 'succeeded' }]),
		)
		const { vm } = await mountViewModel(
			() => new DeploysViewModel(() => ({ ...PROPS, deployParam: '143' })),
		)

		await vi.waitFor(() => expect(vm.log.lines).toHaveLength(1))
		expect(vm.proxyWarning).toBeNull()
		expect(ingress.getProxyStatus).not.toHaveBeenCalled()
	})

	it('uno que se sigue en vivo y termina con la marca: avisa al refrescar el historial', async () => {
		mocks.listDeploys
			.mockResolvedValueOnce(HISTORY)
			.mockResolvedValue([makeDeploy(144, { routesWarning: true }), ...HISTORY.slice(1)])
		mocks.followDeployLog.mockImplementation(
			// Sin la línea en el log: lo que cuenta es la marca del deploy.
			scripted([
				{ kind: 'status', status: 'running' },
				chunk('2026-10-16T12:00:09Z deploy exitoso en 9 s\n', 50),
				{ kind: 'status', status: 'succeeded' },
			]),
		)
		const { vm } = await mountViewModel(() => new DeploysViewModel(() => PROPS))

		await vi.waitFor(() => expect(vm.proxyWarning).toEqual({ kind: 'synced' }))
		expect(vm.selected?.id).toBe(144)
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

	it('sabe qué sigue fijado por un rollback, aunque el deploy actual ya no sea el rollback', async () => {
		const { vm } = await mountViewModel(() => new DeploysViewModel(() => PROPS))
		await vi.waitFor(() => expect(vm.selected?.id).toBe(144))
		expect(vm.pinned).toBeNull()

		// #146 es un deploy sin cuerpo después del rollback #144: repite sus refs pedidas, con
		// digest (`requested_images`).
		const pinned = makeDeploy(144).images
		mocks.listDeploys.mockResolvedValue([
			makeDeploy(147, { status: 'failed' }),
			makeDeploy(146, { requestedImages: pinned }),
			makeDeploy(145, { trigger: 'rollback', status: 'failed', requestedImages: pinned }),
			makeDeploy(144, { trigger: 'rollback', requestedImages: pinned }),
			makeDeploy(143),
		])
		const rolledBack = await mountViewModel(() => new DeploysViewModel(() => PROPS))
		await vi.waitFor(() => expect(rolledBack.vm.selected?.id).toBe(147))
		expect(rolledBack.vm.pinned).toEqual({
			rollback: { id: 144, services: ['api'] },
			others: [],
		})
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
		expect(vm.deployServices).toEqual(['api'])
	})

	it('el diálogo usa el kind y los servicios del catálogo: una compose con un solo servicio que se llama como la app sigue siendo compose', async () => {
		// En Swarm solo corre `api`; el compose guardado ya suma `worker`.
		catalog.getApp.mockResolvedValue({
			kind: 'compose',
			services: [
				{ name: 'api', ports: [3000] },
				{ name: 'worker', ports: [] },
			],
		})
		const { vm } = await mountViewModel(() => new DeploysViewModel(() => PROPS))

		await vi.waitFor(() => expect(vm.kind).toBe('compose'))
		expect(vm.deployServices).toEqual(['api', 'worker'])

		// Lo que llega por props gana.
		const forced = await mountViewModel(
			() => new DeploysViewModel(() => ({ ...PROPS, kind: 'service' })),
		)
		expect(forced.vm.kind).toBe('service')
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

	// Un deploy exitoso adelanta `last_deployed_at` (diseño §7 paso 8): lo que el catálogo
	// tiene en el cache (la marca "Cambios pendientes" de Proyectos, la del Resumen) queda
	// viejo y se vuelve a pedir al mostrarse.
	it('un deploy seguido en vivo que termina bien deja viejo lo del catálogo de esa app', async () => {
		mocks.followDeployLog.mockImplementation(
			scripted([
				{ kind: 'status', status: 'running' },
				{ kind: 'status', status: 'succeeded' },
			]),
		)
		const client = testClient()
		const other = catalogKeys.app(appRefFrom('auditfy', 'staging', 'nats'))
		client.setQueryData(catalogKeys.tree(), [])
		client.setQueryData(other, {})
		const { vm } = await mountViewModel(() => new DeploysViewModel(() => PROPS), client)

		await vi.waitFor(() => expect(vm.liveStatus).toBe('succeeded'))
		await vi.waitFor(() =>
			expect(client.getQueryState(catalogKeys.tree())?.isInvalidated).toBe(true),
		)
		expect(
			client.getQueryState(catalogKeys.app(appRefFrom('auditfy', 'staging', 'api')))
				?.isInvalidated,
		).toBe(true)
		expect(client.getQueryState(other)?.isInvalidated).toBe(false)
	})

	it('uno que falla no toca el catálogo', async () => {
		mocks.followDeployLog.mockImplementation(
			scripted([
				{ kind: 'status', status: 'running' },
				{ kind: 'status', status: 'failed' },
			]),
		)
		const client = testClient()
		client.setQueryData(catalogKeys.tree(), [])
		const { vm } = await mountViewModel(() => new DeploysViewModel(() => PROPS), client)

		await vi.waitFor(() => expect(vm.liveStatus).toBe('failed'))
		expect(client.getQueryState(catalogKeys.tree())?.isInvalidated).toBe(false)
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

	it('otro ?deploy= en la misma ruta (141 → 142) abre ese deploy', async () => {
		mocks.listDeploys.mockResolvedValue([...HISTORY, makeDeploy(141)])
		const props = $state({ ...PROPS, deployParam: '141' })
		const { vm } = await mountViewModel(() => new DeploysViewModel(() => props))
		await vi.waitFor(() => expect(vm.selected?.id).toBe(141))

		props.deployParam = '142'
		await vi.waitFor(() => expect(vm.selected?.id).toBe(142))
	})
})
