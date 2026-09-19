import { expect, test } from '@playwright/test';

test('autocomplete labels ellipsize instead of overflowing a narrow popup', async ({ page }) => {
  await page.goto('/');
  const layout = await page.evaluate(() => {
    const root = document.createElement('div');
    root.className = 'gooru-root';
    root.innerHTML = '<div class="tag-autocomplete"><ul class="tag-autocomplete-list" style="width:110px"><li><button style="display:flex;width:100%"><span>extremely_long_tag_component_repeated_extremely_long_tag_component</span><span class="count">12</span></button></li></ul></div>';
    document.body.append(root);
    const list = root.querySelector('ul')!;
    const label = root.querySelector('button > span')!;
    return { overflow: getComputedStyle(label).textOverflow, clipped: label.scrollWidth > label.clientWidth, horizontal: list.scrollWidth > list.clientWidth };
  });
  expect(layout).toEqual({ overflow: 'ellipsis', clipped: true, horizontal: false });
});
