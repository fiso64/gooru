from pathlib import Path


def replace_once(text: str, old: str, new: str, label: str) -> str:
    if old not in text:
        raise SystemExit(f"{label} not found")
    return text.replace(old, new, 1)


app = Path('frontend/src/lib/components/AuthenticatedApp.svelte')
text = app.read_text()
if 'async function changePassword(currentPassword: string, newPassword: string)' not in text:
    text = replace_once(
        text,
        "  async function logout() {\n",
        "  async function changePassword(currentPassword: string, newPassword: string) {\n    await new ApiClient($authState.csrfToken).changePassword(currentPassword, newPassword);\n  }\n\n  async function logout() {\n",
        'password callback',
    )
text = replace_once(
    text,
    "      <SettingsView username={$authState.user.username} onLogout={logout} />",
    "      <SettingsView username={$authState.user.username} onLogout={logout} onChangePassword={changePassword} />",
    'SettingsView password prop',
)
app.write_text(text)

css = Path('frontend/src/lib/styles/components.css')
text = css.read_text()
if '.settings-accent-control {' not in text:
    marker = ".inline-control .g-input {\n  max-width: 280px;\n}\n"
    additions = r'''

.settings-success {
  color: var(--ok);
}

.settings-accent-control,
.settings-range-control,
.settings-runtime-status,
.settings-server-row {
  display: flex;
  align-items: center;
  gap: 12px;
}

.settings-color-swatch {
  width: 32px;
  height: 32px;
  flex: 0 0 auto;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--accent);
}

.settings-color-value {
  width: 110px;
  flex: 0 0 110px;
  font-family: var(--font-mono);
  text-transform: uppercase;
}

.settings-color-presets {
  display: flex;
  gap: 6px;
}

.settings-color-dot {
  width: 22px;
  height: 22px;
  padding: 0;
  border: 1px solid var(--border);
  border-radius: 50%;
}

.settings-color-dot.sodium { background: #f4d976; }
.settings-color-dot.phosphor { background: #a3e635; }
.settings-color-dot.coral { background: #fb7185; }
.settings-color-dot.cyan { background: #7dd3fc; }
.settings-color-dot.mono { background: #e5e7eb; }

.settings-range-control input[type='range'] {
  width: 240px;
  accent-color: var(--accent);
}

.settings-range-control .g-mono,
.settings-runtime-status,
.settings-server-row,
.settings-mono-input {
  font-family: var(--font-mono);
  font-size: 12px;
}

.settings-server-row {
  min-height: 36px;
  padding: 6px 10px;
  border: 1px solid var(--border);
  border-radius: var(--r-3);
  background: var(--surface);
  color: var(--text-3);
}

.settings-runtime-status {
  min-height: 32px;
  color: var(--text-3);
}

.settings-status-dot {
  width: 7px;
  height: 7px;
  flex: 0 0 auto;
  border-radius: 50%;
  background: var(--text-4);
}

.settings-mono-input {
  max-width: 420px;
}
'''
    text = replace_once(text, marker, marker + additions, 'Settings styles')
css.write_text(text)

client_test = Path('frontend/src/lib/api/client.test.ts')
text = client_test.read_text()
if "it('changes passwords with CSRF and generated request fields'" not in text:
    marker = "  it('uploads files with async preference', async () => {"
    addition = r'''  it('changes passwords with CSRF and generated request fields', async () => {
    const requests: Array<{ url: string; method: string; headers: Headers; body: string }> = [];
    globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = new Request(input, init);
      requests.push({
        url: relativeURL(request.url),
        method: request.method,
        headers: request.headers,
        body: await request.clone().text()
      });
      return Response.json({ ok: true });
    }) as typeof fetch;

    await new ApiClient('secret-token').changePassword('old-secret', 'new-secret');

    expect(requests).toHaveLength(1);
    expect(requests[0].url).toBe('/api/v1/auth/change-password');
    expect(requests[0].method).toBe('POST');
    expect(requests[0].headers.get('Authorization')).toBeNull();
    expect(requests[0].headers.get('X-Gooru-CSRF')).toBe('secret-token');
    expect(JSON.parse(requests[0].body)).toEqual({ current_password: 'old-secret', new_password: 'new-secret' });
  });

'''
    text = replace_once(text, marker, addition + marker, 'password client test')
