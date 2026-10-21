// Paleta cruda del diseño de muelle (Claude Design, `Main.dc.html`). Nadie fuera de
// `shared/ui/themes` debería importar esto: los componentes consumen tokens semánticos
// (`theme.colors.text.secondary`), no hex.
export default {
	dark: {
		'gray-950': '#0A0A0B',
		'gray-925': '#0D0D0F',
		'gray-900': '#101012',
		'gray-875': '#141416',
		'gray-850': '#18181B',
		'gray-825': '#1A1A1D',
		'gray-800': '#222225',
		'gray-750': '#2E2E33',
		'gray-700': '#3A3A40',
		'gray-500': '#6E6E77',
		'gray-450': '#808089',
		'gray-400': '#83838C',
		'gray-300': '#A1A1A8',
		'gray-100': '#EDEDED',
		white: '#FFFFFF',
		'green-500': '#46C37B',
		'amber-500': '#F5A524',
		'amber-300': '#F5C26B',
		'red-500': '#F2555A',
		'red-400': '#F2777D',
		'red-600': '#C7343A',
		'red-700': '#B32D33',
	},
} as const
