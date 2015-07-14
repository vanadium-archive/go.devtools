// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

var hg = require('mercury');
var h = require('mercury').h;
var request = require('superagent');

var AppStateMgr = require('./appstate-manager');
var instanceViewComponent = require('./components/instance-view');
var pageHeaderComponent = require('./components/page-header');
var summaryTableComponent = require('./components/summary-table');

/**
 * A variable to store the most update-to-date dashboard data.
 *
 * We automatically retrive data from backend server every minute.
 */
var curData;

// Initializes the app state manager and sets appStateChanged function as the
// callback for app state changes.
AppStateMgr.init(appStateChanged);

// Ask mercury to listen to mousemove/mouseout/mouseover events.
hg.Delegator().listenTo('mousemove');
hg.Delegator().listenTo('mouseout');
hg.Delegator().listenTo('mouseover');

/** Top level state. */
var state = hg.varhash({
  // The header panel at the top of the page.
  pageHeader: pageHeaderComponent({
    collectionTimestamp: -1,
    oncallIds: [],
    loadingData: false,
    hasLoadingFailure: false
  }),
  // The summary table showing data on the "global" and "zone" level.
  summaryTable: summaryTableComponent(null),
  // The view showing data on the "instance" level.
  instanceView: instanceViewComponent(null),
});

/** The main render function. */
var render = function(state) {
  var mainContent = [
    pageHeaderComponent.render(state.pageHeader),
  ];
  if (state.summaryTable) {
    mainContent.push(
        h('div.main-container',
          summaryTableComponent.render(state.summaryTable)));
  }
  if (state.instanceView) {
    mainContent.push(
        h('div.main-container',
          instanceViewComponent.render(state.instanceView)));
  }
  return h('div.main', mainContent);
};

/** Loads dashboard data from backend server. */
function loadData() {
  // Update data loading indicator.
  state.pageHeader.loadingData.set(true);

  // Get json dashboard data from "/data" endpoint.
  request
      .get('data')
      .accept('json')
      .end(function(err, res) {
    if (!res.ok || err) {
      state.pageHeader.hasLoadingFailure.set(true);
    } else {
      processData(res.body);
    }
  });
}

/**
 * Callback function of loadData.
 * @param {Object} newData - Newly loaded data in json format.
 */
function processData(newData) {
  // Update components.
  curData = newData;
  updateComponents(AppStateMgr.getCurState());

  // Update the data loading indicator.
  state.pageHeader.loadingData.set(false);
}

/**
 * Callback function for app state changes.
 * @param {Object} curAppState - App's current state object.
 */
function appStateChanged(curAppState) {
  if (!state) {
    return;
  }
  updateComponents(curAppState);
}

/**
 * Updates all page components.
 * @param {Object} curAppState - App's current state object.
 */
function updateComponents(curAppState) {
  // Update page header.
  state.pageHeader.collectionTimestamp.set(curData.CollectionTimestamp);
  state.pageHeader.oncallIds.set(curData.OncallIDs.split(','));

  // Update summary table when the current view level is NOT "instance".
  var summaryTableData = null;
  if (curAppState.level !== 'instance') {
    summaryTableData = summaryTableComponent({
      data: curData
    });
  }
  state.put('summaryTable', summaryTableData);

  // Update instance view when the current view level is "instance".
  var instanceViewData = null;
  if (curAppState.level === 'instance') {
    var instance = curAppState.instanceLevelInstance;
    instanceViewData = instanceViewComponent({
      collectionTimestamp: curData.CollectionTimestamp,
      data: curData.Zones[curAppState.instanceLevelZone].Instances[instance],
      appState: curAppState
    });
  }
  state.put('instanceView', instanceViewData);
}

hg.app(document.body, state, render);
loadData();
setInterval(loadData, 60000);
