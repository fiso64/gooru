# Third-party notices

Gooru is licensed under the GNU Affero General Public License v3.0 only (`AGPL-3.0-only`). Some dependencies and bundled assets remain under their own compatible licenses.

## Frontend runtime dependencies and assets

- **Comic Neue**, distributed through `@fontsource/comic-neue` 5.3.0, is licensed under the SIL Open Font License 1.1 (`OFL-1.1`). Copyright 2014 The Comic Neue Project Authors. Gooru's production frontend bundles the font files generated from this package.
- **TanStack Svelte Query**, distributed through `@tanstack/svelte-query`, is licensed under the MIT License.
- **openapi-fetch**, distributed through `openapi-fetch`, is licensed under the MIT License.

The frontend lockfile records the SPDX license identifiers for installed npm packages. Go and frontend dependencies are consumed through their package/module manifests rather than vendored source trees in this repository; their upstream copyright and license terms continue to apply. The Nix package installs this notice and Gooru's license under `share/doc/gooru`.
