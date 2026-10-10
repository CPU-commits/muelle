import { describe, expect, it } from 'vitest'
import { ValidationError } from '@/shared/domain'
import type { AppLogEvent, LogTail } from '../../entities/appLogs'
import { LogStreamFailed } from '../../errors/deployErrors'
import { APP } from '../../repositories/__fakes__/deployFixtures'
import { InMemoryDeployRepository } from '../../repositories/__fakes__/inMemoryDeployRepository'
import { makeStreamAppLogs } from '../streamAppLogs'

const line: AppLogEvent = {
	kind: 'line',
	line: { time: null, task: 'api.1', stream: 'stdout', text: 'GET /health', level: 'info' },
}

async function collect(iterable: AsyncIterable<AppLogEvent>): Promise<AppLogEvent[]> {
	const events: AppLogEvent[] = []
	for await (const event of iterable) events.push(event)
	return events
}

describe('streamAppLogs', () => {
	it('pasa el servicio y el tail al repositorio y emite lo que llega', async () => {
		const repo = new InMemoryDeployRepository()
		repo.appLogStream = { events: [{ kind: 'open' }, line] }
		const stream = makeStreamAppLogs({ deploys: repo })

		const events = await collect(
			stream({ app: APP, service: ' api ', tail: 200 }, new AbortController().signal),
		)

		expect(events).toEqual([{ kind: 'open' }, line])
		expect(repo.appLogQueries).toEqual([{ service: 'api', tail: 200 }])
	})

	it('acepta tail 0 para reconectar sin repetir', async () => {
		const repo = new InMemoryDeployRepository()
		await collect(
			makeStreamAppLogs({ deploys: repo })(
				{ app: APP, service: 'api', tail: 0 },
				new AbortController().signal,
			),
		)
		expect(repo.appLogQueries[0]?.tail).toBe(0)
	})

	it('sin servicio o con un tail raro es un error de validación', async () => {
		const stream = makeStreamAppLogs({ deploys: new InMemoryDeployRepository() })
		const signal = new AbortController().signal

		await expect(
			collect(stream({ app: APP, service: ' ', tail: 200 }, signal)),
		).rejects.toThrow(ValidationError)
		await expect(
			collect(stream({ app: APP, service: 'api', tail: 7 as LogTail }, signal)),
		).rejects.toThrow(ValidationError)
	})

	it('un corte del stream llega como LogStreamFailed', async () => {
		const repo = new InMemoryDeployRepository()
		repo.appLogStream = { events: [{ kind: 'open' }, line], failAfter: 2 }

		const result = collect(
			makeStreamAppLogs({ deploys: repo })(
				{ app: APP, service: 'api', tail: 100 },
				new AbortController().signal,
			),
		)
		await expect(result).rejects.toBeInstanceOf(LogStreamFailed)
		await expect(result).rejects.toMatchObject({ opened: true })
	})
})
