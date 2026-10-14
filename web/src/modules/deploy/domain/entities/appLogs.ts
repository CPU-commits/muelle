// Los logs en vivo de un servicio (`GET …/apps/{a}/logs?service&tail|since&follow`, SSE): lo
// que Docker devuelve de sus tareas. muelle no guarda nada (spec §10).

export type LogStream = 'stdout' | 'stderr'
export type LogLevel = 'info' | 'warn' | 'error'

export interface AppLogLine {
	readonly time: Date | null
	/** La tarea que la escribió (`api.1`), si Docker lo dice. */
	readonly task: string | null
	readonly stream: LogStream
	/** La línea tal cual la escribió la app, sin los escapes ANSI. Nunca se recorta. */
	readonly text: string
	/** De qué color se pinta (ver `parseAppLogText`). */
	readonly level: LogLevel
}

/**
 * Por qué terminó el log (el evento `end` del contrato):
 * - `eof`: el servicio dejó de existir (o se pidió sin `follow`): no hay más que seguir;
 * - `timeout`: el stream llegó a su hora de vida; se puede volver a conectar enseguida.
 */
export type LogEndReason = 'eof' | 'timeout'

/** Lo que llega por el stream: primero `open` (conectado), las líneas y, a veces, `end`. */
export type AppLogEvent =
	| { readonly kind: 'open' }
	| { readonly kind: 'line'; readonly line: AppLogLine }
	| { readonly kind: 'end'; readonly reason: LogEndReason }

/** "Últimas N" del selector de líneas iniciales. `0` = solo lo nuevo. */
export const LOG_TAILS = [100, 200, 500] as const
export type LogTail = (typeof LOG_TAILS)[number] | 0
export const DEFAULT_LOG_TAIL: LogTail = 200

export interface AppLogsQuery {
	readonly service: string
	/** Las últimas N (sin `since`). */
	readonly tail: LogTail
	/**
	 * Al reconectar, en vez de `tail`: todo lo escrito desde este momento (inclusivo), hasta
	 * 5000. Así no se pierde lo que pasó mientras estaba cortado. Pendiente en el backend
	 * (Fase 2): mientras lo ignore, llegan las últimas 200 y el panel descarta lo repetido.
	 */
	readonly since?: Date
}

/** El máximo de `tail` que acepta el backend. */
export const MAX_LOG_TAIL = 5000

const LEVEL_WORDS: Record<string, LogLevel> = {
	trace: 'info',
	debug: 'info',
	info: 'info',
	notice: 'info',
	warn: 'warn',
	warning: 'warn',
	error: 'error',
	err: 'error',
	fatal: 'error',
	panic: 'error',
	crit: 'error',
	critical: 'error',
}

function levelFromWord(word: unknown): LogLevel | null {
	return typeof word === 'string' ? (LEVEL_WORDS[word.toLowerCase()] ?? null) : null
}

// Los escapes de terminal que traen los logs con color: CSI (`ESC [ … letra`: colores,
// cursor), OSC (`ESC ] … BEL` o `ESC ] … ESC \`: títulos, links) y los de dos caracteres.
const ANSI = new RegExp(
	[
		String.raw`\u001b\[[0-?]*[ -/]*[@-~]`,
		String.raw`\u001b\][^\u0007\u001b]*(?:\u0007|\u001b\\)`,
		String.raw`\u001b[@-Z\\-_]`,
	].join('|'),
	'g',
)

/** La línea sin los escapes ANSI (colores, cursor, links de terminal). */
export function stripAnsi(raw: string): string {
	return raw.includes('\u001b') ? raw.replace(ANSI, '') : raw
}

// Solo cuentan como nivel los tokens que una app escribe como nivel, no las palabras del
// mensaje ("error: relation …", "Error al conectar" son texto):
// - en mayúsculas y sueltos al principio de la línea, después de lo que suele ir antes
//   (fecha, hora, `[api]`): `WARN …`, `2026/10/10 14:00 ERROR …`;
// - entre corchetes, en cualquier caja: `[warn]`, `[ERROR]`;
// - `level=` (logfmt) y el campo `level`/`lvl`/`severity` de una línea JSON.
const UPPERCASE_LEVEL =
	/^(?:[\d\s:./T+-]+|\[[^\]]*\]\s*)*(TRACE|DEBUG|INFO|NOTICE|WARN(?:ING)?|ERROR|ERR|FATAL|PANIC|CRIT(?:ICAL)?)(?=[\s:]|$)/
const BRACKETED_LEVEL =
	/\[(trace|debug|info|notice|warn(?:ing)?|error|err|fatal|panic|crit(?:ical)?)\]/i
