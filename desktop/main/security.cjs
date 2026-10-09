'use strict'

const { shell } = require('electron')

function createOriginAllowlist(urls) {
  return new Set(urls.filter(Boolean).map(value => new URL(value).origin))
}

function isTrustedURL(rawURL, allowedOrigins) {
  try {
    return allowedOrigins.has(new URL(rawURL).origin)
  } catch {
    return false
  }
}

function installNavigationPolicy(win, allowedOrigins) {
  win.webContents.setWindowOpenHandler(({ url }) => {
    if (url.startsWith('https://')) void shell.openExternal(url)
    return { action: 'deny' }
  })

  win.webContents.on('will-navigate', (event, url) => {
    if (isTrustedURL(url, allowedOrigins)) return
    event.preventDefault()
    if (url.startsWith('https://')) void shell.openExternal(url)
  })
}

// 仅兼容已确认的官方 OSS 图片防盗链；Referer 不是用户鉴权凭证。
function installAssetRefererPolicy(sess, webContentsId) {
  if (!sess || !sess.webRequest) return
  const assetOrigin = 'https://goteams-cn.oss-cn-hangzhou.aliyuncs.com'
  sess.webRequest.onBeforeSendHeaders({ urls: [`${assetOrigin}/*`] }, (details, callback) => {
    const requestHeaders = { ...details.requestHeaders }
    try {
      const parsed = new URL(details.url)
      if (
        Number.isInteger(webContentsId) && webContentsId > 0 &&
        details.webContentsId === webContentsId &&
        details.method === 'GET' && details.resourceType === 'image' &&
        parsed.origin === assetOrigin && !parsed.username && !parsed.password
      ) {
        // HTTP header 名不区分大小写，避免与既有 referer 重复。
        for (const name of Object.keys(requestHeaders)) {
          if (name.toLowerCase() === 'referer') delete requestHeaders[name]
        }
        requestHeaders.Referer = 'https://goteams.cn/'
      }
    } catch {
      // 非法 URL 不改写请求头。
    }
    callback({ requestHeaders })
  })
}

module.exports = { createOriginAllowlist, installNavigationPolicy, installAssetRefererPolicy, isTrustedURL }
