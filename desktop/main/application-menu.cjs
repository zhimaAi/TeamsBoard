'use strict'

function buildApplicationMenuTemplate(platform = process.platform) {
  const template = []
  if (platform === 'darwin') template.push({ role: 'appMenu' })
  template.push({ role: 'editMenu' })
  return template
}

function installApplicationMenu(Menu, platform = process.platform) {
  Menu.setApplicationMenu(Menu.buildFromTemplate(buildApplicationMenuTemplate(platform)))
}

module.exports = {
  buildApplicationMenuTemplate,
  installApplicationMenu,
}
