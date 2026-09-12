# Exhaustive moderator fallback

This file is a conditional supplement to `.github/maintainer-prompt.md`. **Do not read or execute it during normal moderator-index operation.** Read this file fresh and in full only when the main prompt determines that moderator state is missing, malformed, identity/marker-invalid, or stale.

The main maintainer prompt remains authoritative; its public-repository trust boundary and all other safety/engineering rules continue to apply.

For this fallback run only:

- enumerate every open in-scope issue/PR;
- read each latest issue discussion tail and each PR conversation/review tail through its actual chronological end, expanding farther back when unresolved context requires it;
- reconcile owner feedback and current `updated_at` values;
- list repository branches and classify no-PR branches against #29/live PR state;
- do one final lightweight open-item metadata refresh before choosing work.

This fallback is a safety net, not the normal path, and it does not maintain a durable sweep watermark. After the run, continue using moderator-index discovery once valid fresh moderator state is available again.