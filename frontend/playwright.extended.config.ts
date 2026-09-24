import { defineConfig } from '@playwright/test';
import ordinaryConfig from './playwright.config';

// The formerly ungated browser specs remain runnable on demand while their
// outdated assertions and mocks are rehabilitated. They are not PR blockers.
export default defineConfig({
  ...ordinaryConfig,
  testDir: './tests',
  testMatch: '**/extended/*.spec.ts',
  testIgnore: 'smoke-upload-library.spec.ts'
});
