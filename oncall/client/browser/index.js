// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

var hg = require('mercury');
var h = require('mercury').h;
var request = require('superagent');
var cookie = require('cookie');

var AppStateMgr = require('./appstate-manager');
var instanceViewComponent = require('./components/instance-view');
var pageHeaderComponent = require('./components/page-header');
var settingsPanelComponent = require('./components/settings');
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

// Parse cookie.
var cookies = cookie.parse(document.cookie);

/** Top level state. */
var state = hg.state({
  // The header panel at the top of the page.
  pageHeader: pageHeaderComponent({
    collectionTimestamp: -1,
    oncallIds: ['_unknown', '_unknown'],
    loadingData: false,
    hasLoadingFailure: false
  }),

  components: hg.varhash({
    // The summary table showing data on the "global" and "zone" level.
    summaryTable: summaryTableComponent(null),

    // The view showing data on the "instance" level.
    instanceView: instanceViewComponent(null),
  }),

  // Whether to show settings panel.
  showSettingsPanel: hg.value(false),

  // Settings stored in cookies.
  settings: hg.varhash({
    darkTheme: hg.value(cookies.darkTheme === 'true')
  }),

  channels: {
    changeTheme: changeTheme,
    clickOnSettingsGear: clickOnSettingsGear,
    closeSettingsPanel: closeSettingsPanel
  }
});

/** Callback when user clicks on the settings gear. */
function clickOnSettingsGear(state) {
  state.showSettingsPanel.set(true);
}

/** Callback when user closes the settings panel. */
function closeSettingsPanel(state) {
  state.showSettingsPanel.set(false);
}

/** Callback when user changes theme. */
function changeTheme(state, data) {
  document.cookie = cookie.serialize('darkTheme', data.darkTheme, {
    maxAge: '315360000' // ten years.
  });
  state.settings.darkTheme.set(data.darkTheme);
}

/** The main render function. */
var render = function(state) {
  var mainContent = [
    pageHeaderComponent.render(state.pageHeader),
  ];
  if (state.components.summaryTable) {
    mainContent.push(
        h('div.main-container',
          summaryTableComponent.render(state.components.summaryTable)));
  }
  if (state.components.instanceView) {
    mainContent.push(
        h('div.main-container',
          instanceViewComponent.render(state.components.instanceView)));
  }
  mainContent.push(h('div.settings-gear', {
    'ev-click': hg.send(state.channels.clickOnSettingsGear)
  }));
  if (state.showSettingsPanel) {
    mainContent.push(hg.partial(settingsPanelComponent.render, state));
  }

  var className = state.settings.darkTheme ? 'main.darkTheme' : 'main';
  return h('div.' + className, mainContent);
};

/** Loads dashboard data from backend server. */
function loadData() {
  // Update data loading indicator.
  state.pageHeader.loadingData.set(true);

  // Get json dashboard data from "/data" endpoint.
  var dataEndpoint = window.location.origin + window.location.pathname + 'data';
  request
      .get(dataEndpoint)
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
  state.components.put('summaryTable', summaryTableData);

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
  state.components.put('instanceView' ,instanceViewData);
}

// Add an event handler for closing settings panel when esc key is pressed.
document.onkeydown = function(evt) {
  if (evt.keyCode === 27 && state.showSettingsPanel()) {
    state.showSettingsPanel.set(false);
  }
};

hg.app(document.body, state, render);
loadData();
setInterval(loadData, 60000);
