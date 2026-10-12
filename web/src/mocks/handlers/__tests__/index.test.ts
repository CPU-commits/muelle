import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { server } from '@/mocks/server'
import { http } from '@/shared/data/http/client'
import { withDomainErrors } from '@/shared/data/http/domainErrors'
import { ApiError } from '@/shared/data/http/errors'
import { NotFoundError, pendingPhase } from '@/shared/domain'

// Como el backend: una ruta sin handler responde 404 `no_route`, y un repositorio de
// Fase 2 la lee como "todavía no existe".
beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
afterAll(() => server.close())

describe('handlers de MSW', () => {
	it('una ruta de la API sin handler responde 404 no_route', async () => {
		const error = await http
			.get('/projects/auditfy/stages/staging/apps/api/deploys')
			.catch((caught: unknown) => caught)

		expect(error).toBeInstanceOf(ApiError)
		expect((error as ApiError).status).toBe(404)
		expect((error as ApiError).code).toBe('no_route')
	})

	it('con { phase: 2 } esa respuesta es UnavailableError de la Fase 2', async () => {
		const error = await withDomainErrors(
			() => http.get('/projects/auditfy/stages/staging/apps/api/status'),
			{ phase: 2 },
		).catch((caught: unknown) => caught)

		expect(pendingPhase(error)).toBe(2)
	})

	it('sin fase, la misma respuesta es un NotFoundError común', async () => {
		await expect(withDomainErrors(() => http.get('/no-existe'))).rejects.toBeInstanceOf(
			NotFoundError,
		)
	})
})
