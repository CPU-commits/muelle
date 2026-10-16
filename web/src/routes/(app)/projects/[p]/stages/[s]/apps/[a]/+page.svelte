<script lang="ts">
	import { SummaryScreen } from '@/modules/deploy'
	import type { PageProps } from './$types'

	// El Resumen de la app: es dueño del encabezado grande (nombre, estado, Rollback y
	// Desplegar). El `kind`, los servicios del diálogo de deploy (contrato §2.12), los cambios
	// sin desplegar y el healthcheck los pide la pantalla al catálogo.
	let { params }: PageProps = $props()
</script>

<SummaryScreen project={params.p} stage={params.s} app={params.a} />
