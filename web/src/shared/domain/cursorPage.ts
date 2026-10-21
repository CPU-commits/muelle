/**
 * Una página de una lista paginada por cursor (`/audit?before=…`): los items y el cursor
 * para pedir la siguiente, o `null` si no hay más. A diferencia de `Paginated<T>` no hay
 * total ni número de página: se carga "más" hasta que se acaba.
 */
export interface CursorPage<T> {
	readonly items: readonly T[]
	readonly nextCursor: string | null
}

export function emptyCursorPage<T>(): CursorPage<T> {
	return { items: [], nextCursor: null }
}

export function hasNextPage(page: CursorPage<unknown>): boolean {
	return page.nextCursor !== null
}

/** Las páginas de `createInfiniteQuery` (`data.pages`) como una sola lista. */
export function flattenCursorPages<T>(pages: readonly CursorPage<T>[]): T[] {
	return pages.flatMap((page) => page.items)
}
