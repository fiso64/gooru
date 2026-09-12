export type BuildInfo = { version: string; revision: string; dirty: boolean; development: boolean };

export async function getBuildInfo(): Promise<BuildInfo> {
  const response = await fetch('/api/v1/build', { headers: { Accept: 'application/json' } });
  if (!response.ok) throw new Error(`build info request failed: ${response.status}`);
  return response.json() as Promise<BuildInfo>;
}
