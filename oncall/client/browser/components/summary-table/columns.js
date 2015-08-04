// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/**
 * Columns of the summary table.
 */

var hg = require('mercury');
var h = require('mercury').h;
var svg = require('virtual-dom/virtual-hyperscript/svg');

var AppStateMgr = require('../../appstate-manager');
var Consts = require('../../constants');
var MouseMoveHandler = require('../../mousemove-handler.js');
var Util = require('../../util');
var summaryTableMetricNamesColumnComponent = require('./metricnames-column');

module.exports = {
  render: render
};

/** The main render function. */
function render(state) {
  var data = state.data.Zones;
  var level = AppStateMgr.getAppState('level');

  var cols = [hg.partial(summaryTableMetricNamesColumnComponent.render, state)];
  if (level === 'global') {
    cols = cols.concat(genCellsInGlobalLevel(state, data));
  } else if (level === 'zone') {
    cols = cols.concat(genCellsInZoneLevel(state, data));
  }
  return h('div.columns-container', cols);
}

/** Generates data cells in the "globel" level. */
function genCellsInGlobalLevel(state, data) {
  var aggType = AppStateMgr.getAppState('globalLevelAggType');
  var cols = Consts.orderedZones.filter(function(zone) {
    return data[zone];
  }).map(function(zone) {
    var aggData = data[zone][aggType];
    var rows = [];
    Consts.mainMetrics.forEach(function(curMetric, index) {
      var dataKey = curMetric.dataKey;
      var metricKey = curMetric.metricKey;
      if (!Util.isEmptyObj(aggData[dataKey])) {
        rows.push(renderCell(
            state, aggData, aggData.Range.MinTime, aggData.Range.MaxTime,
            dataKey, metricKey, zone, zone));
      } else {
        rows.push(renderFillerCell(state));
      }
      if (curMetric.addMinorDivider) {
        rows.push(h('div.minor-divider'));
      }
      if (curMetric.addMajorDivider) {
        rows.push(h('div.major-divider'));
      }
    });
    return h('div.zone-col', rows);
  });
  return cols;
}

/** Generates data cells in the "zone" level. */
function genCellsInZoneLevel(state, data) {
  var zoneLevelZone = AppStateMgr.getAppState('zoneLevelZone');
  var zoneLevelType = AppStateMgr.getAppState('zoneLevelType');

  // Filter metrics and instances by zone level type (CloudService or Nginx).
  var metrics = (zoneLevelType === 'CloudService' ?
      Consts.cloudServiceMetrics.concat(Consts.cloudServiceGCEMetrics) :
      Consts.nginxMetrics.concat(Consts.nginxGCEMetrics));
  var instancePrefix = (zoneLevelType === 'CloudService' ?
      'vanadium-' : 'nginx-');
  var instanceNames = Object.keys(data[zoneLevelZone].Instances);
  var cols = instanceNames.sort().filter(function(name) {
    return name.startsWith(instancePrefix);
  }).map(function(instance) {
    var instanceData = data[zoneLevelZone].Instances[instance];
    var rows = [];
    // Calculate the min/max timestamps across all metrics.
    var minTime = Number.MAX_VALUE;
    var maxTime = 0;
    metrics.forEach(function(curMetric) {
      var dataKey = curMetric.dataKey;
      var metricKey = curMetric.metricKey;
      if (!Util.isEmptyObj(instanceData[dataKey])) {
        var metricData = instanceData[dataKey][metricKey];
        if (metricData.MinTime < minTime) {
          minTime = metricData.MinTime;
        }
        if (metricData.MaxTime > maxTime) {
          maxTime = metricData.MaxTime;
        }
      }
    });
    metrics.forEach(function(curMetric, index) {
      var dataKey = curMetric.dataKey;
      var metricKey = curMetric.metricKey;
      if (!Util.isEmptyObj(instanceData[dataKey])) {
        rows.push(renderCell(
            state, instanceData, minTime, maxTime,
            dataKey, metricKey, instance, zoneLevelZone));
      } else {
        rows.push(renderFillerCell(state));
      }
      if (curMetric.addMinorDivider) {
        rows.push(h('div.minor-divider'));
      }
      if (curMetric.addMajorDivider) {
        rows.push(h('div.major-divider'));
      }
    });
    return h('div.zone-col', rows);
  });
  return cols;
}

