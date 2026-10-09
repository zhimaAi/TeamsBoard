'use strict'

const fs = require('node:fs')
const path = require('node:path')

const root = path.resolve(__dirname, '..', '..')
const version = require(path.join(root, 'desktop', 'package.json')).version

if (!/^\d+\.\d+\.\d+$/.test(version)) {
  throw new Error('desktop/package.json version must be major.minor.patch')
}

const files = [
  { name: 'desktop/package-lock.json', packageName: 'goteams-client-desktop', lock: true },
  { name: 'web/package.json', packageName: 'goteams-client-web', lock: false },
  { name: 'web/package-lock.json', packageName: 'goteams-client-web', lock: true },
]

function updateVersion(source, indent) {
  const pattern = new RegExp(`^( {${indent}}"version"\\s*:\\s*")[^"]+("[,]?)$`, 'm')
  if (!pattern.test(source)) {
    throw new Error(`missing version field at indentation ${indent}`)
  }
  return source.replace(pattern, (_, prefix, suffix) => `${prefix}${version}${suffix}`)
}

function syncVersion(checkOnly = false) {
  const changed = []
  for (const file of files) {
    const filePath = path.join(root, file.name)
    const source = fs.readFileSync(filePath, 'utf8')
    const parsed = JSON.parse(source)
    if (parsed.name !== file.packageName || (file.lock && parsed.packages?.['']?.name !== file.packageName)) {
      throw new Error(`unexpected package name in ${file.name}`)
    }
    let updated = updateVersion(source, 2)
    if (file.lock) updated = updateVersion(updated, 6)
    const result = JSON.parse(updated)
    if (result.version !== version || (file.lock && result.packages[''].version !== version)) {
      throw new Error(`failed to synchronize ${file.name}`)
    }
    if (updated !== source) {
      changed.push(file.name)
      if (!checkOnly) fs.writeFileSync(filePath, updated)
    }
  }
  if (checkOnly && changed.length > 0) {
    throw new Error(`version mismatch: ${changed.join(', ')}; run task version-sync`)
  }
  return changed
}

if (require.main === module) {
  const changed = syncVersion(process.argv.includes('--check'))
  console.log(`Product version ${version}; synchronized: ${changed.length ? changed.join(', ') : 'already current'}`)
}

module.exports = { syncVersion }
