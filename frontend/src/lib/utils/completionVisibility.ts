export function keepActiveCompletionVisible(list: HTMLElement | undefined) {
  if (!list) return;
  const active = list.querySelector<HTMLElement>('[role="option"][aria-selected="true"]');
  active?.scrollIntoView({ block: 'nearest', inline: 'nearest' });
}
