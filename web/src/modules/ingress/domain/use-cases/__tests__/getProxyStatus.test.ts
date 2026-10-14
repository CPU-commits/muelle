import { describe, expect, it } from 'vitest'
import { UnavailableError } from '@/shared/domain'
import {
	InMemoryProxyRepository,
	makeProxyStatus,
} from '../../repositories/__fakes__/inMemoryProxyRepository'
import { makeGetProxyStatus } from '../getProxyStatus'

describe('getProxyStatus', () => {
	it('devuelve el estado del proxy', async () => {
		const proxy = new InMemoryProxyRepository()
		proxy.current = makeProxyStatus({ status: 'out_of_sync', error: 'Caddy rechazó la ruta' })
		await expect(makeGetProxyStatus({ proxy })()).resolves.toMatchObject({
			status: 'out_of_sync',
			error: 'Caddy rechazó la ruta',
		})
	})

	it('sin la Fase 3 rechaza con la fase', async () => {
		const proxy = new InMemoryProxyRepository()
		proxy.failure = new UnavailableError('Llega con la Fase 3', { phase: 3 })
		await expect(makeGetProxyStatus({ proxy })()).rejects.toMatchObject({ phase: 3 })
	})
})
