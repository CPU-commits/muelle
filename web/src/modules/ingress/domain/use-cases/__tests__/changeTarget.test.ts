import { describe, expect, it } from 'vitest'
import { ValidationError } from '@/shared/domain'
import { APP, makeDomain } from '../../repositories/__fakes__/domainFixtures'
import { InMemoryDomainRepository } from '../../repositories/__fakes__/inMemoryDomainRepository'
import { asHostname } from '../../value-objects/hostname'
import { makeChangeTarget } from '../changeTarget'

describe('changeTarget', () => {
	it('apunta el dominio a otro servicio:puerto', async () => {
		const repo = new InMemoryDomainRepository()
		repo.domains = [makeDomain('api.tudominio.cl')]

		const domain = await makeChangeTarget({ domains: repo })({
			app: APP,
			hostname: asHostname('api.tudominio.cl'),
			target: 'web:8080',
		})

		expect(domain.target).toEqual({ service: 'web', port: 8080 })
	})

	it('un destino inválido no llega al repositorio', async () => {
		const repo = new InMemoryDomainRepository()
		await expect(
			makeChangeTarget({ domains: repo })({
				app: APP,
				hostname: asHostname('api.tudominio.cl'),
				target: 'web',
			}),
		).rejects.toBeInstanceOf(ValidationError)
		expect(repo.calls).toEqual([])
	})
})
