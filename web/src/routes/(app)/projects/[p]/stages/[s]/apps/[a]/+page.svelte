<script lang="ts">
	import { SummaryScreen } from '@/modules/deploy'
	import type { PageProps } from './$types'

	// El Resumen de la app: es dueño del encabezado grande (nombre, estado, Rollback y
	// Desplegar). El `kind` y los servicios del diálogo de deploy los pide la pantalla al
	// catálogo (contrato §2.12); `pendingChanges` y `healthcheck` todavía no se pasan.
	let { params }: PageProps = $props()
</script>

<SummaryScreen project={params.p} stage={params.s} app={params.a} />
