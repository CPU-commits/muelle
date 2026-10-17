import { describe, expect, it } from 'vitest'
import {
	appendLogChunk,
	EMPTY_LOG,
	flushLog,
	hasProxySyncWarning,
	logLineTone,
	logText,
	maskedSegments,
	parseLogLine,
} from '../deployLog'

describe('parseLogLine', () => {
	it('separa el timestamp RFC 3339 del mensaje', () => {
		const line = parseLogLine('2026-10-10T14:32:07Z resolviendo imágenes', 1)
		expect(line.time?.toISOString()).toBe('2026-10-10T14:32:07.000Z')
		expect(line.text).toBe('resolviendo imágenes')
		expect(line.raw).toBe('2026-10-10T14:32:07Z resolviendo imágenes')
	})

	it('acepta fracciones de segundo y offset', () => {
		const line = parseLogLine('2026-10-10T11:32:07.250-03:00 render ok', 2)
		expect(line.time?.toISOString()).toBe('2026-10-10T14:32:07.250Z')
	})

	it('sin timestamp, la línea entera es el mensaje', () => {
		const line = parseLogLine('Updating service auditfy_staging_api_api', 3)
		expect(line.time).toBeNull()
		expect(line.text).toBe('Updating service auditfy_staging_api_api')
	})
})

describe('logLineTone', () => {
	it.each([
		['$ docker stack deploy --with-registry-auth --prune -c - auditfy_staging_api', 'command'],
		['deploy fallido: la versión nueva no pasó el healthcheck', 'error'],
		['task: non-zero exit (1)', 'error'],
		['error: relation "findings_v2" does not exist', 'error'],
		['api.1 unhealthy · 3 healthchecks fallidos', 'warn'],
		['UpdateStatus: rollback_started', 'warn'],
		['UpdateStatus: completed', 'ok'],
		['deploy exitoso en 52 s', 'ok'],
		['timeout esperando convergencia', 'warn'],
		[
			'advertencia: no se pudo actualizar Caddy; el motivo está en el estado del proxy (GET /api/v1/proxy)',
			'warn',
		],
		['rutas de Caddy actualizadas', 'normal'],
		['esperando convergencia · timeout 300 s', 'normal'],
		['render ok · 1 servicio · 14 variables (6 secretas)', 'normal'],
	])('%s → %s', (text, tone) => {
		expect(logLineTone(text)).toBe(tone)
	})
})

describe('appendLogChunk', () => {
	it('arma las líneas y guarda lo que no terminó en \\n para el siguiente chunk', () => {
		let log = appendLogChunk(EMPTY_LOG, 'uno\ndos\ntr')
		expect(log.lines.map((line) => line.text)).toEqual(['uno', 'dos'])
		expect(log.partial).toBe('tr')

		log = appendLogChunk(log, 'es\ncuatro\n')
		expect(log.lines.map((line) => line.text)).toEqual(['uno', 'dos', 'tres', 'cuatro'])
		expect(log.lines.map((line) => line.n)).toEqual([1, 2, 3, 4])
		expect(log.partial).toBe('')
	})

	it('un chunk vacío no cambia nada', () => {
		const log = appendLogChunk(EMPTY_LOG, 'a\n')
		expect(appendLogChunk(log, '')).toBe(log)
	})

	it('saca el \\r de un fin de línea de Windows', () => {
		expect(appendLogChunk(EMPTY_LOG, 'hola\r\n').lines[0]?.text).toBe('hola')
	})

	it('se queda con las últimas `cap` líneas, sin perder la numeración', () => {
		let log = EMPTY_LOG
		for (let index = 1; index <= 7; index += 1) log = appendLogChunk(log, `l${index}\n`, 5)
		expect(log.lines.map((line) => line.text)).toEqual(['l3', 'l4', 'l5', 'l6', 'l7'])
		expect(log.lines[0]?.n).toBe(3)
		expect(log.count).toBe(7)
	})

	it('flushLog cierra una última línea sin \\n', () => {
		const log = flushLog(appendLogChunk(EMPTY_LOG, 'a\nb'))
		expect(log.lines.map((line) => line.text)).toEqual(['a', 'b'])
		expect(flushLog(log)).toBe(log)
	})

	it('logText devuelve el log tal cual llegó', () => {
		const log = appendLogChunk(EMPTY_LOG, '2026-10-10T14:32:07Z uno\ndos\ntres')
		expect(logText(log)).toBe('2026-10-10T14:32:07Z uno\ndos\ntres')
	})
})

describe('maskedSegments', () => {
	it('marca los *** que puso el backend en vez de un secreto', () => {
		expect(maskedSegments('postgres://auditfy:***@10.0.3.4:5432/auditfy')).toEqual([
			{ text: 'postgres://auditfy:', masked: false },
			{ text: '***', masked: true },
			{ text: '@10.0.3.4:5432/auditfy', masked: false },
		])
	})

	it('varios, al principio y al final', () => {
		expect(maskedSegments('*** y ***')).toEqual([
			{ text: '***', masked: true },
			{ text: ' y ', masked: false },
			{ text: '***', masked: true },
		])
	})

	it('sin secretos es un solo segmento', () => {
		expect(maskedSegments('render ok')).toEqual([{ text: 'render ok', masked: false }])
	})
})

// La advertencia del paso 7 (contrato §2.11) no trae el error de Caddy: el panel lo busca
// en GET /proxy cuando la ve en el log.
describe('hasProxySyncWarning', () => {
	const warning =
		'2026-10-16T12:00:08.800Z advertencia: no se pudo actualizar Caddy; el motivo está en el estado del proxy (GET /api/v1/proxy)\n'

	it('la ve en el log, con o sin el texto que sigue al ";"', () => {
		expect(hasProxySyncWarning(appendLogChunk(EMPTY_LOG, warning))).toBe(true)
		expect(
			hasProxySyncWarning(
				appendLogChunk(
					EMPTY_LOG,
					'2026-10-16T12:00:08Z advertencia: no se pudo actualizar Caddy\n',
				),
			),
		).toBe(true)
	})

	it('un deploy que actualizó las rutas, o una línea que solo la cita, no la tiene', () => {
		expect(
			hasProxySyncWarning(
				appendLogChunk(EMPTY_LOG, '2026-10-16T12:00:08Z rutas de Caddy actualizadas\n'),
			),
		).toBe(false)
		expect(
			hasProxySyncWarning(
				appendLogChunk(
					EMPTY_LOG,
					'web.1 | echo "advertencia: no se pudo actualizar Caddy"\n',
				),
			),
		).toBe(false)
		expect(hasProxySyncWarning(EMPTY_LOG)).toBe(false)
	})
})
