// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/**
 * App state manager.
 */

var hg = require('mercury');
var qs = require('qs');
var url = require('url');

module.exports = {
  init: init,
  getAppState: getAppState,
  setAppState: setAppState,
  getCurState: getCurState,
};

/**
 * An observable to store app's state which will be encoded in url parameters.
 */
var appState = hg.varhash({});

/** The default app state. */
var defaultAppState = {
  // The level of view.
  // - global: all (aggregated) metrics in all zones.
  // - zone: all metrics in a specific zone.
  // - instance: all metrics for a specific instance.
  level: 'global',

  // The data aggregation type for global view.
  // In global view, each metric (e.g. nginx qps) in a certain zone might have
  // multiple instances (e.g. multiple nginx workers). We need to aggregate data
  // from all those instances to a single one.
  // We currently support 'Max' and 'Average'.
  globalLevelAggType: 'Max',

  // The zone to show in the zone level.
  zoneLevelZone: '',

  // The type of metrics to show in the zone level.
  // This could either be 'CloudServices' or 'Nginx'.
  zoneLevelType: '',

  // The instance to show in the instance level.
  instanceLevelInstance: '',

  // The zone of the instance in the instance level.
  instanceLevelZone: ''
};

/**
 * A flag indicating whether to trigger history.pushState when
 * app state changes.
 */
var pushHistoryState = true;

/**
 * Sets up app state and its various event listeners.
 * @param {callback} stateChangedListener - The callback that handles app state
 *     changes.
 */
function init(stateChangedListener) {
  // When appState changes, push the state to browser's history.
  appState(function(data) {
    if (pushHistoryState) {
      var str = qs.stringify(data);
      window.history.pushState(undefined, '',
          window.location.origin + window.location.pathname + '?' + str);
    }
    stateChangedListener(getCurState());
  });

  // Get the current state from url parameters.
  var initState = qs.parse(url.parse(window.location.href).query);
  // Fill in default values.
  Object.keys(defaultAppState).forEach(function(key) {
    if (!initState[key]) {
      initState[key] = defaultAppState[key];
    }
  });
  appState.set(initState);

  // When the history state changes, we update the appState observable
  // which will trigger its change listener defined above.
  window.addEventListener('popstate', function(event) {
    // We don't want to mess with history states in this case.
    pushHistoryState = false;
    appState.set(qs.parse(url.parse(window.location.href).query));
    pushHistoryState = true;
  });
}

/**
 * Gets an app state entry for the given name.
 * @param {string} name - The name of the app state entry.
 * @return {string}
 */
function getAppState(name) {
  return appState()[name];
}

/**
 * Sets app state.
 * @param {Object} stateObj - The state object to set.
 */
function setAppState(stateObj) {
  var curState = appState();
  Object.keys(stateObj).forEach(function(key) {
    curState[key] = stateObj[key];
  });
  appState.set(curState);
}

/**
 * Gets the current app state.
 * @return {Object}
 */
function getCurState() {
  // For some reason, the object returned by appState() doesn't have
  // Object.prototype as its prototype, which will cause issues in some other
  // places. To workaround this, we return a cloned object which has
  // Object.prototype.
  var obj = {};
  var curAppState = appState();
  Object.keys(curAppState).forEach(function(key) {
    obj[key] = curAppState[key];
  });
  return obj;
}
