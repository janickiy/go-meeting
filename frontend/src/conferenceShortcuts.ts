/** A meeting shortcut is only active outside text entry and modal controls. */
export function meetingShortcut(
  event: KeyboardEvent,
  allowed: readonly string[],
): string | null {
  if (
    event.defaultPrevented ||
    event.repeat ||
    event.isComposing ||
    event.ctrlKey ||
    event.metaKey ||
    event.altKey ||
    event.shiftKey
  )
    return null;
  const target = event.target;
  if (
    target instanceof Element &&
    target.closest(
      'input, textarea, select, [contenteditable]:not([contenteditable="false"]), [role="textbox"], [role="combobox"], [role="dialog"], [aria-modal="true"]',
    )
  )
    return null;
  const key = event.key.toLowerCase();
  return allowed.includes(key) ? key : null;
}
