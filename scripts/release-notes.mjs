import { readFileSync } from 'node:fs';

const version = readFileSync('VERSION', 'utf8').trim();
if (!/^\d+\.\d+\.\d+$/.test(version)) throw new Error('VERSION must be MAJOR.MINOR.PATCH');
const changelog = readFileSync('CHANGELOG.md', 'utf8');
const entries = changelog.split(/^## \[([^\]]+)\]\s*$/m);
const versions = entries.slice(1).filter((_, i) => i % 2 === 0);
if (versions.filter((entry) => entry === version).length !== 1) {
  throw new Error(`Expected exactly one changelog entry for ${version}`);
}
const index = versions.indexOf(version);
const notes = entries[index * 2 + 2].trim();
if (!/^### \S/m.test(notes) || !/^- \S/m.test(notes)) {
  throw new Error(`Changelog entry for ${version} must contain release notes`);
}
process.stdout.write(`${notes}\n`);
