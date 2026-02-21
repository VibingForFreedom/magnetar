<script lang="ts">
	import { ChevronLeft, ChevronRight } from 'lucide-svelte';

	interface Props {
		page: number;
		total: number;
		limit: number;
		onPageChange: (page: number) => void;
	}

	let { page, total, limit, onPageChange }: Props = $props();

	let totalPages = $derived(Math.max(1, Math.ceil(total / limit)));
	let hasPrev = $derived(page > 1);
	let hasNext = $derived(page < totalPages);
</script>

<div class="flex items-center justify-between px-4 py-3">
	<span class="text-sm text-text-secondary">
		{total} results
	</span>
	<div class="flex items-center gap-2">
		<button
			onclick={() => onPageChange(page - 1)}
			disabled={!hasPrev}
			class="rounded border border-border p-1 text-text-secondary transition-colors hover:bg-bg-tertiary disabled:opacity-30 disabled:cursor-not-allowed"
		>
			<ChevronLeft class="h-4 w-4" />
		</button>
		<span class="text-sm text-text-secondary">
			{page} / {totalPages}
		</span>
		<button
			onclick={() => onPageChange(page + 1)}
			disabled={!hasNext}
			class="rounded border border-border p-1 text-text-secondary transition-colors hover:bg-bg-tertiary disabled:opacity-30 disabled:cursor-not-allowed"
		>
			<ChevronRight class="h-4 w-4" />
		</button>
	</div>
</div>
