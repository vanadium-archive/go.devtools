// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

var hg = require('mercury');
var h = require('mercury').h;
var svg = require('virtual-dom/virtual-hyperscript/svg');

var Consts = require('../../constants');
var MouseMoveHandler = require('../../mousemove-handler.js');
var Util = require('../../util');

module.exports = create;
module.exports.render = render;

var tableRows = [
  // Mounttable.
  {
    rowHeader: Consts.metricNames.MN_MOUNTTABLE,
    columns: [
      {
        dataKey: Consts.dataKeys.DK_SERVICE_LATENCY,
        label: 'LATENCY',
        metricName: Consts.metricNames.MN_MOUNTTABLE,
        threshold: 2000
      },
      {
        dataKey: Consts.dataKeys.DK_SERVICE_QPS,
        label: 'QPS',
        metricName: Consts.metricNames.MN_MOUNTTABLE
      },
      {
        dataKey: Consts.dataKeys.DK_SERVICE_METADATA,
        label: 'BUILD AGE (h)',
        metricName: Consts.metricNames.MN_MOUNTTABLE
      },
      {
        dataKey: Consts.dataKeys.DK_SERVICE_COUNTERS,
        label: 'MOUNTED SERVERS',
        metricName: Consts.metricNames.MN_MT_MOUNTED_SERVERS
      },
      {
        dataKey: Consts.dataKeys.DK_SERVICE_COUNTERS,
        label: 'NODES',
        metricName: Consts.metricNames.MN_MT_NODES
      },
    ]
  },
  // Roled.
  {
    rowHeader: Consts.metricNames.MN_ROLE,
    columns: [
      {
        dataKey: Consts.dataKeys.DK_SERVICE_LATENCY,
        label: 'LATENCY',
        metricName: Consts.metricNames.MN_ROLE,
        threshold: 2000
      },
      {
        dataKey: Consts.dataKeys.DK_SERVICE_QPS,
        label: 'QPS',
        metricName: Consts.metricNames.MN_ROLE
      },
      {
        dataKey: Consts.dataKeys.DK_SERVICE_METADATA,
        label: 'BUILD AGE (h)',
        metricName: Consts.metricNames.MN_ROLE
      }
    ]
  },
  // Proxy.
  {
    rowHeader: Consts.metricNames.MN_PROXY,
    columns: [
      {
        dataKey: Consts.dataKeys.DK_SERVICE_LATENCY,
        label: 'LATENCY',
        metricName: Consts.metricNames.MN_PROXY,
        threshold: 2000
      },
      {
        dataKey: Consts.dataKeys.DK_SERVICE_QPS,
        label: 'QPS',
        metricName: Consts.metricNames.MN_PROXY
      },
      {
        dataKey: Consts.dataKeys.DK_SERVICE_METADATA,
        label: 'BUILD AGE (h)',
        metricName: Consts.metricNames.MN_PROXY
      }
    ]
  },
  // Identityd.
  {
    rowHeader: Consts.metricNames.MN_IDENTITY,
    columns: [
      {
        dataKey: Consts.dataKeys.DK_SERVICE_LATENCY,
        label: 'LATENCY (MACAROON)',
        metricName: Consts.metricNames.MN_MACAROON,
        threshold: 2000
      },
      {
        dataKey: Consts.dataKeys.DK_SERVICE_LATENCY,
        label: 'LATENCY (BINARY DISCHARGER)',
        metricName: Consts.metricNames.MN_BINARY_DISCHARGER,
        threshold: 2000
      },
      {
        dataKey: Consts.dataKeys.DK_SERVICE_QPS,
        label: 'QPS',
        metricName: Consts.metricNames.MN_IDENTITY
      },
      {
        dataKey: Consts.dataKeys.DK_SERVICE_METADATA,
        label: 'BUILD AGE (h)',
        metricName: Consts.metricNames.MN_IDENTITY
      }
    ]
  },
  // Benchmarks.
  {
    rowHeader: Consts.metricNames.MN_BENCHMARKS,
    columns: [
      {
        dataKey: Consts.dataKeys.DK_SERVICE_LATENCY,
        label: 'LATENCY',
        metricName: Consts.metricNames.MN_BENCHMARKS,
        threshold: 2000
      },
      {
        dataKey: Consts.dataKeys.DK_SERVICE_QPS,
        label: 'QPS',
        metricName: Consts.metricNames.MN_BENCHMARKS
      },
      {
        dataKey: Consts.dataKeys.DK_SERVICE_METADATA,
        label: 'BUILD AGE (h)',
        metricName: Consts.metricNames.MN_BENCHMARKS
      }
    ]
  }
];