const LOGFMT_LEVEL = /(?:^|\s)(?:level|lvl|severity)=("?)([a-z]+)\1(?=\s|$)/i

function levelFromJson(raw: string): LogLevel | null | undefined {
	if (!raw.startsWith('{')) return undefined
	try {
		const value: unknown = JSON.parse(raw)
		if (typeof value !== 'object' || value === null || Array.isArray(value)) return undefined
		const record = value as Record<string, unknown>
		return levelFromWord(record.level ?? record.lvl ?? record.severity)
	} catch {
		return undefined
	}
}

/**
 * El nivel de una línea y su texto. Docker no tiene niveles: se leen de lo que escribió
 * la app, solo de tokens que son un nivel (ver arriba). Sin pistas, `info`, también en
 * stderr (muchas apps loguean todo ahí). El texto es la línea entera sin los escapes
 * ANSI: el nivel da color, no se corta nada.
 */
export function parseAppLogText(raw: string): { level: LogLevel; text: string } {
	const text = stripAnsi(raw)
	const json = levelFromJson(text)
	if (json !== undefined) return { level: json ?? 'info', text }
	const level =
		levelFromWord(UPPERCASE_LEVEL.exec(text)?.[1]) ??
		levelFromWord(BRACKETED_LEVEL.exec(text)?.[1]) ??
		levelFromWord(LOGFMT_LEVEL.exec(text)?.[2]) ??
		'info'
	return { level, text }
}

/** Suma líneas y se queda con las últimas `cap`. */
export function appendCapped<T>(lines: readonly T[], incoming: readonly T[], cap: number): T[] {
	if (incoming.length === 0)
		return lines.length > cap ? lines.slice(lines.length - cap) : [...lines]
	const all = [...lines, ...incoming]
	return all.length > cap ? all.slice(all.length - cap) : all
}

export interface AppLogFilter {
	/** Texto a buscar en el mensaje, sin distinguir mayúsculas. */
	readonly query: string
	/** Solo esta tarea (`api.1`); vacío = todas. */
	readonly task: string
}

export function matchesFilter(line: AppLogLine, filter: AppLogFilter): boolean {
	if (filter.task && line.task !== filter.task) return false
	const query = filter.query.trim().toLowerCase()
	return query.length === 0 || line.text.toLowerCase().includes(query)
}

/** Las tareas que aparecen en las líneas, ordenadas (`api.1`, `api.2`, `api.10`). */
export function logTasks(lines: readonly AppLogLine[]): string[] {
	const tasks = new Set<string>()
	for (const line of lines) if (line.task) tasks.add(line.task)
	return [...tasks].sort((a, b) => a.localeCompare(b, undefined, { numeric: true }))
}

/** Lo que identifica una línea dentro de un mismo milisegundo (para no repetirla). */
export function lineKey(line: AppLogLine): string {
	return `${line.task ?? ''}|${line.stream}|${line.text}`
}

/** La foto de una reconexión: desde cuándo, y cuántas veces llegó cada línea de ese ms. */
export interface ResumePoint {
	readonly since: Date
	readonly seen: ReadonlyMap<string, number>
}

/**
 * La hora de la última línea recibida y las que llegaron en ese mismo milisegundo, para
 * reconectar con `since` sin repetir. Se actualiza en el lugar: miles de líneas en el mismo
 * milisegundo no copian nada, y lo de un milisegundo se descarta cuando la hora avanza.
 */
export class ResumeTracker {
	#since: Date | null = null
	readonly #seen = new Map<string, number>()

	/** Una línea que se mostró (las sin hora, o con una hora anterior, no lo mueven). */
	note(line: AppLogLine): void {
		if (!line.time) return
		const time = line.time.getTime()
		const last = this.#since?.getTime() ?? Number.NEGATIVE_INFINITY
		if (time < last) return
		if (time > last) {
			this.#since = line.time
			this.#seen.clear()
		}
		const key = lineKey(line)
		this.#seen.set(key, (this.#seen.get(key) ?? 0) + 1)
	}

	/** La foto para la próxima conexión; null si todavía no llegó ninguna línea con hora. */
	point(): ResumePoint | null {
		return this.#since ? { since: this.#since, seen: new Map(this.#seen) } : null
	}

	clear(): void {
		this.#since = null
		this.#seen.clear()
	}
}

/**
 * Lo que descarta una conexión que siguió con `since` (inclusivo): todo lo de antes (un
 * backend que todavía no soporta `since` manda las últimas `tail` igual) y, de ese mismo
 * milisegundo, tantas copias de cada línea como ya se tenían. Nunca repite: si falta algo,
 * queda el hueco. Devuelve `true` para una línea repetida.
 */
export function makeResumeFilter(point: ResumePoint): (line: AppLogLine) => boolean {
	const from = point.since.getTime()
	const remaining = new Map(point.seen)
	return (line) => {
		if (!line.time) return false
		const time = line.time.getTime()
		if (time < from) return true
		if (time > from) return false
		const key = lineKey(line)
		const left = remaining.get(key) ?? 0
		if (left === 0) return false
		remaining.set(key, left - 1)
		return true
	}
}

/** La primera espera antes de reconectar un stream de logs que se cortó. */
export const RECONNECT_BASE_MS = 1000
/** La espera más larga entre dos intentos. */
export const RECONNECT_MAX_MS = 30_000

/** Lo que se espera antes del intento `attempt` (0, 1, 2…): 1, 2, 4, 8, 16 y 30 s de ahí en más. */
export function reconnectDelayMs(
	attempt: number,
	baseMs: number = RECONNECT_BASE_MS,
	maxMs: number = RECONNECT_MAX_MS,
): number {
	return Math.min(maxMs, baseMs * 2 ** Math.max(0, Math.floor(attempt)))
}
