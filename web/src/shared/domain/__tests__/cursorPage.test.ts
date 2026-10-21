import { describe, expect, it } from 'vitest'
import { emptyCursorPage, flattenCursorPages, hasNextPage } from '../cursorPage'

describe('CursorPage', () => {
	it('hay más mientras el backend devuelva un cursor', () => {
		expect(hasNextPage({ items: [1, 2], nextCursor: '120' })).toBe(true)
		expect(hasNextPage({ items: [3], nextCursor: null })).toBe(false)
		expect(hasNextPage(emptyCursorPage<number>())).toBe(false)
	})

	it('junta las páginas en orden (lo que devuelve createInfiniteQuery)', () => {
		expect(
			flattenCursorPages([
				{ items: ['a', 'b'], nextCursor: '2' },
				{ items: ['c'], nextCursor: null },
			]),
		).toEqual(['a', 'b', 'c'])
	})
})
