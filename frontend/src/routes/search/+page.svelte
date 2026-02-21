<script lang="ts">
	import SearchBar from '$lib/components/SearchBar.svelte';
	import TorrentTable from '$lib/components/TorrentTable.svelte';
	import Pagination from '$lib/components/Pagination.svelte';
	import { searchQuery, searchPage, searchCategory, searchQuality, searchResults, searchLoading, searchError, performSearch } from '$lib/stores/search';
	import { Loader2 } from 'lucide-svelte';

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

	function doSearch() {
		performSearch({
			q: $searchQuery,
			page: $searchPage,
			limit: 25,
			category: $searchCategory,
			quality: $searchQuality
		});
	}

	function onInput(value: string) {
		searchQuery.set(value);
		searchPage.set(1);
		doSearch();
	}

	function onPageChange(page: number) {
		searchPage.set(page);
		doSearch();
	}

	function onCategoryChange(e: Event) {
		const val = (e.target as HTMLSelectElement).value;
		searchCategory.set(val === '' ? undefined : Number(val));
		searchPage.set(1);
		doSearch();
	}

	function onQualityChange(e: Event) {
		const val = (e.target as HTMLSelectElement).value;
		searchQuality.set(val === '' ? undefined : Number(val));
		searchPage.set(1);
		doSearch();
	}
</script>

<div class="space-y-6">
	<div>
		<h1 class="text-xl font-semibold text-text-primary">Search</h1>
		<p class="mt-1 text-sm text-text-secondary">Search discovered torrents</p>
	</div>

	<div class="space-y-3">
		<SearchBar value={$searchQuery} {onInput} />

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

	{#if $searchLoading}
		<div class="flex items-center justify-center py-12">
			<Loader2 class="h-6 w-6 animate-spin text-text-secondary" />
		</div>
	{:else if $searchError}
		<div class="rounded-lg border border-accent-red/30 bg-accent-red/10 p-4 text-sm text-accent-red">
			{$searchError}
		</div>
	{:else if $searchResults}
		{#if $searchResults.results.length > 0}
			<div class="rounded-lg border border-border bg-bg-secondary">
				<TorrentTable results={$searchResults.results} />
				<Pagination
					page={$searchPage}
					total={$searchResults.total}
					limit={$searchResults.limit}
					{onPageChange}
				/>
			</div>
		{:else}
			<div class="py-12 text-center text-sm text-text-secondary">
				No results found for "{$searchQuery}"
			</div>
		{/if}
	{:else}
		<div class="py-12 text-center text-sm text-text-secondary">
			Enter a search term to find torrents
		</div>
	{/if}
</div>
