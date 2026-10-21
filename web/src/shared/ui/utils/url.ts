export type QueryValue = string | number | boolean | null | undefined

/**
 * Una ruta ya resuelta con `resolve()` más un query string. `resolve()` no arma queries, y
 * concatenar en el `href` (`resolve(…) + '?deploy=7'`) lo marca eslint: esto va como
 * `href` de un componente de `@/shared/ui` (`LinkButton`, `ListRow`, `TextLink`, los items
 * de `Tabs`/`Menu`/`Breadcrumb`):
 *
 *   href={withQuery(resolve('/(app)/projects/[p]/stages/[s]/apps/[a]/deploys', params), { deploy: id })}
 *
 * Los `null` y `undefined` no van; el resto se pasa a texto y se codifica.
 */
export function withQuery(href: string, query: Readonly<Record<string, QueryValue>>): string {
	const params = new URLSearchParams()
	for (const [key, value] of Object.entries(query)) {
		if (value !== null && value !== undefined) params.set(key, String(value))
	}
	const search = params.toString()
	if (search === '') return href

	const hashAt = href.indexOf('#')
	const path = hashAt === -1 ? href : href.slice(0, hashAt)
	const hash = hashAt === -1 ? '' : href.slice(hashAt)
	return `${path}${path.includes('?') ? '&' : '?'}${search}${hash}`
}
