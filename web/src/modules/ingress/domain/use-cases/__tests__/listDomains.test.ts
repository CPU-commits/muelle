import { describe, expect, it } from 'vitest'
import { APP, makeDomain } from '../../repositories/__fakes__/domainFixtures'
import { InMemoryDomainRepository } from '../../repositories/__fakes__/inMemoryDomainRepository'
import { makeListDomains } from '../listDomains'

describe('listDomains', () => {
	it('devuelve los dominios ordenados por hostname', async () => {
		const repo = new InMemoryDomainRepository()
		repo.domains = [makeDomain('webhooks.tudominio.cl'), makeDomain('api.tudominio.cl')]

		const domains = await makeListDomains({ domains: repo })(APP)

		expect(domains.map((domain) => domain.hostname)).toEqual([
			'api.tudominio.cl',
			'webhooks.tudominio.cl',
		])
	})
})
