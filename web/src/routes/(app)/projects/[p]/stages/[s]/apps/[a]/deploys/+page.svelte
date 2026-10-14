<script lang="ts">
	import { page } from '$app/state'
	import { DeploysScreen } from '@/modules/deploy'
	import type { PageProps } from './$types'

	// `?deploy=144` abre ese deploy (lo usan "Desplegar", "Rollback" y el "Último deploy" del
	// Resumen). El `kind` del catálogo, para el diálogo de deploy, lo conecta F2.
	let { params }: PageProps = $props()
</script>

<DeploysScreen
	project={params.p}
	stage={params.s}
	app={params.a}
	deployParam={page.url.searchParams.get('deploy')}
/>
