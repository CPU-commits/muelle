import { describe, expect, it } from 'vitest'
import { APP, makeDomain } from '../../repositories/__fakes__/domainFixtures'
import { InMemoryDomainRepository } from '../../repositories/__fakes__/inMemoryDomainRepository'
import { asHostname } from '../../value-objects/hostname'
import { makeSetProxied } from '../setProxied'

describe('setProxied', () => {
	it('cambia solo proxied', async () => {
		const repo = new InMemoryDomainRepository()
		repo.domains = [makeDomain('api.tudominio.cl', { proxied: true })]

		const domain = await makeSetProxied({ domains: repo })({
			app: APP,
			hostname: asHostname('api.tudominio.cl'),
			proxied: false,
		})

		expect(domain.proxied).toBe(false)
		expect(domain.target).toEqual({ service: 'api', port: 3000 })
	})
})
