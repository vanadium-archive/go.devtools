// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/**
 * Support for mouse move event in mercury.
 */

var hg = require('mercury');
var extend = require('xtend');

module.exports = hg.BaseEvent(handleMouseMove);

function handleMouseMove(ev, broadcast) {
  var data = this.data;
  // Add absolute and relative X position to the existing data object.
  broadcast(extend(data, {
    x: ev.layerX,
    f: ev.layerX / ev.currentTarget.scrollWidth
  }));
}
