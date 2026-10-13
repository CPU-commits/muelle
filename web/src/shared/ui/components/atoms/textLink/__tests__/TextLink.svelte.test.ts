import { createRawSnippet } from 'svelte'
import { describe, expect, it } from 'vitest'
import { render } from 'vitest-browser-svelte'
import { withQuery } from '../../../../utils/url'
import TextLink from '../TextLink.svelte'

const text = (value: string) => createRawSnippet(() => ({ render: () => `<span>${value}</span>` }))

describe('TextLink', () => {
	it('es un link con el href que recibe, también con un query de withQuery', async () => {
		const screen = await render(TextLink, {
			href: withQuery('/projects/auditfy/stages/staging/apps/api/deploys', { deploy: 7 }),
			children: text('ver el deploy #7'),
		})

		await expect
			.element(screen.getByRole('link', { name: 'ver el deploy #7' }))
			.toHaveAttribute('href', '/projects/auditfy/stages/staging/apps/api/deploys?deploy=7')
	})
})
