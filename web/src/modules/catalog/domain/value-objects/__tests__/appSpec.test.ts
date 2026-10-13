import { describe, expect, it } from 'vitest'
import {
	type AppSpec,
	DEFAULT_HEALTHCHECK_TIMINGS,
	type Healthcheck,
	healthcheckPreview,
	InvalidAppSpec,
	isValidImage,
	newServiceSpec,
	parseAppSpec,
	SPEC_MESSAGES_ES,
	specFieldPath,
	specIssues,
	specsEqual,
} from '../..'

const http: Healthcheck = { type: 'http', path: '/health', ...DEFAULT_HEALTHCHECK_TIMINGS }

function spec(overrides: Partial<AppSpec> = {}): AppSpec {
	return { ...newServiceSpec('ghcr.io/aumi-app/backend-app:staging', 3000), ...overrides }
}

/** El primer campo con problema, como lo nombra el backend (o null si es válido). */
function firstField(candidate: AppSpec): string | null {
	return specIssues(candidate)[0]?.field ?? null
}

describe('appSpec', () => {
	it('un spec completo y válido pasa', () => {
		const full = spec({
			replicas: 0,
			command: ['node', 'dist/main.js'],
			healthcheck: http,
			volumes: [{ name: 'data', mount: '/data' }],
			resources: { memoryMb: 512, cpus: 0.5 },
		})
		expect(specIssues(full)).toEqual([])
		expect(parseAppSpec(full)).toBe(full)
	})

	it('acepta las imágenes que acepta el backend', () => {
		const sha40 = 'a'.repeat(40)
		const digest = 'b'.repeat(64)
		for (const image of [
			'nginx',
			'nginx:1.27',
			'nats:2.10-alpine',
			`ghcr.io/aumi-app/backend-app:sha-${sha40}`,
			`registry:5000/a/b@sha256:${digest}`,
			`ghcr.io/x/y:tag@sha256:${digest}`,
			'localhost/app',
			'Registry.Example.com/team/app:Latest',
		]) {
			expect(isValidImage(image), image).toBe(true)
		}
	})

	it('rechaza las imágenes que rechaza el backend', () => {
		for (const image of [
			'x y',
			'${X}',
			'--help',
			'Ghcr.io/Foo/Bar:Latest',
			'nginx\u0000',
			'../../containers/json?',
			'a'.repeat(64).replace(/a/g, 'f'),
			'redis@sha256:corto',
			`redis@md5:${'c'.repeat(64)}`,
			'UPPER',
		]) {
			expect(isValidImage(image), image).toBe(false)
		}
		expect(firstField(spec({ image: '' }))).toBe('image')
		expect(firstField(spec({ image: 'a'.repeat(5000) }))).toBe('image')
	})

	it('el límite de 255 es del path del repositorio, sin el registro (distribution/reference)', () => {
		const longRegistry = `${'r'.repeat(200)}.example.com`
		expect(isValidImage(`${longRegistry}/team/${'a'.repeat(50)}:v1`)).toBe(true)
		expect(isValidImage(`ghcr.io/${'a'.repeat(255)}`)).toBe(true)
		expect(isValidImage(`ghcr.io/${'a'.repeat(256)}`)).toBe(false)
		// Sin registro, el path es `library/<nombre>` (docker.io).
		expect(isValidImage('a'.repeat(247))).toBe(true)
		expect(isValidImage('a'.repeat(248))).toBe(false)
	})

	it('nombra los campos como el backend (TestParseRejects de appspec_test.go)', () => {
		const withHealth = (healthcheck: Healthcheck) => spec({ healthcheck })
		const cases: [string, AppSpec, string][] = [
			['puerto 0', spec({ port: 0 }), 'port'],
			['puerto alto', spec({ port: 70000 }), 'port'],
			['puerto con decimales', spec({ port: 80.5 }), 'port'],
			['puerto vacío', spec({ port: Number.NaN }), 'port'],
			['réplicas negativas', spec({ replicas: -1 }), 'replicas'],
			['demasiados argumentos', spec({ command: Array(65).fill('a') }), 'command'],
			['path sin barra', withHealth({ ...http, path: 'health' }), 'healthcheck.path'],
			...['&', ';', '|', '$', '`', '"', "'", ' '].map((char): [string, AppSpec, string] => [
				`path con ${char}`,
				withHealth({ ...http, path: `/h${char}x` }),
				'healthcheck.path',
			]),
			[
				'command vacío',
				withHealth({ type: 'command', command: [], ...DEFAULT_HEALTHCHECK_TIMINGS }),
				'healthcheck.command',
			],
			[
				'timeout alto',
				withHealth({ ...http, timeoutSeconds: 601 }),
				'healthcheck.timeoutSeconds',
			],
			[
				'start period negativo',
				withHealth({ ...http, startPeriodSeconds: -1 }),
				'healthcheck.startPeriodSeconds',
			],
			['reintentos 0', withHealth({ ...http, retries: 0 }), 'healthcheck.retries'],
			[
				'intervalo alto',
				withHealth({ ...http, intervalSeconds: 3601 }),
				'healthcheck.intervalSeconds',
			],
			[
				'volumen sin nombre',
				spec({ volumes: [{ name: '', mount: '/data' }] }),
				'volumes[0].name',
			],
			[
				'mount relativo',
				spec({ volumes: [{ name: 'd', mount: 'data' }] }),
				'volumes[0].mount',
			],
			[
				'mount sucio',
				spec({ volumes: [{ name: 'd', mount: '/data/' }] }),
				'volumes[0].mount',
			],
			['mount raíz', spec({ volumes: [{ name: 'd', mount: '/' }] }), 'volumes[0].mount'],
			[
				'mount con ..',
				spec({ volumes: [{ name: 'd', mount: '/a/../b' }] }),
				'volumes[0].mount',
			],
			[
				'mount con opciones',
				spec({ volumes: [{ name: 'd', mount: '/data:ro' }] }),
				'volumes[0].mount',
			],
			[
				'mount con variable',
				spec({ volumes: [{ name: 'd', mount: '/$HOME' }] }),
				'volumes[0].mount',
			],
			[
				'volumen repetido',
				spec({
					volumes: [
						{ name: 'd', mount: '/a' },
						{ name: 'd', mount: '/b' },
					],
				}),
				'volumes[1].name',
			],
			[
				'mount repetido',
				spec({
					volumes: [
						{ name: 'a', mount: '/d' },
						{ name: 'b', mount: '/d' },
					],
				}),
				'volumes[1].mount',
			],
			[
				'demasiados volúmenes',
				spec({
					volumes: Array.from({ length: 11 }, (_, i) => ({
						name: `v${i}`,
						mount: `/v${i}`,
					})),
				}),
				'volumes',
			],
			[
				'memoria baja',
				spec({ resources: { memoryMb: 8, cpus: null } }),
				'resources.memoryMb',
			],
			['cpus altas', spec({ resources: { memoryMb: null, cpus: 1000 } }), 'resources.cpus'],
		]
		for (const [name, candidate, field] of cases) {
			expect(firstField(candidate), name).toBe(field)
		}
	})

	it('acepta los paths de healthcheck del backend', () => {
		for (const path of ['/', '/health', '/a/b.c_d-e~f', '/h?x=1%20', '/h?a=b+c']) {
			expect(firstField(spec({ healthcheck: { ...http, path } })), path).toBeNull()
		}
	})

	it('devuelve todos los problemas, con los mensajes que se le pasen', () => {
		const issues = specIssues(spec({ port: 0, replicas: 99 }), {
			...SPEC_MESSAGES_ES,
			port: () => 'puerto malo',
		})
		expect(issues).toEqual([
			{ field: 'port', message: 'puerto malo' },
			{ field: 'replicas', message: SPEC_MESSAGES_ES.replicas() },
		])
	})

	it('parseAppSpec tira el primer problema como InvalidAppSpec', () => {
		try {
			parseAppSpec(spec({ port: 0 }))
			expect.unreachable()
		} catch (error) {
			expect(error).toBeInstanceOf(InvalidAppSpec)
			expect(error).toMatchObject({ field: 'port', message: 'debe estar entre 1 y 65535' })
		}
	})

	it('specFieldPath arma los nombres con corchetes', () => {
		expect(specFieldPath(['volumes', 2, 'mount'])).toBe('volumes[2].mount')
		expect(specFieldPath(['healthcheck', 'path'])).toBe('healthcheck.path')
		expect(specFieldPath([])).toBe('')
	})

	it('specsEqual no mira el orden de las claves', () => {
		const a = spec({ healthcheck: http })
		const b: AppSpec = JSON.parse(JSON.stringify({ ...a, healthcheck: { ...http } }))
		const reordered = Object.fromEntries(Object.entries(b).reverse()) as unknown as AppSpec
		expect(specsEqual(a, reordered)).toBe(true)
		expect(specsEqual(a, { ...a, replicas: 2 })).toBe(false)
	})

	it('la vista previa del healthcheck es la de §6.1', () => {
		expect(healthcheckPreview(http, 3000)).toBe(
			'["CMD", "wget", "-q", "-O", "/dev/null", "http://127.0.0.1:3000/health"]',
		)
		expect(healthcheckPreview({ ...http, type: 'bun-http' }, 8080)).toBe(
			`CMD bun -e "fetch('http://127.0.0.1:8080/health').then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))"`,
		)
		expect(
			healthcheckPreview(
				{ type: 'command', command: ['redis-cli', 'ping'], ...DEFAULT_HEALTHCHECK_TIMINGS },
				6379,
			),
		).toBe('["CMD", "redis-cli", "ping"]')
		expect(healthcheckPreview({ type: 'none' }, 80)).toBeNull()
	})
})
