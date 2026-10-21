export interface Paginated<T> {
	readonly items: readonly T[]
	readonly total: number
	readonly page: number
	readonly pageSize: number
}

export const DEFAULT_PAGE_SIZE = 20

export function totalPages(page: Pick<Paginated<unknown>, 'total' | 'pageSize'>): number {
	if (page.pageSize <= 0) return 1
	return Math.max(1, Math.ceil(page.total / page.pageSize))
}

export function isEmpty(page: Paginated<unknown>): boolean {
	return page.items.length === 0
}

export function emptyPage<T>(pageSize: number = DEFAULT_PAGE_SIZE): Paginated<T> {
	return { items: [], total: 0, page: 1, pageSize }
}
