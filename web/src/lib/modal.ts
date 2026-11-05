/** Keeps keyboard focus inside a mounted dialog and returns it to the opener on close. */
export function modal(node: HTMLElement, close: () => void) {
	const previous = document.activeElement as HTMLElement | null;
	node.setAttribute('aria-modal', 'true');
	node.tabIndex = -1;
	const controls = () =>
		[
			...node.querySelectorAll<HTMLElement>(
				'button:not([disabled]), a[href], input:not([disabled]), textarea:not([disabled]), select:not([disabled]), [tabindex="0"]'
			)
		].filter((element) => element.getClientRects().length > 0);
	queueMicrotask(() => (controls()[0] ?? node).focus());
	function keydown(event: KeyboardEvent) {
		if (event.key === 'Escape') {
			event.preventDefault();
			event.stopPropagation();
			close();
		} else if (event.key === 'Tab') {
			const items = controls();
			const first = items[0] ?? node;
			const last = items.at(-1) ?? node;
			if (event.shiftKey && (document.activeElement === first || document.activeElement === node)) {
				event.preventDefault();
				last.focus();
			} else if (!event.shiftKey && (document.activeElement === last || !items.length)) {
				event.preventDefault();
				first.focus();
			}
		}
	}
	node.addEventListener('keydown', keydown, true);
	return {
		destroy() {
			node.removeEventListener('keydown', keydown, true);
			if (previous?.isConnected) previous.focus();
		}
	};
}
