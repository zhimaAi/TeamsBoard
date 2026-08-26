'use strict'

const readline = require('node:readline/promises')
const {
  loadReleaseSigningConfig,
  normalizeCertificateSha1,
} = require('./release-signing.cjs')

let signingInput

async function waitForManualSigningInput() {
  if (signingInput) {
    return signingInput
  }

  const config = loadReleaseSigningConfig()
  if (!process.stdin.isTTY) {
    throw new Error('Windows 发布签名需要交互式终端输入证书的 40 位 SHA-1 指纹。')
  }

  process.stdout.write(
    '\n[SimplySign] 构建已到达首次代码签名步骤。\n' +
      '请在 SimplySign Desktop 中人工完成登录，打开证书详情并复制 SHA-1 指纹。\n' +
      `签名程序：${config.signtoolPath}\n`,
  )

  const prompt = readline.createInterface({ input: process.stdin, output: process.stdout })
  try {
    while (true) {
      const value = await prompt.question('请粘贴 40 位证书指纹，按 Enter 开始签名：')
      try {
        signingInput = Object.freeze({
          ...config,
          certificateSha1: normalizeCertificateSha1(value),
        })
        return signingInput
      } catch (error) {
        process.stdout.write(`${error.message || error}\n`)
      }
    }
  } finally {
    prompt.close()
  }
}

module.exports = { waitForManualSigningInput }
