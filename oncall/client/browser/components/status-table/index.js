// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

var dateformat = require('dateformat');
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
  },
  // Syncbase allocator.
  {
    rowHeader: Consts.metricNames.MN_SB_ALLOCATOR,
    columns: [
      {
        dataKey: Consts.dataKeys.DK_SERVICE_LATENCY,
        label: 'LATENCY',
        metricName: Consts.metricNames.MN_SB_ALLOCATOR,
        threshold: 2000
      },
      {
        dataKey: Consts.dataKeys.DK_SERVICE_QPS,
        label: 'QPS',
        metricName: Consts.metricNames.MN_SB_ALLOCATOR
      },
      {
        dataKey: Consts.dataKeys.DK_SERVICE_METADATA,
        label: 'BUILD AGE (h)',
        metricName: Consts.metricNames.MN_SB_ALLOCATOR
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

    mouseOverDataKey: hg.value(''),
    mouseOverMetricName: hg.value(''),

    channels: {
      mouseMoveOnSparkline: mouseMoveOnSparkline,
      mouseOverSparkline: mouseOverSparkline,
      mouseOutOfSparkline: mouseOutOfSparkline,
    }
  });
}

/** Callback for moving mouse on sparkline. */
function mouseMoveOnSparkline(state, data) {
  state.mouseOffsetFactor.set(data.f);
}

/** Callback for moving mouse over a sparkline. */
function mouseOverSparkline(state, colData) {
  state.mouseOverDataKey.set(colData.dataKey);
  state.mouseOverMetricName.set(colData.metricName);
}

/** Callback for moving mouse out of sparkline. */
function mouseOutOfSparkline(state) {
  state.mouseOffsetFactor.set(-1);
  state.mouseOverDataKey.set('');
  state.mouseOverMetricName.set('');
}

/** The main render function. */
function render(globalState, state) {
  var data = state.data;

  var rows = tableRows.map(function(rowData, rowIndex) {
    var numWarnings = 0;
    var numReplicas = 0;
    var hasFatalErrors = false;
    var cols = rowData.columns.map(function(colData, colIndex) {
      // Create a column for each metric.
      //
      // Use the first column (usually latency) to determine number of replicas.
      var metricsData = data[colData.dataKey][colData.metricName];
      if (numReplicas === 0) {
        numReplicas = metricsData.length;
      }

      // Calculate average from all replicas.
      var avg = {};   // timestamps -> [values from replicas]
      var numErrors = 0;
      metricsData.forEach(function(metricData) {
        if (metricData && metricData.HistoryTimestamps) {
          for (var i = 0; i < metricData.HistoryTimestamps.length; i++) {
            var t = metricData.HistoryTimestamps[i];
            var v = metricData.HistoryValues[i];
            if (!avg[t]) {
              avg[t] = [];
            }
            avg[t].push(v);
          }
        }

        var hasError = false;

        // Handle error when getting time series.
        if (metricData.ErrMsg !== '') {
          hasError = true;
        }

        // Handle stale data.
        var tsLen = metricData.HistoryTimestamps.length;
        if (tsLen > 0) {
          var lastTimestamp = metricData.HistoryTimestamps[tsLen - 1];
          if (data.MaxTime - lastTimestamp >
              Consts.stableDataThresholdInSeconds) {
            hasError = true;
          }
        } else {
          hasError = true;
        }

        // Value over threshold.
        var overThreshold = (
            colData.threshold && metricData.CurrentValue >= colData.threshold);
        if (overThreshold) {
          hasError = true;
        }

        if (hasError) {
          numErrors++;
        }
      });
      var extraColMetricClass = '';
      if (numErrors > 0) {
        extraColMetricClass = '.warning';
        // Use the latency column to estimate how many replicas are in warning
        // state.
        if (colData.label === 'LATENCY') {
          numWarnings = numErrors;
        }
      }

      // Prepare data for average timeseries.
      var avgTimestamps = [];
      var avgValues = [];
      var avgMinValue = Number.MAX_VALUE;
      var avgMaxValue = 0;
      Object.keys(avg).sort().forEach(function(t) {
        avgTimestamps.push(parseInt(t));
        var values = avg[t];
        var sum = 0;
        values.forEach(function(v) {
          sum += v;
        });
        var avgValue = sum / values.length;
        avgValues.push(avgValue);
        avgMinValue = Math.min(avgMinValue, avgValue);
        avgMaxValue = Math.max(avgMaxValue, avgValue);
      });

      // Render sparkline for average values.
      //
      // 100 is the default logical width of any svg graphs.
      var points = '0,100 100,100';
      if (avgTimestamps.length > 0 && avgValues.length > 0) {
        points = Util.genPolylinePoints(
          avgTimestamps, avgValues,
          data.MinTime, data.MaxTime,
          avgMinValue, avgMaxValue);
      }
      var avgCurValue = 0;
      if (avgValues.length > 0) {
        avgCurValue = avgValues[avgValues.length - 1];
      }
      var curValue = Util.formatValue(avgCurValue);
      var extraCurValueClass = '';
      var mouseOffset = 100 * state.mouseOffsetFactor;
      if (mouseOffset >= 0) {
        curValue = Util.formatValue(Util.interpolateValue(
            avgCurValue, state.mouseOffsetFactor,
            avgTimestamps, avgValues));
        extraCurValueClass = '.history';
      }

      // Handle avg value over threshold.
      var overThreshold = (
          colData.threshold && avgCurValue >= colData.threshold);
      var thresholdValue = -100;
      if (overThreshold) {
        hasFatalErrors = true;
        extraColMetricClass = '.fatal';
        thresholdValue = (colData.threshold-avgMinValue)/
            (avgMaxValue - avgMinValue)*100.0;
      }

      var sparklineItems = [
        h('div.highlight-overlay'),
        Util.renderMouseLine(mouseOffset),
        h('div.sparkline', {
          'ev-mousemove': new MouseMoveHandler(
              state.channels.mouseMoveOnSparkline),
          'ev-mouseover': hg.send(state.channels.mouseOverSparkline, colData),
          'ev-mouseout': hg.send(state.channels.mouseOutOfSparkline),
        }, [
          renderThreshold(thresholdValue),
          Util.renderSparkline(points)
        ]),
        h('div.cur-value' + extraCurValueClass, [curValue])
      ];
      if (state.mouseOffsetFactor >= 0 &&
          state.mouseOverDataKey === colData.dataKey &&
          state.mouseOverMetricName === colData.metricName) {
        var curTimestamp =
            (data.MaxTime - data.MinTime) * state.mouseOffsetFactor +
            data.MinTime;
        sparklineItems.push(
            h('div.mouseover-time', dateformat(new Date(curTimestamp * 1000))));
      }
      var sparkline = h('div.col-metric' + extraColMetricClass, {
        'ev-click': hg.send(globalState.channels.mouseClickOnMetric, {
          serviceName: rowData.rowHeader,
          colData: colData
        })
      }, sparklineItems);

      var items = [h('div.col-header', colData.label), sparkline];
      return h('div.col', items);
    });
    var headerExtraClass = (numWarnings !== 0) ? '.warning' : '';
    if (hasFatalErrors) {
      headerExtraClass = '.fatal';
    }
    cols.unshift(h('div.row-header' + headerExtraClass, [
        h('div.header-label', Consts.getDisplayName(rowData.rowHeader)),
        h('div.header-numhealthy',
          ((numReplicas - numWarnings) + '/' + numReplicas))
    ]));
    return h('div.row', cols);
  });

  return h('div.status-table', rows);
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
