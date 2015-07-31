// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/**
 * A settings panel.
 */

var hg = require('mercury');
var h = require('mercury').h;

module.exports = {
  render: render
};

/** The main render function. */
function render(state) {
  var darkTheme = state.settings.darkTheme ? true : false;

  var settingsContent = h('div.settings-content', [
      h('div.settings-title', 'Settings'),
      h('div.settings-item', [
        h('label', [
          h('input', {
            type: 'checkbox',
            checked: darkTheme,
            'ev-change': hg.send(
                state.channels.changeTheme, {darkTheme: !darkTheme})
          }),
          'Dark theme'
        ])
      ]),
      h('div.btn-close', {
        'ev-click': hg.send(state.channels.closeSettingsPanel)
      }, 'Close')
  ]);
  return h('div.settings-container', settingsContent);
}
