import type { Reroute } from '@sveltejs/kit';
import { kitRouteForAppPath } from '$lib/utils/appRoute';

export const reroute: Reroute = ({ url }) => kitRouteForAppPath(url.pathname);
