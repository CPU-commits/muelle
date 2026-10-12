import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

export const form = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[16],
})

export const row = style({
	display: 'grid',
	gridTemplateColumns: 'repeat(auto-fit, minmax(min(140px, 100%), 1fr))',
	gap: theme.space[16],
})

export const wide = style({ gridColumn: 'span 2' })

export const kind = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[8],
})

export const kindLabel = style({
	fontSize: theme.typography.fontSize[13],
	fontWeight: theme.typography.fontWeight.medium,
	color: theme.colors.text.secondary,
})

export const kindHint = style({
	margin: 0,
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
})

export const yaml = style({ minHeight: '200px' })
