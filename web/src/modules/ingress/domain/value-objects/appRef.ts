// La app dueña de los dominios, por sus slugs (spec §10). Es la misma forma que el
// `AppRef` de `deploy`, pero ingress no depende de ese módulo.
export interface AppRef {
	readonly project: string
	readonly stage: string
	readonly app: string
}