/** Constructor. */
function create(data) {
  if (!data) {
    return null;
  }

  return hg.state({
    // Raw data.
    data: hg.struct(data.data),

    mouseOffsetFactor: hg.value(-1),
    showMetricActionsPanel: hg.value(false),

    channels: {
      mouseMoveOnSparkline: mouseMoveOnSparkline,
      mouseOutOfSparkline: mouseOutOfSparkline,
    }
  });
}

/** Callback for moving mouse on sparkline. */
function mouseMoveOnSparkline(state, data) {
  state.mouseOffsetFactor.set(data.f);
}

/** Callback for moving mouse out of sparkline. */
function mouseOutOfSparkline(state) {
  state.mouseOffsetFactor.set(-1);
}

/** The main render function. */
function render(globalState, state) {
  var data = state.data;

  var rows = tableRows.map(function(rowData) {
    var cols = rowData.columns.map(function(colData) {
      // Create a column for a metric.
      var colHeader = h('div.col-header', colData.label);
      var metricsData = data[colData.dataKey][colData.metricName];
      var sparkLines = metricsData.map(function(metricData) {
        // 100 is the default logical width of any svg graphs.
        var points = '0,100 100,100';
        if (metricData && metricData.HistoryTimestamps) {
          points = Util.genPolylinePoints(
            metricData.HistoryTimestamps, metricData.HistoryValues,
            data.MinTime, data.MaxTime,
            metricData.MinValue, metricData.MaxValue);
        }
        var curValue = Util.formatValue(metricData.CurrentValue);
        // Handle error when getting time series.
        var hasErrors = metricData.ErrMsg !== '';
        var extraColMetricClass = '';
        if (hasErrors) {
          curValue = '?';
          extraColMetricClass = '.err';
        }
        // Handle current value over threshold.
        var overThreshold = (
            colData.threshold && metricData.CurrentValue >= colData.threshold);
        var thresholdValue = -1;
        if (overThreshold) {
          extraColMetricClass = '.unhealthy';
          thresholdValue = (colData.threshold-metricData.MinValue)/
              (metricData.MaxValue-metricData.MinValue)*100.0;
        }
        // Handle stale data.
        var tsLen = metricData.HistoryTimestamps.length;
        if (tsLen > 0) {
          var lastTimestamp = metricData.HistoryTimestamps[tsLen - 1];
          if (data.MaxTime - lastTimestamp > 600) {
            extraColMetricClass = '.stale';
          }
        } else {
          extraColMetricClass = '.unhealthy';
        }

        // Mouse line.
        var extraCurValueClass = '';
        var mouseOffset = 100 * state.mouseOffsetFactor;
        if (!hasErrors && mouseOffset >= 0) {
          curValue = Util.formatValue(Util.interpolateValue(
              metricData.CurrentValue, state.mouseOffsetFactor,
              metricData.HistoryTimestamps, metricData.HistoryValues));
          extraCurValueClass = '.history';
        }

        return h('div.col-metric' + extraColMetricClass, {
          'title': hasErrors ?
              metricData.ErrMsg : metricData.Instance + ', ' + metricData.Zone,
          'ev-click': hg.send(globalState.channels.mouseClickOnMetric, {
            metricData: metricData,
            serviceName: rowData.rowHeader
          })
        }, [
          h('div.highlight-overlay'),
          renderMouseLine(mouseOffset),
          h('div.sparkline', {
            'ev-mousemove': new MouseMoveHandler(
                state.channels.mouseMoveOnSparkline),
            'ev-mouseout': hg.send(state.channels.mouseOutOfSparkline)
          }, [
            renderThreshold(thresholdValue),
            renderSparkline(points)
          ]),
          h('div.cur-value' + extraCurValueClass, [curValue])
        ]);
      });
      var items = [colHeader];
      items = items.concat(sparkLines);
      return h('div.col', items);
    });
    cols.unshift(h('div.row-header', Consts.getDisplayName(rowData.rowHeader)));
    return h('div.row', cols);
  });

  return h('div.status-table', rows);
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
 * Renders threshold line.
 */
function renderThreshold(value) {
  return svg('svg', {
    'class': 'threshold',
    'viewBox': '0 0 100 100',
    'preserveAspectRatio': 'none'
  }, [
    svg('path', {
      'd': 'M 0 ' + value + ' L 100 ' + value,
      'stroke-dasharray': '2,2'
    }),
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
