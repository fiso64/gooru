import type { ParamMatcher } from '@sveltejs/kit';

const appRouteSegments = new Set(['upload', 'uploads', 'jobs', 'tags', 'settings', 'shortcuts']);

export const match: ParamMatcher = (param) => appRouteSegments.has(param);
