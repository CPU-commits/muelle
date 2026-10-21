import { mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { PREFIX } from './env'

// La carpeta temporal de una corrida del e2e real (globalSetup.ts).

export interface RunDir {
	/** `<parent>/muelle-it7-…`: la base, la clave maestra (0600) y el log. */
	dir: string
	logPath: string
	keyFile: string
	/** MUELLE_DATA_DIR. */
	dataDir: string
}

/**
 * Crea la carpeta de la corrida en `parent` y escribe la clave maestra que da `genkey`
 * (`muelle genkey`), en 0600. Corre antes de que haya una limpieza (la de `globalSetup.ts`
 * se arma cuando muelle ya arrancó): si `genkey` o la escritura fallan, borra la carpeta acá,
 * así no queda un `muelle-it7-*` vacío en /tmp.
 */
export function createRunDir(parent: string, genkey: () => Buffer | string): RunDir {
	const dir = mkdtempSync(join(parent, `muelle-${PREFIX}-`))
	const keyFile = join(dir, 'master.key')
	try {
		writeFileSync(keyFile, genkey(), { mode: 0o600 })
	} catch (error) {
		rmSync(dir, { recursive: true, force: true })
		throw error
	}
	return { dir, logPath: join(dir, 'muelle.log'), keyFile, dataDir: join(dir, 'data') }
}
