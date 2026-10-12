import { describe, expect, it } from 'vitest'
import { APP, makeDomain } from '../../repositories/__fakes__/domainFixtures'
import { InMemoryDomainRepository } from '../../repositories/__fakes__/inMemoryDomainRepository'
import { asHostname } from '../../value-objects/hostname'
import { makeRetryDomain } from '../retryDomain'

describe('retryDomain', () => {
	it('vuelve a intentar: el error se limpia y queda pendiente', async () => {
		const repo = new InMemoryDomainRepository()
		repo.domains = [
			makeDomain('api.tudominio.cl', {
				status: 'error',
				errorKind: 'cloudflare',
				lastError: 'Cloudflare respondió 403',
			}),
		]

		const domain = await makeRetryDomain({ domains: repo })({
			app: APP,
			hostname: asHostname('api.tudominio.cl'),
		})

		expect(domain).toMatchObject({ status: 'pending', errorKind: null, lastError: null })
		expect(repo.calls).toEqual(['retry api.tudominio.cl'])
	})
})
