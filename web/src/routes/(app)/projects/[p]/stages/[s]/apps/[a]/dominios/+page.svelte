<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { EmptyState, PageHeader } from '@/shared/ui'
</script>

<!-- Placeholder de F0: lo reemplaza F1-deploy-ingress con su pantalla. -->
<PageHeader title={m.nav_app_domains()} />
<EmptyState icon="clock" title={m.common_wip_title()} body={m.common_wip_body()} />
