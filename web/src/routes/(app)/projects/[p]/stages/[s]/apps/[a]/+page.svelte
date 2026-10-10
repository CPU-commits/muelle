<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { EmptyState, PageHeader } from '@/shared/ui'
	import type { PageProps } from './$types'

	let { params }: PageProps = $props()
</script>

<!-- Placeholder de F0: lo reemplaza F1-deploy-ingress con el Resumen (estado, réplicas,
     CPU/RAM, Desplegar y Rollback). El encabezado de la app lo pone F1-catalog. -->
<PageHeader title={params.a} titleSize="lg" />
<EmptyState icon="clock" title={m.common_wip_title()} body={m.common_wip_body()} />
