<script lang="ts">
	import { LogsScreen } from '@/modules/deploy'
	import type { PageProps } from './$types'

	let { params }: PageProps = $props()
</script>

<LogsScreen project={params.p} stage={params.s} app={params.a} />