client_test.write_text(text)

docs = Path('docs/issue-21-concept-discrepancies.md')
text = docs.read_text()
old = '2. Settings password/config/library/appearance controls: A. They match plausible future server settings, but the backend does not expose mutable settings APIs in this issue. Controls stay greyed out with "coming soon" copy.'
new = '2. Settings password: B because `/api/v1/auth/change-password` is already a CSRF-protected backend endpoint and must be wired to the concept `Change…` action. Other config/library/appearance controls remain A because there is no mutable settings API for them in this issue.'
if old in text:
    text = text.replace(old, new, 1)
additions = [
    '34. Settings hot-reload/save-instantly copy: C. The concept claims edits write `gooru.yaml` and hot-reload the server, but no such API exists; visible copy must say configuration is server-managed instead.',
    '35. Settings bearer-token section: C. Transitional bearer-token runtime auth was removed when DB-backed sessions/CSRF landed, so the concept token UI is inaccurate and must not expose or invent `sk_live`-style credentials.',
    '36. Settings mock filesystem paths, public URLs, processor versions, executable paths, and upload limits: C. Do not present prototype values as server facts; use neutral server-managed/not-exposed copy while keeping the concept section geometry.',
    '37. Settings Server, Library, Appearance, and Media processing controls: A. Keep the concept surfaces visible and disabled where plausible, without leaking filesystem paths or pretending unsupported settings are writable.',
]
for addition in additions:
    if addition not in text:
        text = text.rstrip() + '\n' + addition + '\n'
docs.write_text(text)

spec = Path('frontend/tests/shell.spec.ts')
text = spec.read_text()
if "test('Settings preserves concept structure and changes password through CSRF'" not in text:
    text += r'''

test('Settings preserves concept structure and changes password through CSRF', async ({ page }) => {
  await mockAuth(page);
  await mockShellApis(page);
  await page.route('**/api/v1/files?**', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } }) });
  });

  const changes: Array<{ csrf: string; body: unknown }> = [];
  await page.route('**/api/v1/auth/change-password', async (route) => {
    changes.push({
      csrf: route.request().headers()['x-gooru-csrf'] ?? '',
      body: route.request().postDataJSON()
    });
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ ok: true }) });
  });

  await page.goto('/');
  await signIn(page);
  await page.locator('.sidebar').getByRole('button', { name: 'Settings' }).click();

  await expect(page.getByRole('heading', { name: 'Server & library settings' })).toBeVisible();
  for (const heading of ['Account', 'Appearance', 'Server', 'Library', 'Media processing']) {
    await expect(page.getByRole('heading', { name: heading, exact: true })).toBeVisible();
  }
  await expect(page.getByText('Auth tokens', { exact: true })).toHaveCount(0);
  await expect(page.getByLabel('Listen address')).toBeDisabled();
  await expect(page.getByLabel('Grid density coming soon')).toBeDisabled();
  await expect(page.getByText('filesystem paths are not exposed')).toBeVisible();

  await page.getByRole('button', { name: 'Change…' }).click();
  const dialog = page.getByRole('dialog', { name: 'Change password' });
  await dialog.getByLabel('Current password').fill('old-secret');
  await dialog.getByLabel('New password', { exact: true }).fill('new-secret');
  await dialog.getByLabel('Confirm new password').fill('new-secret');
  await dialog.getByRole('button', { name: 'Change password' }).click();

  await expect.poll(() => changes).toEqual([{ csrf: 'csrf-one', body: { current_password: 'old-secret', new_password: 'new-secret' } }]);
  await expect(page.getByRole('status')).toHaveText('Password updated.');
});
'''
spec.write_text(text)
