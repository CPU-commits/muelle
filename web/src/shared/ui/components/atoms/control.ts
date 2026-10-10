// Vive fuera de `control.css.ts` a proposito: vanilla-extract serializa los
// exports de un `.css.ts` y rechaza cualquier cosa que no sea un valor plano.
export function isInvalid(value: unknown): boolean {
	return value === true || value === 'true'
}
