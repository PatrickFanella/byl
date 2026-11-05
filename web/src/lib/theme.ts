import { writable } from 'svelte/store';

export type Appearance = 'system' | 'light' | 'dark';
export const appearanceKey = 'byl.appearance';
export const appearance = writable<Appearance>('system');
export const darkTheme = writable(false);

export function startTheme() {
	const media = window.matchMedia('(prefers-color-scheme: dark)');
	let preference: Appearance = 'system';
	try {
		const saved = localStorage.getItem(appearanceKey);
		if (saved === 'light' || saved === 'dark') preference = saved;
	} catch {
		/* Device storage can be unavailable. */
	}
	appearance.set(preference);
	const apply = () => {
		const dark = preference === 'dark' || (preference === 'system' && media.matches);
		document.documentElement.dataset.bylTheme = preference;
		document.documentElement.style.colorScheme = dark ? 'dark' : 'light';
		darkTheme.set(dark);
	};
	const unsubscribe = appearance.subscribe((value) => {
		preference = value;
		try {
			localStorage.setItem(appearanceKey, value);
		} catch {
			/* Keep the in-memory choice. */
		}
		apply();
	});
	media.addEventListener('change', apply);
	return () => {
		unsubscribe();
		media.removeEventListener('change', apply);
	};
}
