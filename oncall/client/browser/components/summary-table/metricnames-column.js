// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/**
 * Metric names column (the 1st column) of the summary table.
 */

var h = require('mercury').h;

var AppStateMgr = require('../../appstate-manager');
var Consts = require('../../constants');
var Util = require('../../util');

module.exports = {
  render: render
};

/** The main render function. */
function render(state) {
  var dataKey = state.hoveredCellData.dataKey;
  var metricKey = state.hoveredCellData.metricKey;
  var level = AppStateMgr.getAppState('level');

  // In the "globel" level, we show all metrics.
  // In the "zone" level, we only show either cloud services metrics or nginx
  // metrics.
  var metricNameRows = [];
  var metrics = Consts.mainMetrics;
  if (level === 'zone') {
    var zoneLevelType = AppStateMgr.getAppState('zoneLevelType');
    metrics = (zoneLevelType === 'CloudService' ?
        Consts.cloudServiceMetrics.concat(Consts.cloudServiceGCEMetrics) :
        Consts.nginxMetrics.concat(Consts.nginxGCEMetrics));
  }

  metrics.forEach(function(curMetric, index) {
    var className = 'metric-name-cell.cell';
    if (curMetric.dataKey === dataKey && curMetric.metricKey === metricKey) {
      className += '.highlight';
    }
    if (!checkHealthForMetric(state, curMetric)) {
      className += '.unhealthy';
    }
    // secontionLabel is the light grey label above each section of the table.
    // For example: NGINX SERVER LOAD, NGINX GCE, etc.
    if (curMetric.sectionLabel) {
      var sectionLabelClassName = '.section-label';
      if (curMetric.dataKey === dataKey) {
        sectionLabelClassName += '.highlight-section';
      }
      metricNameRows.push(h('div.' + className, [
        h('div' + sectionLabelClassName , h('span', curMetric.sectionLabel)),
        h('div', h('span', curMetric.label))
      ]));
    } else {
      metricNameRows.push(h('div.' + className, h('span', curMetric.label)));
    }
    if (curMetric.addMinorDivider) {
      metricNameRows.push(h('div.minor-divider'));
    }
    if (curMetric.addMajorDivider) {
      metricNameRows.push(h('div.major-divider'));
    }
  });
  return h('div.metric-names-col', metricNameRows);
}


/**
 * Checks whether the given metric across all the zones/instances is healthy.
 * @param {hg.state} state
 * @param {Object} metric
 * @return {boolean}
 */
function checkHealthForMetric(state, metric) {
  var data = state.data.Zones;
  var aggType = AppStateMgr.getAppState('globalLevelAggType');
  var level = AppStateMgr.getAppState('level');
  var isHealthy = true;

  var i;
  if (level === 'global') {
    var availableZones = Consts.orderedZones.filter(function(zone) {
      return data[zone];
    });
    for (i = 0; i < availableZones.length; i++) {
      var zone = availableZones[i];
      var aggData = data[zone][aggType];
      if (!Util.isEmptyObj(aggData[metric.dataKey])) {
        var metricAggData = aggData[metric.dataKey][metric.metricKey];
        if (!metricAggData || !metricAggData.Healthy) {
          isHealthy = false;
          break;
        }
      }
    }
  } else if (level === 'zone') {
    var zoneLevelZone = AppStateMgr.getAppState('zoneLevelZone');
    var instances = Object.keys(data[zoneLevelZone].Instances).sort();
    for (i = 0; i < instances.length; i++) {
      var instance = instances[i];
      var instanceData = data[zoneLevelZone].Instances[instance];
      if (!Util.isEmptyObj(instanceData[metric.dataKey])) {
        var metricData = instanceData[metric.dataKey][metric.metricKey];
        if (!metricData || !metricData.Healthy) {
          isHealthy = false;
          break;
        }
      }
    }
  }

  return isHealthy;
}
