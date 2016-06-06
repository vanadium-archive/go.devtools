// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

var hg = require('mercury');
var h = require('mercury').h;
var request = require('superagent');
var cookie = require('cookie');

var pageHeaderComponent = require('./components/page-header');
var settingsPanelComponent = require('./components/settings');
var statusTableComponent = require('./components/status-table');
var metricActionsPanelComponent = require('./components/metric-actions-panel');

/**
 * A variable to store the most update-to-date dashboard data.
 *
 * We automatically retrive data from backend server every minute.
 */
var curData;

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
    startTimestamp: -1,
    endTimestamp: -1,
    oncallIds: ['_unknown', '_unknown'],
    loadingData: false,
    hasLoadingFailure: false
  }),

  components: hg.varhash({
    // The status table showing service status.
    statusTable: statusTableComponent(null),

    // The metric actions panel.
    metricActionsPanel: metricActionsPanelComponent(null)
  }),

  // Whether to show settings panel.
  showSettingsPanel: hg.value(false),

  // Settings stored in cookies.
  settings: hg.varhash({
    darkTheme: hg.value(cookies.darkTheme === 'true')
  }),

  channels: {
    mouseClickOnMetric: mouseClickOnMetric,
    changeTheme: changeTheme,
    clickOnSettingsGear: clickOnSettingsGear,
    closeSettingsPanel: closeSettingsPanel,
  }
});

/** Callback for clicking on a metric. */
function mouseClickOnMetric(state, data) {
  var metricActionPanelData = metricActionsPanelComponent({
    selectedMetric: data,
    selectedMetricIndex: 0,
    visible: true
  });
  state.components.put('metricActionsPanel', metricActionPanelData);
}

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
  if (state.components.statusTable) {
    mainContent.push(
      h('div.main-container',
          statusTableComponent.render(state, state.components.statusTable))
    );
  }
  mainContent.push(
    h('div.settings-gear', {
      'ev-click': hg.send(state.channels.clickOnSettingsGear)
    })
  );
  if (state.showSettingsPanel) {
    mainContent.push(hg.partial(settingsPanelComponent.render, state));
  }

  if (state.components.metricActionsPanel &&
      state.components.metricActionsPanel.visible) {
    mainContent.push(
        metricActionsPanelComponent.render(state.components.metricActionsPanel,
          curData));
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
      .timeout(30000)
      .end(function(err, res) {
    if (!res || !res.ok || err) {
      state.pageHeader.hasLoadingFailure.set(true);
    } else {
      state.pageHeader.hasLoadingFailure.set(false);
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
  updateComponents();

  // Update the data loading indicator.
  state.pageHeader.loadingData.set(false);
}

/**
 * Updates all page components.
 */
function updateComponents() {
  // Update page header.
  state.pageHeader.endTimestamp.set(curData.MaxTime);
  state.pageHeader.oncallIds.set(curData.Oncalls);

  // Update status table.
  var statusTableData = statusTableComponent({
    data: curData
  });
  state.components.put('statusTable', statusTableData);
}

// Add an event handler for closing settings panel when esc key is pressed.
document.onkeydown = function(evt) {
  if (evt.keyCode === 27) {
    if (state.showSettingsPanel()) {
      state.showSettingsPanel.set(false);
    }
    if (state.components.metricActionsPanel) {
      state.components.metricActionsPanel.visible.set(false);
    }
  }
};

hg.app(document.body, state, render);
loadData();
setInterval(loadData, 60000);
