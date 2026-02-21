<script lang="ts">
	import SearchBar from '$lib/components/SearchBar.svelte';
	import TorrentTable from '$lib/components/TorrentTable.svelte';
	import Pagination from '$lib/components/Pagination.svelte';
	import { search, fetchLatest, type SearchResponse } from '$lib/api';
	import { Loader2 } from 'lucide-svelte';
	import { onMount } from 'svelte';

	const categories = [
		{ value: undefined, label: 'All Categories' },
		{ value: 0, label: 'Movies' },
		{ value: 1, label: 'TV' },
		{ value: 2, label: 'Anime' }
	];

	const qualities = [
		{ value: undefined, label: 'All Qualities' },
		{ value: 1, label: 'SD' },
		{ value: 2, label: 'HD' },
		{ value: 3, label: 'FHD' },
		{ value: 4, label: 'UHD' }
	];

	let query = $state('');
	let page = $state(1);
	let category = $state<number | undefined>(undefined);
	let quality = $state<number | undefined>(undefined);
	let results = $state<SearchResponse | null>(null);
	let loading = $state(false);
	let error = $state<string | null>(null);

	let debounceTimer: ReturnType<typeof setTimeout>;

	async function loadTorrents() {
		clearTimeout(debounceTimer);
		debounceTimer = setTimeout(async () => {
			loading = true;
			error = null;

			try {
				if (query.trim()) {
					results = await search({
						q: query,
						page,
						limit: 50,
						category,
						quality
					});
				} else {
					results = await fetchLatest({
						page,
						limit: 50,
						category,
						quality
					});
				}
			} catch (err) {
				error = err instanceof Error ? err.message : 'Failed to load torrents';
			} finally {
				loading = false;
			}
		}, query.trim() ? 300 : 0);
	}

	function onInput(value: string) {
		query = value;
		page = 1;
		loadTorrents();
	}

	function onPageChange(p: number) {
		page = p;
		loadTorrents();
	}

	function onCategoryChange(e: Event) {
		const val = (e.target as HTMLSelectElement).value;
		category = val === '' ? undefined : Number(val);
		page = 1;
		loadTorrents();
	}

	function onQualityChange(e: Event) {
		const val = (e.target as HTMLSelectElement).value;
		quality = val === '' ? undefined : Number(val);
		page = 1;
		loadTorrents();
	}

	onMount(() => {
		loadTorrents();
	});
</script>

<div class="space-y-6">
	<div>
		<h1 class="text-xl font-semibold text-text-primary">Torrents</h1>
		<p class="mt-1 text-sm text-text-secondary">Browse and search discovered torrents</p>
	</div>

	<div class="space-y-3">
		<SearchBar value={query} {onInput} placeholder="Search torrents..." />

		<div class="flex gap-3">
			<select
				onchange={onCategoryChange}
				class="rounded-md border border-border bg-bg-tertiary px-3 py-2 text-sm text-text-primary focus:border-accent-blue focus:outline-none"
			>
				{#each categories as cat}
					<option value={cat.value ?? ''}>{cat.label}</option>
				{/each}
			</select>

			<select
				onchange={onQualityChange}
				class="rounded-md border border-border bg-bg-tertiary px-3 py-2 text-sm text-text-primary focus:border-accent-blue focus:outline-none"
			>
				{#each qualities as q}
					<option value={q.value ?? ''}>{q.label}</option>
				{/each}
			</select>
		</div>
	</div>

	{#if loading}
		<div class="flex items-center justify-center py-12">
			<Loader2 class="h-6 w-6 animate-spin text-text-secondary" />
		</div>
	{:else if error}
		<div class="rounded-lg border border-accent-red/30 bg-accent-red/10 p-4 text-sm text-accent-red">
			{error}
		</div>
	{:else if results}
		{#if results.results.length > 0}
			<div class="rounded-lg border border-border bg-bg-secondary">
				<TorrentTable results={results.results} />
				<Pagination
					page={page}
					total={results.total}
					limit={results.limit}
					onPageChange={onPageChange}
				/>
			</div>
		{:else}
			<div class="py-12 text-center text-sm text-text-secondary">
				{query.trim() ? `No results found for "${query}"` : 'No torrents discovered yet'}
			</div>
		{/if}
	{/if}
</div>
