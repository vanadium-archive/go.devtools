// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/**
 * The header row of the summary table.
 *
 * In the "global" view, it shows the aggregation switch and all the zones. In
 * the "zone" view, it shows all the instances for that zone.
 */

var hg = require('mercury');
var h = require('mercury').h;

var Consts = require('../../constants');
var AppStateMgr = require('../../appstate-manager');
var Util = require('../../util');

module.exports = {
  render: render
};

/** The main render function. */
function render(state) {
  var data = state.data.Zones;
  var level = AppStateMgr.getAppState('level');

  // First column is the aggregation switch.
  var cols = [renderGlobalLevelAggSwitch(state)];

  // Generate the rest of the columns.
  var headers = [];
  if (level === 'global') {
    // In the "global" level, add all the zones.
    headers = Consts.orderedZones.filter(function(zone) {
      return data[zone];
    });
  } else if (level === 'zone') {
    // In the "zone" level, add instances based on the type.
    var zoneLevelZone = AppStateMgr.getAppState('zoneLevelZone');
    var zoneLevelType = AppStateMgr.getAppState('zoneLevelType');
    var instancePrefix =
        (zoneLevelType === 'CloudService' ? 'vanadium-' : 'nginx-');
    // Filter instances by instancePrefix (determined by zoneLevelType).
    headers = Object.keys(data[zoneLevelZone].Instances)
        .sort().filter(function(name) {
      return name.startsWith(instancePrefix);
    });
  }
  cols = cols.concat(renderHeaders(state, headers));

  return h('div.zone-names-row', cols);
}

/** Renders the aggregtion type switch. */
function renderGlobalLevelAggSwitch(state) {
  var aggType = AppStateMgr.getAppState('globalLevelAggType');
  var level = AppStateMgr.getAppState('level');
  if (level === 'global') {
    // Only generate the switch (two links: average and max) in the "global"
    // level to specify the aggregation method for all the cell data.
    return h('div.view-type.zone-name-cell', [
      h('div.average', {
        className: aggType  === 'Average' ? 'selected' : '',
        'ev-click': hg.send(state.channels.changeGlobalLevelAggType, 'Average')
      }, 'average'),
      h('div.max', {
        className: aggType === 'Max' ? 'selected' : '',
        'ev-click': hg.send(state.channels.changeGlobalLevelAggType, 'Max')
      }, 'max')
    ]);
  } else {
    // In the "zone" level, don't show the switch.
    return h('div.view-type-dummy');
  }
}

/**
 * Renders the given headers.
 * @param {hg.state} state
 * @param {Array<string>} headers
 */
function renderHeaders(state, headers) {
  var hoveredColumn = state.hoveredCellData.column;
  return headers.map(function(header) {
    var className = 'zone-name-cell';
    if (header === hoveredColumn) {
      className += '.highlight';
    }
    if (!checkMetricsHealthForHeader(state, header)) {
      className += '.unhealthy';
    }
    return h('div.' + className, h('span', shortenHeaderLabel(header)));
  });
}

/**
 * Checks whether all the metrics for the given header are healthy.
 * @param {hg.state} state
 * @param {string} header
 * @return {boolean}
 */
function checkMetricsHealthForHeader(state, header) {
  var data = state.data.Zones;
  var aggType = AppStateMgr.getAppState('globalLevelAggType');
  var level = AppStateMgr.getAppState('level');
  var isHealthy = true;

  // In different levels, we check different health data for a given header.
  // The health data is already calculated and recorded in the data object
  // loaded from the backend server.
  var i, curMetric;
  if (level === 'global') {
    var zone = header;
    var aggData = data[zone][aggType];
    for (i = 0; i < Consts.metrics.length; i++) {
      curMetric = Consts.metrics[i];
      if (!Util.isEmptyObj(aggData[curMetric.dataKey])) {
        if (!aggData[curMetric.dataKey][curMetric.metricKey].Healthy) {
          isHealthy = false;
          break;
        }
      }
    }
  } else if (level === 'zone') {
    var instance = header;
    var zoneLevelZone = AppStateMgr.getAppState('zoneLevelZone');
    var zoneLevelType = AppStateMgr.getAppState('zoneLevelType');
    var metrics = (zoneLevelType === 'CloudService' ?
        Consts.cloudServiceMetrics : Consts.nginxMetrics);
    var instanceData = data[zoneLevelZone].Instances[instance];
    for (i = 0; i < metrics.length; i++) {
      curMetric = metrics[i];
      if (!Util.isEmptyObj(instanceData[curMetric.dataKey])) {
        if (!instanceData[curMetric.dataKey][curMetric.metricKey].Healthy) {
          isHealthy = false;
          break;
        }
      }
    }
  }

  return isHealthy;
}

/**
 * Shortens the header label by removing the common prefix.
 * @param {string} header
 * @return {string} The shortened header label.
 */
function shortenHeaderLabel(header) {
  if (header.startsWith('vanadium')) {
    return header.replace('vanadium-', '');
  }
  if (header.startsWith('nginx')) {
    return header.replace('nginx-', '');
  }
  return header;
}
