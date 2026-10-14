<script lang="ts">
	import { SummaryScreen } from '@/modules/deploy'
	import type { PageProps } from './$types'

	// El Resumen de la app: es dueño del encabezado grande (nombre, estado, Rollback y
	// Desplegar). Lo del catálogo (`kind`, `pendingChanges`, `pendingChangesHref`) lo
	// conecta F2 con el barrel de `catalog`.
	let { params }: PageProps = $props()
</script>

<SummaryScreen project={params.p} stage={params.s} app={params.a} />
