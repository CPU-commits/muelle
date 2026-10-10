import { http, HttpResponse } from 'msw'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from 'vitest'
import { api } from '@/mocks/api'
import { authDb, MOCK_PASSWORD, MOCK_USERNAME } from '@/mocks/db/auth'
import { secretsDb } from '@/mocks/db/secrets'
import { server } from '@/mocks/server'
import { ContractError } from '@/shared/data/http/errors'
import { UnauthorizedError, UnavailableError, ValidationError } from '@/shared/domain'
import { authUseCases } from '@/modules/auth'
import { secretsUseCases } from '../container'
import {
	appOwner,
	EnvLimitExceeded,
	EnvOutOfScope,
	EnvOwnerNotFound,
	InvalidEnvKey,
	MAX_VALUE_BYTES,
	parseDotenv,
	planImport,
	stageOwner,
} from '../domain'

// Recorre el módulo entero —caso de uso, repositorio, datasource, cliente HTTP, schema
// Zod y mapper— contra el backend simulado por MSW (src/mocks/handlers/secrets.ts).
beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
beforeEach(async () => {
	authDb.reset()
	secretsDb.reset()
	await authUseCases.login({ username: MOCK_USERNAME, password: MOCK_PASSWORD })
})
afterEach(() => {
	server.resetHandlers()
	secretsDb.setOwnerResolver(null)
	secretsDb.setChangeListener(null)
})
afterAll(() => server.close())

const staging = stageOwner('auditfy', 'staging')
const apiApp = appOwner('auditfy', 'staging', 'api')

describe('variables de stage y app contra MSW', () => {
	it('lee las de la app ordenadas, con las secretas sin valor', async () => {
		const vars = await secretsUseCases.getEnv(apiApp)

		expect(vars.map((envVar) => envVar.key)).toEqual([
			'CORS_ORIGINS',
			'JWT_SECRET',
			'LOG_LEVEL',
			'PORT',
			'S3_ACCESS_KEY_ID',
			'S3_BUCKET',
			'S3_SECRET_ACCESS_KEY',
		])
		expect(vars.find((envVar) => envVar.key === 'JWT_SECRET')).toMatchObject({
			secret: true,
			value: null,
		})
		expect(vars.find((envVar) => envVar.key === 'PORT')).toMatchObject({
			secret: false,
			value: '3000',
		})
		expect(vars[0]?.updatedAt).toBeInstanceOf(Date)
	})

	it('agregar una secreta: vuelve enmascarada y avisa el cambio del dueño', async () => {
		const changes: string[] = []
		secretsDb.setChangeListener((target) => changes.push(target.app ?? target.stage))

		await secretsUseCases.patchEnv({ owner: apiApp, set: [['API_KEY', 'k-1']], secret: true })

		const vars = await secretsUseCases.getEnv(apiApp)
		expect(vars.find((envVar) => envVar.key === 'API_KEY')).toMatchObject({
			secret: true,
			value: null,
		})
		expect(changes).toEqual(['api'])
	})

	it('un PATCH que no cambia nada no marca cambios pendientes', async () => {
		const changes: string[] = []
		secretsDb.setChangeListener((target) => changes.push(target.app ?? target.stage))

		await secretsUseCases.patchEnv({ owner: apiApp, set: [['PORT', '3000']], secret: false })
		await secretsUseCases.patchEnv({ owner: apiApp, unset: ['NO_EXISTE'], secret: true })

		expect(changes).toEqual([])
	})

	it('editar, marcar como secreta y borrar en el stage', async () => {
		await secretsUseCases.patchEnv({ owner: staging, set: [['TZ', 'UTC']], secret: false })
		await secretsUseCases.patchEnv({
			owner: staging,
			set: [['NODE_ENV', 'staging']],
			secret: true,
		})
		await secretsUseCases.patchEnv({ owner: staging, unset: ['REDIS_URL'], secret: true })

		const vars = await secretsUseCases.getEnv(staging)
		expect(vars.find((envVar) => envVar.key === 'TZ')).toMatchObject({ value: 'UTC' })
		expect(vars.find((envVar) => envVar.key === 'NODE_ENV')).toMatchObject({
			secret: true,
			value: null,
		})
		expect(vars.find((envVar) => envVar.key === 'REDIS_URL')).toBeUndefined()
	})

	it('importar un .env reemplazando todas: el plan va en un solo PATCH', async () => {
		const current = await secretsUseCases.getEnv(apiApp)
		const parsed = parseDotenv('PORT=3000\nexport LOG_LEVEL=warn\nNEW="a b"\n')
		const plan = planImport(current, parsed.entries, { mode: 'replace', secret: false })
		expect(plan.patch).not.toBeNull()

		await secretsUseCases.patchEnv({
			owner: apiApp,
			set: plan.patch?.set ?? [],
			unset: plan.patch?.unset ?? [],
			secret: false,
			current,
		})

		const vars = await secretsUseCases.getEnv(apiApp)
		expect(vars.map((envVar) => [envVar.key, envVar.value])).toEqual([
			['LOG_LEVEL', 'warn'],
			['NEW', 'a b'],
			['PORT', '3000'],
		])
	})

	it('PUT reemplaza todo con un solo secret', async () => {
		await secretsUseCases.replaceEnv({ owner: staging, vars: [['ONLY', '1']], secret: true })

		await expect(secretsUseCases.getEnv(staging)).resolves.toMatchObject([
			{ key: 'ONLY', secret: true, value: null },
		])
	})

	it('__proto__ es una clave más de punta a punta', async () => {
		await secretsUseCases.patchEnv({ owner: apiApp, set: [['__proto__', 'x']], secret: false })

		const vars = await secretsUseCases.getEnv(apiApp)
		expect(vars.find((envVar) => envVar.key === '__proto__')).toMatchObject({ value: 'x' })
	})
})

