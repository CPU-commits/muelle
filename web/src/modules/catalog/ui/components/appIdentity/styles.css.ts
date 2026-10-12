import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

// La línea "Servicio · stack auditfy_staging_api · red auditfy_staging" bajo el título.
export const details = style({
	margin: 0,
	display: 'flex',
	flexWrap: 'wrap',
	gap: `${theme.space[4]} ${theme.space[16]}`,
	minWidth: 0,
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
})

export const item = style({
	display: 'inline-flex',
	alignItems: 'baseline',
	gap: theme.space[6],
	minWidth: 0,
	maxWidth: '100%',
})
