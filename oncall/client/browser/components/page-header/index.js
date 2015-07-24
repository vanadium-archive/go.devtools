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

var AppStateMgr = require('../../appstate-manager');

module.exports = create;
module.exports.render = render;

/** Constructor. */
function create(data) {
  var state = hg.state({
    // The timestamp when the current data was loaded from the backend server.
    collectionTimestamp: hg.value(data.collectionTimestamp),

    // IDs of current oncalls.
    oncallIds: hg.array(data.oncallIds),

    // Whether the data is being loaded.
    loadingData: hg.value(data.loadingData),

    // Whether there is any error loading data.
    hasLoadingFailure: hg.value(data.hasLoadingFailure),

    channels: {
      clickNavItem: clickNavItem
    }
  });

  return state;
}

/** Callback when a navigation item is clicked. */
function clickNavItem(state, data) {
  if (data.level ==='global') {
    AppStateMgr.setAppState({
      'level': data.level,
      'zoneLevelZone': '',
      'zoneLevelType': '',
      'instanceLevelInstance': '',
      'instanceLevelZone': ''
    });
  } else if (data.level === 'zone') {
    AppStateMgr.setAppState({
      'level': data.level,
      'instanceLevelInstance': '',
      'instanceLevelZone': ''
    });
  }
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
  if (state.collectionTimestamp >= 0) {
    var date = new Date(state.collectionTimestamp * 1000);
    strTime = dateformat(date);
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

  // Current view level and navigation items.
  var navTitle = '';
  var navItems = [];
  var level = AppStateMgr.getAppState('level');
  var zoneLevelZone = AppStateMgr.getAppState('zoneLevelZone');
  if (level === 'global') {
    navTitle = 'Global Status';
  } else if (level === 'zone') {
    var zoneType = AppStateMgr.getAppState('zoneLevelType')
        .startsWith('CloudService') ? 'Vanadium Services' : 'Nginx';
    navTitle = zoneType + ' @ ' + zoneLevelZone;
    navItems.push(h('div.navitems-container', [
      h('div.navitem', {
        'ev-click': hg.send(state.channels.clickNavItem, {level: 'global'})
      }, 'GLOBAL ←')
    ]));
  } else if (level === 'instance') {
    var instanceType = AppStateMgr.getAppState('instanceLevelInstance')
        .startsWith('vanadium') ? 'Vanadium Services' : 'Nginx';
    navTitle =
        instanceType + ' @ ' + AppStateMgr.getAppState('instanceLevelInstance');
    navItems.push(h('div.navitems-container', [
      h('div.navitem', {
        'ev-click': hg.send(state.channels.clickNavItem, {level: 'global'})
      }, 'GLOBAL ←'),
      h('div.navitem', {
        'ev-click': hg.send(state.channels.clickNavItem,
            {level: 'zone', zone: zoneLevelZone})
      }, zoneLevelZone.toUpperCase() + ' ←')
    ]));
  }

  navItems.push(h('div.navtitle', h('span', navTitle.toUpperCase())));
  return h('div.header', [
      h('div.info', [
        h('div.dashboard-title', [
          h('div#logo', ''),
          h('div.title-and-time', [
            h('div.title', 'Oncall Dashboard'),
            h('div' + timeClass, strTime)
          ])
        ]),
        h('div.navtitle-container', navItems),
        h('div.pics', pics)
      ])
  ]);
}
