import { readFile, writeFile } from 'node:fs/promises'
import openapiTS, { astToString } from 'openapi-typescript'

const source = new URL('../../api/openapi.json', import.meta.url)
const destination = new URL('../src/shared/api/generated.ts', import.meta.url)
const tree = await openapiTS(source, { alphabetize: true })
const output = astToString(tree)
  .replace(/\/\*[\s\S]*?\*\//g, '')
  .replace(/^\s*\n/gm, '')

if (process.argv.includes('--check')) {
  const current = await readFile(destination, 'utf8').catch(() => '')
  if (current !== output) {
    console.error('API types have drifted. Run npm run generate:api in web/.')
    process.exitCode = 1
  }
} else {
  await writeFile(destination, output)
  console.log('Generated TypeScript API types from api/openapi.json.')
}
