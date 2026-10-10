// Los logs en vivo de un servicio (`GET …/apps/{a}/logs?service&tail&follow`, SSE): lo
// que Docker devuelve de sus tareas. muelle no guarda nada (spec §10).

export type LogStream = 'stdout' | 'stderr'
export type LogLevel = 'info' | 'warn' | 'error'

export interface AppLogLine {
	readonly time: Date | null
	/** La tarea que la escribió (`api.1`), si Docker lo dice. */
	readonly task: string | null
	readonly stream: LogStream
	/** El mensaje, sin el nivel si venía como prefijo (`WARN …`) o en JSON. */
	readonly text: string
	readonly level: LogLevel
}

/** Lo que llega por el stream: primero `open` (conectado) y después las líneas. */
export type AppLogEvent =
	| { readonly kind: 'open' }
	| { readonly kind: 'line'; readonly line: AppLogLine }

/** "Últimas N" del selector de líneas iniciales. `0` = solo lo nuevo (al reconectar). */
export const LOG_TAILS = [100, 200, 500] as const
export type LogTail = (typeof LOG_TAILS)[number] | 0
export const DEFAULT_LOG_TAIL: LogTail = 200

export interface AppLogsQuery {
	readonly service: string
	readonly tail: LogTail
}

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

// `WARN algo`, `[warn] algo`, `ERROR: algo`.
const LEVEL_PREFIX = /^\s*\[?([a-z]+)\]?:?\s+(.*)$/is
// `level=warn` en logfmt.
const LOGFMT_LEVEL = /\blevel=("?)([a-z]+)\1/i

function fromJson(raw: string): { level: LogLevel | null; text: string } | null {
	if (!raw.startsWith('{')) return null
	try {
		const value: unknown = JSON.parse(raw)
		if (typeof value !== 'object' || value === null || Array.isArray(value)) return null
		const record = value as Record<string, unknown>
		const level = levelFromWord(record.level ?? record.lvl ?? record.severity)
		const message = record.msg ?? record.message
		return { level, text: typeof message === 'string' ? message : raw }
	} catch {
		return null
	}
}

/**
 * Saca el nivel de una línea y lo que se muestra como mensaje. Docker no tiene niveles:
 * se leen de lo que escribió la app (JSON con `level`, `level=` de logfmt o un prefijo
 * `WARN`). Sin pistas, es `info`, también en stderr (muchas apps loguean todo ahí).
 */
export function parseAppLogText(raw: string): { level: LogLevel; text: string } {
	const json = fromJson(raw)
	if (json) return { level: json.level ?? 'info', text: json.text }

	const prefix = LEVEL_PREFIX.exec(raw)
	const prefixLevel = levelFromWord(prefix?.[1])
	if (prefix && prefixLevel) return { level: prefixLevel, text: prefix[2] ?? '' }

	const logfmt = LOGFMT_LEVEL.exec(raw)
	return { level: levelFromWord(logfmt?.[2]) ?? 'info', text: raw }
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
