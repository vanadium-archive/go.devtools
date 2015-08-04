// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/**
 * The summary table showing the data in "global" and "zone" level.
 *
 * In the "global" level, the table's columnes are zones with available data,
 * and its rows are metrics for vanadium services and nginx servers. Each table
 * cell (for zone Z and metric M) shows data aggregated across all the instances
 * in that zone for that metric.The aggregation method is either "average" or
 * "max", and can be selected at the top left corner of the summary table.
 *
 * Users can "drill down" to the "zone" level by clicking on any table cell.
 *
 * In the "zone" level, the table's columns are instances in that zone, and its
 * rows are metrics for EITHER vanadium services or nginx servers (depending on
 * which cell users click in the "global" level). In this level, users can drill
 * further down to the "instance" level by clicking on any table cell. The
 * instance view will be rendered by the "instance-view" component.
 */

var hg = require('mercury');
var h = require('mercury').h;

var AppStateMgr = require('../../appstate-manager');
var summaryTableHeaderComponent = require('./header');
var summaryTableColumnsComponent = require('./columns');

module.exports = create;
module.exports.render = render;

/** Constructor. */
function create(data) {
  if (!data) {
    return null;
  }

  return hg.state({
    // Raw data.
    data: hg.struct(data.data),

    // Keeps track of which cell the mouse cursor is hovering over.
    //
    // This will be used to highlight the hovered table cell as well as the
    // corresponding column/row header cell.
    hoveredCellData: hg.struct({
      // Column name.
      column: '',
      // See "createMetric" function in constants.js.
      dataKey: '',
      // See "createMetric" function in constants.js.
      metricKey: ''
    }),

    // Keeps track of the cell and relative location of the mouse cursor.
    //
    // This will be used to render a mouse line in all related table cells for
    // easier data comparison.
    mouseMoveCellData: hg.struct({
      // Column name.
      column: '',
      // See "createMetric" function in constants.js.
      dataKey: '',
      // See "createMetric" function in constants.js.
      offsetFactor: -1
    }),

    channels: {
      changeGlobalLevelAggType: changeGlobalLevelAggType,
      clickCell: clickCell,
      mouseMoveOnSparkline: mouseMoveOnSparkline,
      mouseOutOfSparkline: mouseOutOfSparkline,
      mouseOverTableCell: mouseOverTableCell,
      mouseOutOfTableCell: mouseOutOfTableCell
    }
  });
}

/**
 * Callback for changing the aggregation type in the "global" level.
 */
function changeGlobalLevelAggType(state, newAggType) {
  AppStateMgr.setAppState({'globalLevelAggType': newAggType});
}

/**
 * Callback for clicking a table cell.
 */
function clickCell(state, data) {
  var level = AppStateMgr.getAppState('level');
  if (level === 'global') {
    // Clicking a cell in the "global" level will go to the corresponding "zone"
    // level.
    var zoneLevelType =
        data.dataKey.startsWith('CloudService') ? 'CloudService' : 'Nginx';
    AppStateMgr.setAppState({
      'level': 'zone',
      'zoneLevelZone': data.column,
      'zoneLevelType': zoneLevelType
    });
  } else if (level === 'zone') {
    // Clicking a cell in the "zone" level will go to the corresponding
    // "instance" level.
    AppStateMgr.setAppState({
      'level': 'instance',
      'instanceLevelInstance': data.column,
      'instanceLevelZone': data.zone,
    });
  }
}

/** Callback for moving mouse on sparkline. */
function mouseMoveOnSparkline(state, data) {
  state.mouseMoveCellData.set({
    column: data.column,
    dataKey: data.dataKey,
    offsetFactor: data.f
  });
}

/** Callback for moving mouse out of sparkline. */
function mouseOutOfSparkline(state) {
  state.mouseMoveCellData.set({
    column: '',
    dataKey: '',
    offsetFactor: -1
  });
}

/** Callback for moving mouse over a table cell. */
function mouseOverTableCell(state, data) {
  state.hoveredCellData.set({
    column: data.column,
    dataKey: data.dataKey,
    metricKey: data.metricKey
  });
}

/** Callback for moving mouse out of a table cell. */
function mouseOutOfTableCell(state) {
  state.hoveredCellData.set({
    column: '',
    dataKey: '',
    metricKey: ''
  });
}

/** The main render function. */
function render(state) {
  // Delegate rendering to sub-components.
  return h('div.summary-table', [
    hg.partial(summaryTableHeaderComponent.render, state),
    hg.partial(summaryTableColumnsComponent.render, state)
  ]);
}
