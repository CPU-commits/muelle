// Lo que un Field le pasa a su control para que label, error y lector de pantalla queden
// conectados sin que la pantalla invente ids: `<Input {...a11y} />`.
export interface FieldA11y {
	id: string
	'aria-invalid': boolean
	'aria-describedby': string | undefined
}
