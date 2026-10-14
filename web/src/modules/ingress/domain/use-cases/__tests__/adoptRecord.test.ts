import { describe, expect, it } from 'vitest'
import { domainState } from '../../entities/appDomain'
import { NothingToAdopt } from '../../errors/ingressErrors'
import { APP, makeDomain, recordExists } from '../../repositories/__fakes__/domainFixtures'
import { InMemoryDomainRepository } from '../../repositories/__fakes__/inMemoryDomainRepository'
import { asHostname } from '../../value-objects/hostname'
import { makeAdoptRecord } from '../adoptRecord'

describe('adoptRecord', () => {
	it('toma el registro existente: muelle pasa a manejarlo', async () => {
		const repo = new InMemoryDomainRepository()
		repo.domains = [recordExists('webhooks.tudominio.cl')]

		const domain = await makeAdoptRecord({ domains: repo })({
			app: APP,
			hostname: asHostname('webhooks.tudominio.cl'),
		})

		expect(domain.recordId).not.toBeNull()
		expect(domainState(domain)).toBe('provisioning')
	})

	it('sin registro que adoptar es NothingToAdopt', async () => {
		const repo = new InMemoryDomainRepository()
		repo.domains = [makeDomain('api.tudominio.cl')]

		await expect(
			makeAdoptRecord({ domains: repo })({
				app: APP,
				hostname: asHostname('api.tudominio.cl'),
			}),
		).rejects.toBeInstanceOf(NothingToAdopt)
	})
})
