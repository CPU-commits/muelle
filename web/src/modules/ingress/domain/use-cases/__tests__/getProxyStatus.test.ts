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

	it('si muelle no responde, rechaza con el error del repositorio', async () => {
		const proxy = new InMemoryProxyRepository()
		proxy.failure = new UnavailableError('caído', { reason: 'server' })
		await expect(makeGetProxyStatus({ proxy })()).rejects.toBe(proxy.failure)
	})
})
