import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

// "Exigir 2FA": título y explicación a la izquierda, el switch a la derecha; toda la fila
// es el <label>.
export const switchRow = style({
	display: 'flex',
	alignItems: 'flex-start',
	justifyContent: 'space-between',
	gap: theme.space[16],
	maxWidth: '560px',
	cursor: 'pointer',
})

export const switchText = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[2],
})

export const switchTitle = style({
	fontWeight: theme.typography.fontWeight.medium,
})

export const switchHint = style({
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
})

export const switchControl = style({
	marginTop: theme.space[2],
})

export const cidrs = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[8],
	maxWidth: '560px',
})

export const clientIp = style({
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
})

export const clientIpOut = style({
	color: theme.colors.status.warnText,
})

export const pending = style({
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
})
