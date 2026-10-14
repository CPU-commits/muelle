import { describe, expect, it } from 'vitest'
import { ValidationError } from '@/shared/domain'
import { HostnameTaken } from '../../errors/ingressErrors'
import { APP, makeDomain } from '../../repositories/__fakes__/domainFixtures'
import { InMemoryDomainRepository } from '../../repositories/__fakes__/inMemoryDomainRepository'
import { makeAddDomain } from '../addDomain'

describe('addDomain', () => {
	it('normaliza el hostname, lee el destino y crea', async () => {
		const repo = new InMemoryDomainRepository()
		const domain = await makeAddDomain({ domains: repo })({
			app: APP,
			hostname: ' API.TuDominio.cl ',
			target: 'api:3000',
			proxied: true,
		})

		expect(domain.hostname).toBe('api.tudominio.cl')
		expect(domain.target).toEqual({ service: 'api', port: 3000 })
		expect(repo.calls).toEqual(['create api.tudominio.cl'])
	})

	it('un hostname inválido no llega al repositorio', async () => {
		const repo = new InMemoryDomainRepository()
		await expect(
			makeAddDomain({ domains: repo })({
				app: APP,
				hostname: 'localhost',
				target: 'api:3000',
				proxied: false,
			}),
		).rejects.toMatchObject({ name: 'ValidationError', field: 'hostname' })
		expect(repo.calls).toEqual([])
	})

	it('un destino inválido es un error del campo target', async () => {
		const result = makeAddDomain({ domains: new InMemoryDomainRepository() })({
			app: APP,
			hostname: 'api.tudominio.cl',
			target: 'api',
			proxied: false,
		})
		await expect(result).rejects.toBeInstanceOf(ValidationError)
		await expect(result).rejects.toMatchObject({ field: 'target' })
	})

	it('un hostname que ya existe es HostnameTaken', async () => {
		const repo = new InMemoryDomainRepository()
		repo.domains = [makeDomain('api.tudominio.cl')]
		await expect(
			makeAddDomain({ domains: repo })({
				app: APP,
				hostname: 'api.tudominio.cl',
				target: 'api:3000',
				proxied: true,
			}),
		).rejects.toBeInstanceOf(HostnameTaken)
	})
})
