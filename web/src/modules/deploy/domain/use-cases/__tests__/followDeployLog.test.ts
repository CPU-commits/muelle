import { beforeEach, describe, expect, it, type Mock, vi } from 'vitest'
import { UnavailableError } from '@/shared/domain'
import { asLogOffset, type DeployLogEvent } from '../../entities/deployLog'
import { APP } from '../../repositories/__fakes__/deployFixtures'
import { InMemoryDeployRepository } from '../../repositories/__fakes__/inMemoryDeployRepository'
import type { Wait } from '../../repositories/deployRepository'
import { asDeployId } from '../../value-objects/deployId'
import { LOG_POLL_INTERVAL_MS, makeFollowDeployLog } from '../followDeployLog'

const ID = asDeployId(144)

async function collect(iterable: AsyncIterable<DeployLogEvent>): Promise<DeployLogEvent[]> {
	const events: DeployLogEvent[] = []
	for await (const event of iterable) events.push(event)
	return events
}

const chunk = (text: string, next: number): DeployLogEvent => ({
	kind: 'chunk',
	text,
	nextOffset: asLogOffset(next),
})

describe('followDeployLog', () => {
	let repo: InMemoryDeployRepository
	let wait: Mock<Wait>
	let follow: ReturnType<typeof makeFollowDeployLog>

	beforeEach(() => {
		repo = new InMemoryDeployRepository()
		wait = vi.fn<Wait>(async () => {})
		follow = makeFollowDeployLog({ deploys: repo, wait })
	})

	it('por SSE: termina con el estado final, sin tocar el polling', async () => {
		repo.logStream = {
			events: [
				{ kind: 'status', status: 'running' },
				chunk('uno\n', 4),
				chunk('dos\n', 8),
				{ kind: 'status', status: 'succeeded' },
				// Nada después del estado final se lee.
				chunk('tres\n', 13),
			],
		}

		const events = await collect(
			follow({ app: APP, id: ID, live: true }, new AbortController().signal),
		)

		expect(events).toEqual([
			{ kind: 'status', status: 'running' },
			chunk('uno\n', 4),
			chunk('dos\n', 8),
			{ kind: 'status', status: 'succeeded' },
		])
		expect(repo.streamOffsets).toEqual([0])
		expect(repo.readLogOffsets).toEqual([])
	})

	it('si el SSE no abre, sigue por polling desde el principio', async () => {
		repo.logStream = { events: [], failAfter: 0 }
		repo.logs.set(ID, [
			{ text: 'uno\n', status: 'running' },
			{ text: 'uno\ndos\n', status: 'running' },
			{ text: 'uno\ndos\ntres\n', status: 'succeeded' },
		])

		const events = await collect(
			follow({ app: APP, id: ID, live: true }, new AbortController().signal),
		)

		expect(events).toEqual([
			{ kind: 'polling' },
			chunk('uno\n', 4),
			{ kind: 'status', status: 'running' },
			chunk('dos\n', 8),
			chunk('tres\n', 13),
			{ kind: 'status', status: 'succeeded' },
		])
		// La última lectura confirma que no queda nada.
		expect(repo.readLogOffsets).toEqual([0, 4, 8, 13])
	})

	it('si el SSE se corta a la mitad, el polling sigue desde el último offset sin repetir', async () => {
		repo.logStream = {
			events: [{ kind: 'status', status: 'running' }, chunk('uno\n', 4), chunk('dos\n', 8)],
			failAfter: 3,
		}
		repo.logs.set(ID, [{ text: 'uno\ndos\ntres\n', status: 'failed' }])

		const events = await collect(
			follow({ app: APP, id: ID, live: true }, new AbortController().signal),
		)

		expect(events).toEqual([
			{ kind: 'status', status: 'running' },
			chunk('uno\n', 4),
			chunk('dos\n', 8),
			{ kind: 'polling' },
			chunk('tres\n', 13),
			{ kind: 'status', status: 'failed' },
		])
		expect(repo.readLogOffsets[0]).toBe(8)
	})

	it('si el SSE termina sin estado final, también sigue por polling', async () => {
		repo.logStream = { events: [chunk('uno\n', 4)] }
		repo.logs.set(ID, [{ text: 'uno\n', status: 'succeeded' }])

		const events = await collect(
			follow({ app: APP, id: ID, live: true }, new AbortController().signal),
		)

		expect(events).toEqual([
			chunk('uno\n', 4),
			{ kind: 'polling' },
			{ kind: 'status', status: 'succeeded' },
		])
	})

	it('si el polling no reconoce el offset (422), vuelve a leer desde el principio una vez', async () => {
		// El SSE dio un `id` más allá de lo que tiene el log guardado.
		repo.logStream = { events: [chunk('uno\n', 4), chunk('dos\n', 99)], failAfter: 2 }
		repo.logs.set(ID, [{ text: 'uno\ndos\n', status: 'succeeded' }])

		const events = await collect(
			follow({ app: APP, id: ID, live: true }, new AbortController().signal),
		)

		expect(events).toEqual([
			chunk('uno\n', 4),
			chunk('dos\n', 99),
			{ kind: 'polling' },
			{ kind: 'reset' },
			chunk('uno\ndos\n', 8),
			{ kind: 'status', status: 'succeeded' },
		])
		expect(repo.readLogOffsets).toEqual([99, 0, 8])
	})

	it('un deploy terminado se lee por ?offset, sin abrir un stream', async () => {
		repo.logs.set(ID, [{ text: 'todo el log\n', status: 'succeeded' }])

		const events = await collect(
			follow({ app: APP, id: ID, live: false }, new AbortController().signal),
		)

		expect(events).toEqual([
			chunk('todo el log\n', 12),
			{ kind: 'status', status: 'succeeded' },
		])
		expect(repo.streamOffsets).toEqual([])
		expect(wait).not.toHaveBeenCalled()
	})

	it('con páginas cortas pide la siguiente enseguida, sin esperar', async () => {
		repo.pageSize = 4
		repo.logs.set(ID, [{ text: 'uno\ndos\n', status: 'succeeded' }])

		const events = await collect(
			follow({ app: APP, id: ID, live: false }, new AbortController().signal),
		)

		expect(events.filter((event) => event.kind === 'chunk')).toEqual([
			chunk('uno\n', 4),
			chunk('dos\n', 8),
		])
		expect(wait).not.toHaveBeenCalled()
	})

	it('mientras corre sin novedades, espera el intervalo entre lecturas', async () => {
		repo.logs.set(ID, [
			{ text: 'uno\n', status: 'running' },
			{ text: 'uno\n', status: 'running' },
			{ text: 'uno\n', status: 'succeeded' },
		])

		await collect(follow({ app: APP, id: ID, live: false }, new AbortController().signal))

		expect(wait).toHaveBeenCalledWith(LOG_POLL_INTERVAL_MS, expect.any(AbortSignal))
	})

	it('abortar corta el seguimiento sin error', async () => {
		const controller = new AbortController()
		repo.logs.set(ID, [{ text: 'uno\n', status: 'running' }])
		wait.mockImplementation(async () => controller.abort())

		const events = await collect(follow({ app: APP, id: ID, live: false }, controller.signal))

		expect(events).toEqual([chunk('uno\n', 4), { kind: 'status', status: 'running' }])
	})

	it('un error del polling se propaga', async () => {
		repo.logStream = { events: [], failAfter: 0 }
		repo.readLog = async () => {
			throw new UnavailableError('caído')
		}

		await expect(
			collect(follow({ app: APP, id: ID, live: true }, new AbortController().signal)),
		).rejects.toBeInstanceOf(UnavailableError)
	})
})