describe('errores', () => {
	it('un dueño que no existe es EnvOwnerNotFound', async () => {
		secretsDb.setOwnerResolver((target) => target.app !== 'nada')
		const missing = appOwner('auditfy', 'staging', 'nada')

		await expect(secretsUseCases.getEnv(missing)).rejects.toBeInstanceOf(EnvOwnerNotFound)
		await expect(
			secretsUseCases.patchEnv({ owner: missing, unset: ['X'], secret: true }),
		).rejects.toBeInstanceOf(EnvOwnerNotFound)
	})

	it('el dominio frena lo que el backend rechazaría, sin llegar a la red', async () => {
		// Si la petición saliera, este handler la haría fallar con otro error.
		server.use(
			http.patch(api('/projects/:p/stages/:s/apps/:a/env'), () => {
				throw new Error('no debería mandarse')
			}),
		)

		await expect(
			secretsUseCases.patchEnv({
				owner: apiApp,
				set: [['COMPOSE_PROFILES', 'x']],
				secret: true,
			}),
		).rejects.toBeInstanceOf(InvalidEnvKey)
	})

	it('un 422 del backend llega como ValidationError con su campo (vars, set o unset)', async () => {
		// Una regla que el panel no conoce: el handler responde como lo haría el backend.
		server.use(
			http.patch(api('/projects/:p/stages/:s/apps/:a/env'), () =>
				HttpResponse.json(
					{ error: { code: 'invalid', message: 'unset: máximo 500 claves' } },
					{ status: 422 },
				),
			),
		)

		const error = await secretsUseCases
			.patchEnv({ owner: apiApp, unset: ['A'], secret: true })
			.catch((caught: unknown) => caught)

		expect(error).toBeInstanceOf(ValidationError)
		expect((error as ValidationError).field).toBe('unset')
		expect((error as ValidationError).message).toBe('máximo 500 claves')
	})

	it('el límite de 1 MiB del dueño lo revisa el backend cuando no se conocen las secretas', async () => {
		// 15 secretas de 64 KiB: el panel solo conoce sus claves.
		const big = 'a'.repeat(MAX_VALUE_BYTES)
		await secretsUseCases.replaceEnv({
			owner: staging,
			vars: Array.from({ length: 15 }, (_, index): [string, string] => [`S${index}`, big]),
			secret: true,
		})
		const current = await secretsUseCases.getEnv(staging)

		const error = await secretsUseCases
			.patchEnv({ owner: staging, set: [['MORE', big]], secret: true, current })
			.catch((caught: unknown) => caught)

		expect(error).toBeInstanceOf(ValidationError)
		expect(error).toMatchObject({
			field: 'set',
			message: 'las variables suman más de 1 MiB (claves y valores)',
		})
	})

	it('un cuerpo de más de 1 MiB no se manda', async () => {
		// 14 valores que el JSON escapa (\n ocupa 2 bytes): el dueño no llega a 1 MiB, el cuerpo sí.
		const lines = '\n'.repeat(40_000)
		const error = await secretsUseCases
			.replaceEnv({
				owner: staging,
				vars: Array.from({ length: 14 }, (_, index): [string, string] => [
					`K${index}`,
					lines,
				]),
				secret: true,
			})
			.catch((caught: unknown) => caught)

		expect(error).toBeInstanceOf(EnvLimitExceeded)
		expect(error).toMatchObject({ limit: 'request', field: 'vars' })
	})

	it('403 out_of_scope es EnvOutOfScope; 401 manda al login como UnauthorizedError', async () => {
		server.use(
			http.get(api('/projects/:p/stages/:s/env'), () =>
				HttpResponse.json(
					{ error: { code: 'out_of_scope', message: 'el token no tiene acceso' } },
					{ status: 403 },
				),
			),
		)
		await expect(secretsUseCases.getEnv(staging)).rejects.toBeInstanceOf(EnvOutOfScope)

		authDb.reset()
		await expect(secretsUseCases.getEnv(apiApp)).rejects.toBeInstanceOf(UnauthorizedError)
	})

	it('un backend caído es UnavailableError, y otra forma de respuesta ContractError', async () => {
		server.use(http.get(api('/projects/:p/stages/:s/env'), () => HttpResponse.error()))
		await expect(secretsUseCases.getEnv(staging)).rejects.toBeInstanceOf(UnavailableError)

		server.use(
			http.get(api('/projects/:p/stages/:s/apps/:a/env'), () =>
				HttpResponse.json({ vars: [{ key: 'X', value: 'filtrado', secret: true }] }),
			),
		)
		await expect(secretsUseCases.getEnv(apiApp)).rejects.toBeInstanceOf(ContractError)
	})
})
