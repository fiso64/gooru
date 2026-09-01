# Issue 21 Concept Discrepancy Matrix

This slice treats `temp/gooru-concept-ui/` as the visual source of truth and the current stacked frontend/backend wiring as the functional reference. Each concept/backend mismatch is classified as:

- A: keep visible but disabled, greyed out, or marked coming soon because it is a plausible future feature.
- B: implement now because the current backend/app should support it for this issue.
- C: remove or correct because it is not applicable or is inaccurate for Gooru.

1. Settings route and sidebar entry: B for making the settings view reachable and keeping account sign out functional; A for unsupported settings controls inside the view. The issue owner explicitly asked for the settings view, not a disabled settings button.
2. Settings password: B because `/api/v1/auth/change-password` is already a CSRF-protected backend endpoint and must be wired to the concept `Change…` action. Other config/library/appearance controls remain A because there is no mutable settings API for them in this issue.
3. Settings sign out: B. Session logout is already supported and should work from Settings.
4. Login first-run command: C. The concept text `gooru auth init` is inaccurate for this repository; visible copy must use the verified command from the current CLI/docs.
5. Login docs/source/changelog footer links: A. They remain visible as concept footer affordances but point at placeholder routes until docs pages exist.
6. Upload target selector: B. Upload targets are already configured and exposed through `/api/v1/upload-targets`; the concept selector should be wired to real targets.
7. Upload initial tags: B. The upload API already accepts initial tags, so the concept control should remain functional.
8. Upload conflict mode `skip` / `rename` / `replace`: B. The concept presents this as required import behavior, so this slice wires a request-level conflict policy into the upload endpoint instead of leaving the control decorative.
9. Upload duplicate detection text: B. The backend already hashes files and returns duplicate statuses; the copy can stay because the behavior is real and results are rendered per file.
10. Upload max 5 GB copy: C. No hard 5 GB product limit is guaranteed. The concept line is replaced with neutral browse/drop copy.
11. Upload Paste URL button: A. URL import is plausible but not currently backed by an endpoint; keep the concept button disabled/coming soon rather than hiding it.
12. Upload drag/drop and staged state: B. Drag/drop, staging, clear staged, remove staged item, and upload staged files are required workflows.
13. Upload queue result statuses/progress: B. Per-file staged/uploading/imported/duplicate/skipped/error/canceled rows are required and must stay aligned to the concept row grid.
14. Saved search create/update/delete actions: B. Backend endpoints exist, so the app-owned dialogs remain functional.
15. Search token pills/autocomplete: B. This is a central concept interaction and must be ported, not approximated with a plain input.
16. Search suggestions from backend: B. The concept autocomplete should use debounced API suggestions and cancellation rather than static prototype data.
17. Tag index/count surface: B. The backend exposes bounded tag counts, so Tags remains a real view.
18. Jobs view and drawer/status button: B. The backend exposes jobs, cancel, and clear actions.
19. Account view separate from Settings: A. The concept has account controls in Settings; the existing Account route can remain as a compact reachable utility view, but Settings is the primary concept location for sign out.
20. Shortcuts view: A. The concept includes it as a static/help surface; keep visible with applicable shortcuts and mark unsupported commands as future where needed.
21. Appearance controls in Settings: A. Accent/grid controls are concept features but are not persisted server settings in this issue.
22. Delete/replace/destructive media controls not currently exposed in the concept screens: C unless already backed by current backend behavior. Do not invent destructive UI that is not in the concept or backend requirement.
23. Concept mock media names, counts, paths, and seeded upload queue: C. Real app views must render actual API data or empty states; mock data can only appear in tests/screenshots.
24. Original active-content file serving from `/content`: B. The current media/content response policy must stay safe-by-default; the exact visual port must not weaken it.
25. Lightbox tag history/suggestion and passive info actions: A. Keep the concept affordances visible but disabled/coming soon until backed by real history/suggestion/detail APIs.
26. Lightbox remove-from-library action: B. The backend already exposes `DELETE /api/v1/files/{id}` with untrack semantics, so the concept trash action must untrack the location through an app-owned confirmation dialog; it must never imply disk deletion.
27. Lightbox add/remove tag interactions: B. Enter-to-add and per-tag removal map directly to the existing tag mutation API and should match the concept instead of exposing the functional-reference Add/Set/Remove button row.
28. Audio preview: B as a domain extension. The concept reference does not include an audio mock, but Gooru already supports audio files; keep a styled functional audio stage without changing the photo/video concept layout.
29. Jobs page and top-bar drawer row rendering: B. Use one shared concept-shaped row component so status, progress, timestamps, and cancel behavior cannot drift between the page and drawer.
30. Jobs pause-all control: A. The concept affordance remains visible in the drawer but disabled/coming soon because the backend has no pause/resume endpoint.
31. Concept job `done / total` counters: C. The backend exposes a normalized progress ratio but not authoritative item totals, so the real UI must render percentage/progress and timestamps rather than invent prototype counts.
32. Running-job cancel and clear-completed actions: B. Preserve the supported backend actions, but keep them visually subordinate (row/header hover or focus) so the resting Jobs surfaces remain faithful to the concept.
33. Shortcuts `?` launcher: B. The concept page explicitly tells the user to press `?` from anywhere, so that launcher is implemented as a real global shortcut outside text-entry controls. It accepts both the printable `?` key and the layout-stable Shift+Slash representation; other concept shortcut rows that are not implemented remain visible but greyed as A rather than falsely advertising behavior.
34. Settings hot-reload/save-instantly copy: C. The concept claims edits write `gooru.yaml` and hot-reload the server, but no such API exists; visible copy must say configuration is server-managed instead.
35. Settings bearer-token section: C. Transitional bearer-token runtime auth was removed when DB-backed sessions/CSRF landed, so the concept token UI is inaccurate and must not expose or invent `sk_live`-style credentials.
36. Settings mock filesystem paths, public URLs, processor versions, executable paths, and upload limits: C. Do not present prototype values as server facts; use neutral server-managed/not-exposed copy while keeping the concept section geometry.
37. Settings Server, Library, Appearance, and Media processing controls: A. Keep the concept surfaces visible and disabled where plausible, without leaking filesystem paths or pretending unsupported settings are writable.
38. Login version/build metadata: C. The concept hard-codes `v0.4.2` / `4f7a91d`, but the real unauthenticated health endpoint exposes only `status`; the identity strip keeps the exact three-part geometry while showing honest server readiness instead of fabricated release/build values.
39. Shared design primitives and self-hosted font weights: B. Buttons, inputs, segmented controls, cards, tags, focus rings, and default gallery geometry must use the concept source values directly; the exact IBM Plex 500/700 faces used by those rules are self-hosted rather than browser-synthesized.
40. Lightbox exact frame and real media behavior: B. Port the concept overlay/panel/metadata/tag/rail/navigation rendering directly while retaining the real photo/video/audio sources, video controls, tag mutations, download/open-original links, safe-content policy, and untrack workflow.
41. Tags index and shared page typography: B. Keep the real backend tag counts/filter/search routing, but use the concept page-heading tracking and Tags tile padding, mono type, namespace emphasis, count size, and hover treatment directly.
