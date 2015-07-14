// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/**
 * A simple data table.
 */

var hg = require('mercury');
var h = require('mercury').h;

module.exports = create;
module.exports.render = render;

/** Constructor. */
function create(data) {
  var state = hg.state({
    title: data.title,
    rows: data.rows
  });
  return state;
}

/** The main render function. */
function render(state) {
  return h('div.data-table', [
    h('div.data-table-title', state.title),
    h('table', state.rows.map(function(row) {
      return h('tr', row.map(function(col) {
        return h('td', col !== '' ? col : 'n/a');
      }));
    }))
  ]);
}
