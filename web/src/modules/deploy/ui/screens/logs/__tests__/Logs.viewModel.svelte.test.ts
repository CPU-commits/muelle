import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { UnauthorizedError, UnavailableError } from '@/shared/domain'
import {
	type AppLogEvent,
	type AppLogLine,
	LOG_LINE_CAP,
	LogStreamFailed,
	type StreamAppLogsInput,
} from '../../../../domain'
import { makeService, makeStatus } from '../../../../domain/repositories/__fakes__/deployFixtures'
import { mountViewModel } from '../../../__tests__/mountViewModel'
import { LOG_FLUSH_MS, type LogTiming, LogsViewModel } from '../Logs.viewModel.svelte.ts'

const mocks = vi.hoisted(() => ({
	getAppStatus: vi.fn(),
	streamAppLogs: vi.fn(),
}))

vi.mock('@/modules/deploy/container', () => ({
	deployUseCases: {
		getAppStatus: mocks.getAppStatus,
		listDeploys: vi.fn(),
		getDeploy: vi.fn(),
		createDeploy: vi.fn(),
		rollback: vi.fn(),
		followDeployLog: vi.fn(),
		streamAppLogs: mocks.streamAppLogs,
	},
}))

const PROPS = { project: 'auditfy', stage: 'staging', app: 'api' }

/**
 * Un stream que el test maneja: `push` manda una línea, `fail` lo corta. Cada conexión
 * abre (salvo que `refuse` diga lo contrario) y lee de la misma cola.
 */
class ControlledStream {
	readonly inputs: StreamAppLogsInput[] = []
	readonly signals: AbortSignal[] = []
	/** Las próximas conexiones fallan con esto sin abrir. */
	refuse: Error | null = null
	#queue: (AppLogEvent | Error)[] = []
	#wake: (() => void) | null = null

