import { writable, derived } from 'svelte/store';
import { search, type SearchResponse, type SearchParams } from '$lib/api';

export const searchQuery = writable('');
export const searchPage = writable(1);
export const searchCategory = writable<number | undefined>(undefined);
export const searchQuality = writable<number | undefined>(undefined);
export const searchResults = writable<SearchResponse | null>(null);
export const searchLoading = writable(false);
export const searchError = writable<string | null>(null);

let debounceTimer: ReturnType<typeof setTimeout>;

export function performSearch(params: SearchParams) {
	clearTimeout(debounceTimer);
	debounceTimer = setTimeout(async () => {
		if (!params.q.trim()) {
			searchResults.set(null);
			return;
		}

		searchLoading.set(true);
		searchError.set(null);

		try {
			const result = await search(params);
			searchResults.set(result);
		} catch (err) {
			searchError.set(err instanceof Error ? err.message : 'Search failed');
		} finally {
			searchLoading.set(false);
		}
	}, 300);
}
