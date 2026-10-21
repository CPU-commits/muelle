import { copyFileSync, existsSync, mkdirSync, rmSync } from 'node:fs'
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
 * en /tmp una clave maestra ni una base. Después falla si `removeDocker` falló o dejó algo.
 */
export async function cleanUpRun(
	files: RunFiles,
	removeDocker: () => Promise<string[]>,
	label: string,
): Promise<void> {
	let left: string[]
	try {
		left = await removeDocker()
	} finally {
		try {
			if (existsSync(files.logPath)) {
				mkdirSync(dirname(files.logCopy), { recursive: true })
				copyFileSync(files.logPath, files.logCopy)
			}
		} finally {
			rmSync(files.dir, { recursive: true, force: true })
		}
	}
	if (left.length > 0) throw new Error(`no se pudo borrar lo de ${label}: ${left.join(', ')}`)
}