	readonly impl = async function* (
		this: ControlledStream,
		input: StreamAppLogsInput,
		signal: AbortSignal,
	): AsyncGenerator<AppLogEvent> {
		this.inputs.push(input)
		this.signals.push(signal)
		signal.addEventListener('abort', () => this.#wake?.())
		if (this.refuse) throw this.refuse
		yield { kind: 'open' }
		while (!signal.aborted) {
			const item = this.#queue.shift()
			if (!item) {
				await new Promise<void>((resolve) => (this.#wake = resolve))
				continue
			}
			if (item instanceof Error) throw item
			yield item
		}
	}

	push(...lines: AppLogLine[]): void {
		for (const line of lines) this.#queue.push({ kind: 'line', line })
		this.#wake?.()
	}

	fail(): void {
		this.#queue.push(new LogStreamFailed(true))
		this.#wake?.()
	}
}

function logLine(
	text: string,
	task = 'api.1',
	level: AppLogLine['level'] = 'info',
	time = '2026-10-10T14:31:40.118Z',
): AppLogLine {
	return { time: new Date(time), task, stream: 'stdout', text, level }
}

/** Esperas cortas, para que el test no tarde lo que tarda la app. */
const FAST: Partial<LogTiming> = { reconnectBaseMs: 10, reconnectMaxMs: 40, hiddenCloseMs: 30 }

let visibility: DocumentVisibilityState = 'visible'

function setVisibility(state: DocumentVisibilityState): void {
	visibility = state
	document.dispatchEvent(new Event('visibilitychange'))
}

const flushed = () => new Promise((resolve) => setTimeout(resolve, LOG_FLUSH_MS + 30))

describe('LogsViewModel', () => {
	let stream: ControlledStream

	beforeEach(() => {
		vi.clearAllMocks()
		visibility = 'visible'
		Object.defineProperty(document, 'visibilityState', {
			configurable: true,
			get: () => visibility,
		})
		stream = new ControlledStream()
		mocks.getAppStatus.mockResolvedValue(
			makeStatus([makeService({ name: 'api' }), makeService({ name: 'worker' })]),
		)
		mocks.streamAppLogs.mockImplementation((input: StreamAppLogsInput, signal: AbortSignal) =>
			stream.impl(input, signal),
		)
	})

	afterEach(() => {
		Reflect.deleteProperty(document, 'visibilityState')
	})

	async function mounted(timing: Partial<LogTiming> = FAST): Promise<LogsViewModel> {
		const { vm } = await mountViewModel(() => new LogsViewModel(() => PROPS, timing))
		await vi.waitFor(() => expect(vm.connection).toBe('live'))
		return vm
	}

	it('sigue el primer servicio con las últimas 200 y pinta las líneas por tandas', async () => {
		const vm = await mounted()

		expect(stream.inputs[0]).toMatchObject({ service: 'api', tail: 200 })
		expect(vm.services).toEqual(['api', 'worker'])

		stream.push(logLine('GET /health'), logLine('consulta lenta', 'api.2', 'warn'))
		await vi.waitFor(() => expect(vm.lines).toHaveLength(2))
		expect(vm.tasks).toEqual(['api.1', 'api.2'])
	})

	it('en pausa guarda lo que llega y al reanudar lo suma', async () => {
		const vm = await mounted()
		stream.push(logLine('uno'))
		await vi.waitFor(() => expect(vm.lines).toHaveLength(1))

		vm.togglePause()
		stream.push(logLine('dos'), logLine('tres'))
		await flushed()
		expect(vm.lines).toHaveLength(1)
		expect(vm.pending).toHaveLength(2)

		vm.setFollow(false)
		vm.togglePause()
		expect(vm.paused).toBe(false)
		expect(vm.lines.map((keyed) => keyed.line.text)).toEqual(['uno', 'dos', 'tres'])
		expect(vm.pending).toHaveLength(0)
		expect(vm.follow).toBe(true)
	})

	it('limpiar vacía las líneas sin cortar el stream', async () => {
		const vm = await mounted()
		stream.push(logLine('uno'))
		await vi.waitFor(() => expect(vm.lines).toHaveLength(1))

		vm.clear()
		expect(vm.lines).toHaveLength(0)

		stream.push(logLine('dos'))
		await vi.waitFor(() => expect(vm.lines).toHaveLength(1))
		expect(vm.connection).toBe('live')
	})

	it(`guarda como mucho ${LOG_LINE_CAP} líneas`, async () => {
		const vm = await mounted()
		const many = Array.from({ length: LOG_LINE_CAP + 25 }, (_, index) => logLine(`l${index}`))
		stream.push(...many)

		await vi.waitFor(() => expect(vm.lines.at(-1)?.line.text).toBe(`l${LOG_LINE_CAP + 24}`))
		expect(vm.lines).toHaveLength(LOG_LINE_CAP)
		expect(vm.lines[0]?.line.text).toBe('l25')
		expect(vm.isCapped).toBe(true)
	})

	it('si se corta, avisa y reconecta sola desde la hora de la última línea, sin repetir', async () => {
		// Una espera que deja ver el aviso antes de reconectar.
		const vm = await mounted({ ...FAST, reconnectBaseMs: 300, reconnectMaxMs: 300 })
		stream.push(
			logLine('uno', 'api.1', 'info', '2026-10-10T14:31:51.000Z'),
			logLine('dos', 'api.1', 'info', '2026-10-10T14:31:51.517Z'),
		)
		await vi.waitFor(() => expect(vm.lines).toHaveLength(2))

		stream.fail()
		await vi.waitFor(() => expect(vm.problem).toBe('dropped'))
		await vi.waitFor(() => expect(vm.connection).toBe('live'))

		expect(stream.inputs[1]).toMatchObject({ service: 'api', tail: 200 })
		expect(stream.inputs[1]?.since?.toISOString()).toBe('2026-10-10T14:31:51.517Z')
		expect(vm.problem).toBeNull()

		// `since` es inclusivo: lo del último instante vuelve a llegar y no se repite.
		stream.push(
			logLine('dos', 'api.1', 'info', '2026-10-10T14:31:51.517Z'),
			logLine('tres', 'api.2', 'info', '2026-10-10T14:31:51.517Z'),
			logLine('cuatro', 'api.1', 'info', '2026-10-10T14:31:53.000Z'),
		)
		await vi.waitFor(() => expect(vm.lines).toHaveLength(4))
		expect(vm.lines.map((keyed) => keyed.line.text)).toEqual(['uno', 'dos', 'tres', 'cuatro'])
	})

	it('si no llega a abrir no dice "se cortó", y sigue intentando con espera creciente', async () => {
		stream.refuse = new LogStreamFailed(false)
		const { vm } = await mountViewModel(() => new LogsViewModel(() => PROPS, FAST))

		await vi.waitFor(() => expect(vm.problem).toBe('unreachable'))
		await vi.waitFor(() => expect(stream.inputs.length).toBeGreaterThanOrEqual(3))
		expect(vm.connection).not.toBe('live')

		stream.refuse = null
		await vi.waitFor(() => expect(vm.connection).toBe('live'))
		expect(vm.problem).toBeNull()
	})

	it('"Reconectar ahora" no espera el backoff', async () => {
		const vm = await mounted({ reconnectBaseMs: 60_000 })
		stream.fail()
		await vi.waitFor(() => expect(vm.connection).toBe('retrying'))

		vm.reconnect()
		await vi.waitFor(() => expect(vm.connection).toBe('live'))
		expect(stream.inputs).toHaveLength(2)
	})

	it('con la sesión vencida (401) no reintenta: el cliente HTTP ya mandó al login', async () => {
		stream.refuse = new UnauthorizedError('sesión vencida')
		const { vm } = await mountViewModel(() => new LogsViewModel(() => PROPS, FAST))

		await vi.waitFor(() => expect(vm.connection).toBe('stopped'))
		await new Promise((resolve) => setTimeout(resolve, 80))
		expect(stream.inputs).toHaveLength(1)
		expect(vm.problem).toBeNull()
	})

	it('con la pestaña oculta un rato cierra el stream, y al volver sigue desde la última línea', async () => {
		const vm = await mounted()
		stream.push(logLine('antes', 'api.1', 'info', '2026-10-10T14:31:51.517Z'))
		await vi.waitFor(() => expect(vm.lines).toHaveLength(1))

		setVisibility('hidden')
		await vi.waitFor(() => expect(vm.connection).toBe('asleep'))
		expect(stream.signals[0]?.aborted).toBe(true)
		expect(vm.problem).toBeNull()

		setVisibility('visible')
		await vi.waitFor(() => expect(vm.connection).toBe('live'))
		expect(stream.inputs[1]?.since?.toISOString()).toBe('2026-10-10T14:31:51.517Z')
		expect(vm.lines.map((keyed) => keyed.line.text)).toEqual(['antes'])
	})

	it('una vuelta corta a la pestaña no corta el stream', async () => {
		const vm = await mounted({ hiddenCloseMs: 200 })
		setVisibility('hidden')
		setVisibility('visible')
		await new Promise((resolve) => setTimeout(resolve, 260))

		expect(vm.connection).toBe('live')
		expect(stream.inputs).toHaveLength(1)
	})

	it('en pausa anuncia una sola vez que hay líneas nuevas (sin la cuenta)', async () => {
		const vm = await mounted()
		vm.togglePause()
		expect(vm.announcement).toBe('')

		stream.push(logLine('dos'))
		await vi.waitFor(() => expect(vm.pending).toHaveLength(1))
		const first = vm.announcement
		expect(first).toBe(m.deploy_logs_new_lines_announce())

		stream.push(logLine('tres'), logLine('cuatro'))
		await vi.waitFor(() => expect(vm.pending).toHaveLength(3))
		expect(vm.announcement).toBe(first)
	})

	it('la cuenta de líneas en pausa va en singular o plural', () => {
		expect(m.deploy_logs_new_lines({ count: 1 })).toBe('1 línea nueva en pausa')
		expect(m.deploy_logs_new_lines({ count: 3 })).toBe('3 líneas nuevas en pausa')
		expect(m.deploy_tasks_meta({ running: 2, history: 1 })).toBe('2 corriendo · 1 histórica')
		expect(m.deploy_usage_sampled({ sampled: 1, total: 2 })).toMatch(/^1 de 2 réplicas medidas/)
	})

	it('desmontar la pantalla corta el stream', async () => {
		const { vm, unmount } = await mountViewModel(() => new LogsViewModel(() => PROPS, FAST))
		await vi.waitFor(() => expect(vm.connection).toBe('live'))

		unmount()

		expect(stream.signals[0]?.aborted).toBe(true)
	})

	it('cambiar de app (los params de la ruta) abre el stream de la otra', async () => {
		const props = $state({ ...PROPS })
		const { vm } = await mountViewModel(() => new LogsViewModel(() => props, FAST))
		await vi.waitFor(() => expect(vm.connection).toBe('live'))

		props.app = 'worker-app'
		await vi.waitFor(() => expect(stream.inputs).toHaveLength(2))
		expect(stream.inputs[1]?.app).toEqual({ ...PROPS, app: 'worker-app' })
		expect(stream.signals[0]?.aborted).toBe(true)
	})

	it('cambiar de servicio o de líneas iniciales abre otro stream y empieza de cero', async () => {
		const vm = await mounted()
		stream.push(logLine('de api'))
		await vi.waitFor(() => expect(vm.lines).toHaveLength(1))

		vm.selectService('worker')
		await vi.waitFor(() => expect(stream.inputs).toHaveLength(2))
		expect(stream.inputs[1]).toMatchObject({ service: 'worker', tail: 200 })
		expect(vm.lines).toHaveLength(0)

		vm.setTail('500')
		await vi.waitFor(() => expect(stream.inputs).toHaveLength(3))
		expect(stream.inputs[2]).toMatchObject({ service: 'worker', tail: 500 })

		vm.setTail('7')
		expect(vm.tail).toBe(500)
	})

	it('filtra por texto y por tarea', async () => {
		const vm = await mounted()
		stream.push(
			logLine('GET /health', 'api.1'),
			logLine('POST /v1/audits', 'api.2'),
			logLine('GET /v1/audits', 'api.1'),
		)
		await vi.waitFor(() => expect(vm.lines).toHaveLength(3))

		vm.query = 'audits'
		expect(vm.visibleLines).toHaveLength(2)
		vm.setTask('api.1')
		expect(vm.visibleLines.map((keyed) => keyed.line.text)).toEqual(['GET /v1/audits'])
		expect(vm.isFiltering).toBe(true)
	})

	it('contra un backend sin la Fase 2 muestra la fase y no abre el stream', async () => {
		mocks.getAppStatus.mockRejectedValue(
			new UnavailableError('Llega con la Fase 2', { phase: 2 }),
		)
		const { vm } = await mountViewModel(() => new LogsViewModel(() => PROPS))

		await vi.waitFor(() => expect(vm.pendingPhase).toBe(2))
		expect(mocks.streamAppLogs).not.toHaveBeenCalled()
	})

	it('una app sin servicios no abre nada', async () => {
		mocks.getAppStatus.mockResolvedValue(makeStatus([]))
		const { vm } = await mountViewModel(() => new LogsViewModel(() => PROPS))

		await vi.waitFor(() => expect(vm.noServices).toBe(true))
		expect(vm.connection).toBe('idle')
	})
})
