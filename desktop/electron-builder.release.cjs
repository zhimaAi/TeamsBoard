'use strict'

module.exports = {
  extends: './electron-builder.yml',
  forceCodeSigning: true,
  win: {
    signtoolOptions: {
      sign: './build/simplysign-sign.cjs',
      signingHashAlgorithms: ['sha256'],
      rfc3161TimeStampServer: 'http://timestamp.sectigo.com',
      timeStampServer: 'http://timestamp.sectigo.com',
    },
  },
}
