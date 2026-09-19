import { readFileSync } from 'node:fs';
const version = readFileSync('VERSION', 'utf8').trim();
const changelog = readFileSync('CHANGELOG.md', 'utf8');
const section = changelog.split(`## [${version}]\n`)[1]?.split('\n## [')[0]?.trim();
if (!section || !section.includes('- ')) throw Error(`Missing notes for ${version}`);
process.stdout.write(section + '\n');
