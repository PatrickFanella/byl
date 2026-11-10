export function when(iso: string | null | undefined): string {
	if (!iso) return '';
	const d = new Date(iso);
	if (Number.isNaN(d.getTime())) return iso;
	const diff = (Date.now() - d.getTime()) / 1000;
	if (diff < 60) return 'just now';
	if (diff < 3600) return `${Math.floor(diff / 60)} min ago`;
	if (diff < 86400) return `${Math.floor(diff / 3600)} h ago`;
	return d.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
}

export function excerpt(s: string | null | undefined, n = 140): string {
	if (!s) return '';
	return s.length > n ? s.slice(0, n).trimEnd() + '…' : s;
}

/** Returns an http(s) URL for use as a link target, or undefined so stored javascript: and data: URLs never render as links. */
export function safeHref(url: unknown): string | undefined {
	return typeof url === 'string' && /^https?:\/\//i.test(url.trim()) ? url.trim() : undefined;
}
