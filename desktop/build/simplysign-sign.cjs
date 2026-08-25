'use strict'

const { execFileSync } = require('node:child_process')
const { buildSigntoolArguments } = require('./release-signing.cjs')
const { waitForManualSigningInput } = require('./manual-signing.cjs')

module.exports = async function simplySign(configuration) {
  const signing = await waitForManualSigningInput()
  const args = buildSigntoolArguments(
    signing.certificateSha1,
    configuration.path,
    configuration.hash,
  )

  try {
    execFileSync(signing.signtoolPath, args, {
      stdio: 'inherit',
      windowsHide: true,
    })
  } catch (error) {
    throw new Error(`Signtool 签名失败：${configuration.path}\n${error.message || error}`)
  }
}
