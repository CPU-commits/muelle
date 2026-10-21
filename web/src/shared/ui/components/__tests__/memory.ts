// Para los tests de memoria en el Chromium de vitest, que corre con `--expose-gc`
// (vite.config.ts): ¿quedó vivo algo que ya no tendría que estar referenciado?

type Gc = (options?: { type?: 'major' | 'minor'; execution?: 'sync' | 'async' }) => unknown

const nextTask = () => new Promise((done) => setTimeout(done, 0))
const nextFrame = () => new Promise((done) => requestAnimationFrame(() => done(undefined)))

/**
 * Un GC completo de verdad. `execution: 'async'` corre el GC en su propia tarea, con la
 * pila vacía: uno sincrónico revisa la pila de forma conservadora y a veces deja vivo un
 * nodo del DOM que nadie referencia (el test fallaba 2 de cada 10 veces). Las tareas y el
 * frame del medio sueltan las WeakRef leídas (viven hasta el final de su tarea) y lo que el
 * navegador tiene pendiente de esa tarea.
 */
export async function collectGarbage(): Promise<void> {
	const gc = (globalThis as { gc?: Gc }).gc
	if (!gc) throw new Error('falta --expose-gc en el Chromium de vitest (vite.config.ts)')
	await nextTask()
	await gc({ type: 'major', execution: 'async' })
	await nextFrame()
	await gc({ type: 'major', execution: 'async' })
}

/**
 * Cuántos de `refs` siguen vivos después de hasta `attempts` rondas de GC. Una fuga real
 * sobrevive a todas; algo que el navegador suelta un poco más tarde, no.
 */
export async function aliveAfterGc(
	refs: readonly WeakRef<object>[],
	attempts = 5,
): Promise<number> {
	let alive = refs.length
	for (let attempt = 0; attempt < attempts && alive > 0; attempt++) {
		await collectGarbage()
		alive = refs.filter((ref) => ref.deref() !== undefined).length
	}
	return alive
}