/**
 * Renders a single table cell.
 * @param {hg.state} state - The component's state.
 * @param {Object} cellData - The data object for the cell.
 * @param {Number} minTime - The minimum time to render the sparkline for.
 * @param {Number} maxTime - The maximum time to render the sparkline for.
 * @param {string} dataKey - See comments of createMetric function in
 *     constants.js.
 * @param {string} metricKey - See comments of createMetric function in
 *     constants.js.
 * @param {string} column - The column name of the cell.
 * @param {string} zone - The zone name associated with the cell.
 */
function renderCell(state, cellData, minTime, maxTime,
    dataKey, metricKey, column, zone) {
  var data = cellData[dataKey][metricKey];
  // 100 is the default logical width of any svg graphs.
  var mouseOffset = 100 * state.mouseMoveCellData.offsetFactor;
  var mouseEventData = {column: column, dataKey: dataKey, metricKey: metricKey};
  var points = Util.genPolylinePoints(
      data.HistoryTimestamps, data.HistoryValues,
      minTime, maxTime, data.MinValue, data.MaxValue);

  // Show mouse line based on the dataKey type (CloudService or Nginx).
  var mouseMoveOnCloudServices =
      state.mouseMoveCellData.dataKey.startsWith('CloudService');
  var mouseMoveOnNginx = state.mouseMoveCellData.dataKey.startsWith('Nginx');
  var showMouseLine = (column === state.mouseMoveCellData.column &&
      ((dataKey.startsWith('CloudService') && mouseMoveOnCloudServices) ||
       (dataKey.startsWith('Nginx') && mouseMoveOnNginx)));

  var curValue = data.CurrentValue;
  var curValueClass = 'value';
  if (showMouseLine) {
    curValue = Util.interpolateValue(
        data.CurrentValue, state.mouseMoveCellData.offsetFactor,
        data.HistoryTimestamps, data.HistoryValues);
    curValueClass += '.history';
  }
  var items = [
    h('div.highlight-overlay'),
    h('div.mouse-line', showMouseLine ? renderMouseLine(mouseOffset) : []),
    h('div.sparkline', {
      'ev-mousemove': new MouseMoveHandler(
          state.channels.mouseMoveOnSparkline, mouseEventData),
      'ev-mouseout': hg.send(state.channels.mouseOutOfSparkline)
    }, renderSparkline(points)),
    h('div.' + curValueClass, Util.formatValue(curValue))
  ];

  var cellClassName =
      'zone-col-cell.cell.' + (data.Healthy ? 'healthy' : 'unhealthy');
  return h('div.' + cellClassName, {
    'ev-mouseout': hg.send(state.channels.mouseOutOfTableCell),
    'ev-mouseover': hg.send(state.channels.mouseOverTableCell, mouseEventData),
    'ev-click': hg.send(state.channels.clickCell, {
      column: column,
      dataKey: dataKey,
      zone: zone
    })
  }, items);
}

/** Renders a empty filler table cell. */
function renderFillerCell(state) {
  return h('div.zone-col-cell-filler.cell', {
    'ev-mouseout': hg.send(state.channels.mouseOutOfTableCell)
  });
}

/**
 * Renders sparkline for the given points.
 * @param {string} points - A string in the form of "x1,y1 x2,y2 ...".
 */
function renderSparkline(points) {
  return svg('svg', {
    'class': 'content',
    'viewBox': '0 0 100 100',
    'preserveAspectRatio': 'none'
  }, [
    svg('polyline', {'points': points}),
  ]);
}

/**
 * Renders mouse line at the given offset.
 * @param {Number} mouseOffset - The logical offset for the mouse line.
 */
function renderMouseLine(mouseOffset) {
  return svg('svg', {
    'class': 'mouse-line',
    'viewBox': '0 0 100 100',
    'preserveAspectRatio': 'none'
  }, [
    svg('polyline', {
      'points': mouseOffset + ',0 ' + mouseOffset + ',100'
    })
  ]);
}
