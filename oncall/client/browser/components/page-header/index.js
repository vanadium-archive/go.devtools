// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/**
 * The page header.
 *
 * In the page header panel, we show the following items:
 * - At the left side, we show the logo, title, and the date+time of the data
 *   currently shown in the dashboard.
 * - At the center, we show the current view level and navigation links to go
 *   back to higher levels.
 * - At the right side, we show the pictures of the current oncalls.
 */

var hg = require('mercury');
var h = require('mercury').h;
var dateformat = require('dateformat');

var staleDataThresholdInSec = 900;

module.exports = create;
module.exports.render = render;

/** Constructor. */
function create(data) {
  var state = hg.state({
    // The time period of the current data.
    startTimestamp: hg.value(data.startTimestamp),
    endTimestamp: hg.value(data.endTimestamp),

    // IDs of current oncalls.
    oncallIds: hg.array(data.oncallIds),

    // Whether the data is being loaded.
    loadingData: hg.value(data.loadingData),

    // Whether there is any error loading data.
    hasLoadingFailure: hg.value(data.hasLoadingFailure)
  });

  return state;
}

/** The main render function. */
function render(state) {
  // Oncalls' pictures.
  var pics = state.oncallIds.map(
    function(oncallId) {
      return h('img', {
        'src': 'pic?id=' + oncallId,
        'title': oncallId
      });
    }
  );

  // Timestamp for current data.
  var strTime = '';
  var timeClass = '.time';
  var infoClass = '.info';
  var staleData = false;
  if (state.endTimestamp >= 0) {
    var date = new Date(state.endTimestamp * 1000);
    strTime = dateformat(date);

    // Check stale data.
    var curTs = Math.round(new Date().getTime() / 1000.0);
    if (curTs - state.endTimestamp > staleDataThresholdInSec) {
      infoClass += '.stale-data';
      staleData = true;
    }
  }
  // It also shows whether the data is being loaded or errors.
  if (state.loadingData) {
    strTime = 'LOADING...';
    timeClass += '.loading';
  }
  if (state.hasLoadingFailure) {
    strTime = 'FAILED TO LOAD DATA';
    timeClass += '.failure';
  }
  if (staleData) {
    strTime = 'STALE DATA [' + strTime + ']';
    timeClass += '.failure';
  }

  return h('div.header', [
      h('div' + infoClass, [
        h('div.dashboard-title', [
          h('div#logo', ''),
        ]),
        h('div.title-and-time', [
          h('div.title', 'Vanadium Oncall Dashboard'),
          h('div' + timeClass, strTime)
        ]),
        h('div.pics', pics)
      ])
  ]);
}
