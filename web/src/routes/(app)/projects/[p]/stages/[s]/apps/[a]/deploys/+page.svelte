<script lang="ts">
	import { page } from '$app/state'
	import { DeploysScreen } from '@/modules/deploy'
	import type { PageProps } from './$types'

	// `?deploy=144` abre ese deploy (lo usan "Desplegar", "Rollback" y el "Último deploy" del
	// Resumen). El `kind` y los servicios del diálogo de deploy los pide la pantalla al
	// catálogo (contrato §2.12).
	let { params }: PageProps = $props()
</script>

<DeploysScreen
	project={params.p}
	stage={params.s}
	app={params.a}
	deployParam={page.url.searchParams.get('deploy')}
/>
