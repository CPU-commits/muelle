import { existsSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { dirname } from 'node:path'

// La limpieza de una corrida del e2e real (globalSetup.ts), ya con muelle apagado.

export interface RunFiles {
	/** La carpeta temporal de la corrida: la base, la clave maestra (0600) y el log. */
	dir: string
	/** El log de muelle, adentro de `dir`. */
	logPath: string
	/** A dónde se copia el log para mirarlo después (test-results/real-muelle.log). */
	logCopy: string
}

/**
 * Borra lo de Docker de la corrida con `removeDocker` (devuelve lo que no pudo borrar) y,
 * pase lo que pase con Docker (también si el CLI falla), copia el log y borra `dir`: no queda
 * en /tmp una clave maestra ni una base. Después falla, con un solo error que dice todo lo
 * que pasó, si `removeDocker` falló o dejó algo, o si muelle registró un error interno (un
 * bug, aunque el panel no lo haya mostrado).
 */
export async function cleanUpRun(
	files: RunFiles,
	removeDocker: () => Promise<string[]>,
	label: string,
): Promise<void> {
	const problems: string[] = []
	let dockerFailure: unknown
	try {
		const left = await removeDocker()
		if (left.length > 0) problems.push(`no se pudo borrar lo de ${label}: ${left.join(', ')}`)
	} catch (error) {
		dockerFailure = error
		problems.push(error instanceof Error ? error.message : String(error))
	} finally {
		try {
			if (existsSync(files.logPath)) {
				const log = readFileSync(files.logPath, 'utf8')
				mkdirSync(dirname(files.logCopy), { recursive: true })
				writeFileSync(files.logCopy, log)
				const internal = internalErrors(log)
				if (internal.length > 0) {
					problems.push(
						`muelle registró errores internos (${files.logCopy}):\n${internal.join('\n')}`,
					)
				}
			}
		} finally {
			rmSync(files.dir, { recursive: true, force: true })
		}
	}
	if (problems.length > 0) {
		throw new Error(
			problems.join('\n'),
			dockerFailure === undefined ? {} : { cause: dockerFailure },
		)
	}
}

/**
 * Los `msg` de un error interno en el log de muelle: el de `Kit.Fail` (`cmd/http/utils`, un
 * 500) y el del despachador de deploys (`deploy/app/usecases/dispatcher.go`, que no responde
 * nada: el deploy queda `running` y se reintenta).
 */
const INTERNAL_ERROR_MESSAGES: readonly unknown[] = ['error interno', 'deploy: error interno']

/** Las líneas del log JSON de muelle con un error interno. */
function internalErrors(log: string): string[] {
	return log.split('\n').filter((line) => {
		try {
			const entry = JSON.parse(line) as { level?: unknown; msg?: unknown }
			return entry.level === 'ERROR' && INTERNAL_ERROR_MESSAGES.includes(entry.msg)
		} catch {
			return false
		}
	})
}
