import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

type TestMediaKind = 'photo' | 'video' | 'audio';

function fileItem(id: string, name: string, kind: TestMediaKind = 'photo') {
  return {
    id,
    content_id: `hash-${id}`,
    name,
    safe_display_path: `library/${name}`,
    size: kind === 'photo' ? 2048 : 104857600,
    modified_time: '2026-05-20T00:00:00Z',
    media_type: kind === 'video' ? 'video/mp4' : kind === 'audio' ? 'audio/mpeg' : 'image/jpeg',
    media_kind: kind,
    metadata: kind === 'video'
      ? { video_width: 1920, video_height: 1080, video_duration: 8 }
      : kind === 'audio'
        ? { audio_duration: 8 }
        : { image_width: 800, image_height: 600 },
    tags: [],
    media_urls: {
      thumbnail: `/api/v1/files/${id}/thumbnail`,
      preview: `/api/v1/files/${id}/preview`,
      content: `/api/v1/files/${id}/content`,
      download: `/api/v1/files/${id}/download`
    }
  };
}

async function mockApp(page: Page, files = [fileItem('one', 'one.jpg'), fileItem('two', 'two.jpg'), fileItem('three', 'three.jpg')]) {
  let loggedIn = false;

  await page.route('**/api/v1/auth/me', async (route) => {
    await route.fulfill({
      status: loggedIn ? 200 : 401,
      contentType: 'application/json',
      body: JSON.stringify(loggedIn ? session : { error: { code: 'unauthorized', message: 'login required' } })
    });
  });
  await page.route('**/api/v1/auth/login', async (route) => {
    loggedIn = true;
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) });
  });
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ files, total_count: files.length, library_count: files.length, facets: { kind: [] } })
  }));
  await page.route('**/api/v1/files/*/thumbnail', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><rect width="32" height="32"/></svg>' }));
  await page.route('**/api/v1/files/*/preview', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="96" height="64"><rect width="96" height="64"/></svg>' }));
  await page.route('**/api/v1/files/*/content', async (route) => route.fulfill({ status: 404, contentType: 'text/plain', body: 'media fixture intentionally unavailable' }));

  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
}

async function makeMediaControllable(page: Page, selector: 'video' | 'audio') {
  const media = page.locator(selector);
  await expect(media).toHaveCount(1);
  await media.evaluate((node) => {
    const element = node as HTMLMediaElement;
    element.style.width = '320px';
    element.style.height = '180px';
    element.style.display = 'block';
    let paused = false;
    let currentTime = 4;
    let playCalls = 0;
    let pauseCalls = 0;

    Object.defineProperty(element, 'paused', { configurable: true, get: () => paused });
    Object.defineProperty(element, 'duration', { configurable: true, get: () => 8 });
    Object.defineProperty(element, 'currentTime', {
      configurable: true,
      get: () => currentTime,
      set: (value) => { currentTime = Number(value); }
    });
    element.play = async () => {
      playCalls += 1;
      paused = false;
      element.dispatchEvent(new Event('play'));
    };
    element.pause = () => {
      pauseCalls += 1;
      paused = true;
      element.dispatchEvent(new Event('pause'));
    };
    (window as typeof window & { __viewerMediaState?: () => unknown }).__viewerMediaState = () => ({ paused, currentTime, playCalls, pauseCalls });
    element.dispatchEvent(new Event('loadedmetadata'));
  });
}

async function makeVideoControllable(page: Page) {
  await makeMediaControllable(page, 'video');
}

async function viewerMediaState(page: Page) {
  return page.evaluate(() => (window as typeof window & { __viewerMediaState?: () => unknown }).__viewerMediaState?.());
}

test('Delete and Shift+Delete remove a query-wide selection through confirmation dialogs', async ({ page }) => {
	await mockApp(page);
	const removals: unknown[] = [];
	await page.route('**/api/v1/files', async (route) => {
		if (route.request().method() !== 'DELETE') return route.fallback();
		removals.push(route.request().postDataJSON());
		await route.fulfill({
			contentType: 'application/json',
			body: JSON.stringify({ mode: (removals.at(-1) as { mode: string }).mode, selector: removals.at(-1), removed_locations: 3 })
		});
	});

	await page.keyboard.press('a');
	await expect(page.getByRole('button', { name: 'Untrack', exact: true })).toBeVisible();
	await expect(page.getByRole('button', { name: 'Delete', exact: true })).toBeVisible();

	await page.keyboard.press('Delete');
	const untrackDialog = page.getByRole('dialog', { name: 'Untrack selected files' });
	await expect(untrackDialog).toBeVisible();
	await untrackDialog.getByRole('button', { name: 'Untrack', exact: true }).click();
	await expect.poll(() => removals.length).toBe(1);
	expect(removals[0]).toEqual({ mode: 'untrack', query: '*' });
	await expect(page.getByText('3 of 3 selected')).toHaveCount(0);

	await page.keyboard.press('a');
	await page.keyboard.press('Shift+Delete');
	await expect(page.getByRole('dialog', { name: 'Delete selected files' })).toBeVisible();
	await page.getByRole('button', { name: 'Delete files' }).click();
	await expect.poll(() => removals.length).toBe(2);
	expect(removals[1]).toEqual({ mode: 'delete', query: '*' });
});

test('Escape clears selection even when a selected-file checkbox owns focus', async ({ page }) => {
  await mockApp(page);

  await page.getByRole('button', { name: 'Preview one.jpg' }).hover();
  await page.getByRole('checkbox', { name: 'Select one.jpg' }).click();
  const selected = page.getByRole('checkbox', { name: 'Deselect one.jpg' });
  await expect(page.getByText('1 of 3 selected')).toBeVisible();
  await expect(selected).toBeFocused();

  await page.keyboard.press('Escape');
  await expect(page.locator('.selection-bar')).toHaveCount(0);
});

test('pointer viewer controls return focus to the stage so shortcuts remain global', async ({ page }) => {
  await mockApp(page);

  await page.getByRole('button', { name: 'Preview one.jpg' }).click();
  await expect(page.getByRole('dialog', { name: 'one.jpg' })).toBeVisible();

  await page.getByLabel('Next file').click();
  await expect(page.getByRole('dialog', { name: 'two.jpg' })).toBeVisible();
  await expect(page.locator('.viewer-stage')).toBeFocused();

  await page.keyboard.press('r');
  await expect(page.locator('.viewer-visual-media')).toHaveAttribute('style', /rotate\(90deg\)/);
  await expect(page.locator('.viewer-stage')).toBeFocused();
});

test('viewer tag editor keeps Left and Right for caret movement', async ({ page }) => {
  await mockApp(page);

  await page.getByRole('button', { name: 'Preview two.jpg' }).click();
  const dialog = page.getByRole('dialog', { name: 'two.jpg' });
  await expect(dialog).toBeVisible();

  const tagInput = page.getByLabel('Tags for two.jpg');
  await tagInput.fill('portrait');
  await tagInput.evaluate((node) => {
    const input = node as HTMLInputElement;
    input.setSelectionRange(4, 4);
  });
  await expect(tagInput).toBeFocused();

  await page.keyboard.press('ArrowLeft');
  await expect(dialog).toBeVisible();
  await expect.poll(() => tagInput.evaluate((node) => (node as HTMLInputElement).selectionStart)).toBe(3);

  await page.keyboard.press('ArrowRight');
  await expect(dialog).toBeVisible();
  await expect.poll(() => tagInput.evaluate((node) => (node as HTMLInputElement).selectionStart)).toBe(4);
});

test('Space pauses the current video once and does not reactivate the seek control', async ({ page }) => {
  await mockApp(page, [fileItem('video', 'clip.mp4', 'video')]);

  await page.getByRole('button', { name: 'Preview clip.mp4' }).click();
  await makeVideoControllable(page);
  await page.locator('.viewer-stage').focus();

  await page.keyboard.press('Space');
  await expect.poll(() => viewerMediaState(page)).toEqual({ paused: true, currentTime: 4, playCalls: 0, pauseCalls: 1 });
  await page.waitForTimeout(75);
  expect(await viewerMediaState(page)).toEqual({ paused: true, currentTime: 4, playCalls: 0, pauseCalls: 1 });

  await page.getByRole('button', { name: 'Play video' }).click();
  await expect.poll(() => viewerMediaState(page)).toEqual({ paused: false, currentTime: 4, playCalls: 1, pauseCalls: 1 });

  const seek = page.getByRole('button', { name: 'Seek video' });
  await seek.click({ position: { x: 80, y: 8 } });
  await expect(page.locator('.viewer-stage')).toBeFocused();
  const afterSeek = await viewerMediaState(page) as { paused: boolean; currentTime: number; playCalls: number; pauseCalls: number };
  expect(afterSeek.currentTime).toBeGreaterThan(0);

  await page.keyboard.press('Space');
  const afterSpace = await viewerMediaState(page) as { paused: boolean; currentTime: number; playCalls: number; pauseCalls: number };
  expect(afterSpace).toMatchObject({ paused: true, playCalls: 1, pauseCalls: 2 });
  expect(afterSpace.currentTime).toBe(afterSeek.currentTime);
});

test('clicking video toggles playback and restores shortcut focus', async ({ page }) => {
  await mockApp(page, [fileItem('video', 'clip.mp4', 'video')]);

  await page.getByRole('button', { name: 'Preview clip.mp4' }).click();
  await makeVideoControllable(page);
  const video = page.locator('video');

  await video.click();
  await expect.poll(() => viewerMediaState(page)).toEqual({ paused: true, currentTime: 4, playCalls: 0, pauseCalls: 1 });
  await expect(page.locator('.viewer-stage')).toBeFocused();

  await video.click();
  await expect.poll(() => viewerMediaState(page)).toEqual({ paused: false, currentTime: 4, playCalls: 1, pauseCalls: 1 });
  await expect(page.locator('.viewer-stage')).toBeFocused();
});

test('Space toggles audio globally but remains text input inside the tag editor', async ({ page }) => {
  await mockApp(page, [fileItem('audio', 'song.mp3', 'audio')]);

  await page.getByRole('button', { name: 'Preview song.mp3' }).click();
  await makeMediaControllable(page, 'audio');
  await page.locator('.viewer-stage').focus();

  await page.keyboard.press('Space');
  await expect.poll(() => viewerMediaState(page)).toEqual({ paused: true, currentTime: 4, playCalls: 0, pauseCalls: 1 });
  await page.keyboard.press('Space');
  await expect.poll(() => viewerMediaState(page)).toEqual({ paused: false, currentTime: 4, playCalls: 1, pauseCalls: 1 });

  const tagInput = page.getByLabel('Tags for song.mp3');
  await tagInput.fill('music');
  await tagInput.press('Space');
  await expect(tagInput).toHaveValue('music ');
  expect(await viewerMediaState(page)).toEqual({ paused: false, currentTime: 4, playCalls: 1, pauseCalls: 1 });
});

test('Space after navigating to video controls the current item instead of the opener', async ({ page }) => {
  await mockApp(page, [fileItem('photo', 'photo.jpg'), fileItem('video', 'clip.mp4', 'video')]);

  await page.getByRole('button', { name: 'Preview photo.jpg' }).click();
  await page.getByLabel('Next file').click();
  await expect(page.getByRole('dialog', { name: 'clip.mp4' })).toBeVisible();
  await makeVideoControllable(page);

  await page.keyboard.press('Space');
  await expect.poll(() => viewerMediaState(page)).toEqual({ paused: true, currentTime: 4, playCalls: 0, pauseCalls: 1 });
  await expect(page.getByRole('dialog', { name: 'clip.mp4' })).toBeVisible();
});

test('filename search shortcut replaces only filename_contains and viewer shortcuts do not steal search focus', async ({ page }) => {
  await mockApp(page);
  const search = page.getByLabel('Search library');
  await search.fill('alpha');
  await search.press('Enter');
  await search.fill('@filename_contains:old.jpg');
  await search.press('Enter');
  await search.press('Escape');

  await page.keyboard.press('f');
  await expect(search).toBeFocused();
  await expect(search).toHaveValue('@filename_contains:');
  await expect(page.getByRole('button', { name: 'Remove alpha' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Remove @filename_contains:old.jpg' })).toHaveCount(0);
  await expect.poll(() => search.evaluate((node) => (node as HTMLInputElement).selectionStart)).toBe('@filename_contains:'.length);

  await search.press('Escape');
  await page.getByRole('button', { name: 'Preview one.jpg' }).click();
  const stage = page.locator('.viewer-stage');
  await stage.focus();
  await page.keyboard.press('/');
  await expect(search).not.toBeFocused();
  await expect(stage).toBeFocused();
});
