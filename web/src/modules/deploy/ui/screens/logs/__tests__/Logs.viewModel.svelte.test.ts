import { beforeEach, describe, expect, it, vi } from 'vitest'
import { UnavailableError } from '@/shared/domain'
import {
	type AppLogEvent,
	type AppLogLine,
	LOG_LINE_CAP,
	LogStreamFailed,
	type StreamAppLogsInput,
} from '../../../../domain'
import { makeService, makeStatus } from '../../../../domain/repositories/__fakes__/deployFixtures'
import { mountViewModel } from '../../../__tests__/mountViewModel'
import { LOG_FLUSH_MS, LogsViewModel } from '../Logs.viewModel.svelte.ts'

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

/** Un stream que el test maneja: `push` manda una línea, `fail` lo corta. */
class ControlledStream {
	readonly inputs: StreamAppLogsInput[] = []
	#queue: (AppLogEvent | Error)[] = []
	#wake: (() => void) | null = null

	readonly impl = async function* (
		this: ControlledStream,
		input: StreamAppLogsInput,
		signal: AbortSignal,
	): AsyncGenerator<AppLogEvent> {
		this.inputs.push(input)
		signal.addEventListener('abort', () => this.#wake?.())
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

function logLine(text: string, task = 'api.1', level: AppLogLine['level'] = 'info'): AppLogLine {
	return { time: new Date('2026-10-10T14:31:40.118Z'), task, stream: 'stdout', text, level }
}

const flushed = () => new Promise((resolve) => setTimeout(resolve, LOG_FLUSH_MS + 30))

describe('LogsViewModel', () => {
	let stream: ControlledStream

	beforeEach(() => {
		vi.clearAllMocks()
		stream = new ControlledStream()
		mocks.getAppStatus.mockResolvedValue(
			makeStatus([makeService({ name: 'api' }), makeService({ name: 'worker' })]),
		)
		mocks.streamAppLogs.mockImplementation((input: StreamAppLogsInput, signal: AbortSignal) =>
			stream.impl(input, signal),
		)
	})

	async function mounted(): Promise<LogsViewModel> {
		const { vm } = await mountViewModel(() => new LogsViewModel(() => PROPS))
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

	it('si se corta, ofrece reconectar: pide tail=0 y no pierde lo que había', async () => {
		const vm = await mounted()
		stream.push(logLine('antes'))
		await vi.waitFor(() => expect(vm.lines).toHaveLength(1))

		stream.fail()
		await vi.waitFor(() => expect(vm.connection).toBe('lost'))

		vm.reconnect()
		await vi.waitFor(() => expect(vm.connection).toBe('live'))
		expect(stream.inputs[1]).toMatchObject({ service: 'api', tail: 0 })
		expect(vm.lines.map((keyed) => keyed.line.text)).toEqual(['antes'])
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
