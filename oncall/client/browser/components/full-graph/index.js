// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/**
 * A graph showing multiple metrics in different colors along with their
 * summary items showing their current/pass values and line colors. User can
 * "select" one or more metrics by clicking on their summary items. The graph
 * then will only show selected metrics (if the selection is not empty).
 *
 * The graph will also overlay a graph of the metric stored in the
 * "hoveredMetric" observable passed from some other component.
 */

var hg = require('mercury');
var h = require('mercury').h;
var svg = require('virtual-dom/virtual-hyperscript/svg');
var dateformat = require('dateformat');

var Consts = require('../../constants');
var MouseMoveEvent = require('../../mousemove-handler');
var Util = require('../../util.js');

module.exports = create;
module.exports.render = render;

var colors = [
  '#16a085',
  '#2980b9',
  '#8e44ad',
  '#808B96',
  '#f39c12',
  '#d35400',
  '#27ae60',
  '#c0392b'
];

/** Constructor. */
function create(data) {
  // Transform the raw data to metrics array.
  // Also calculate time and value range across all metrics.
  var minTime = Number.MAX_VALUE;
  var maxTime = 0;
  var minValue = Number.MAX_VALUE;
  var maxValue = 0;
  var metrics = data.metrics.map(function(metric, index) {
    minTime = Math.min(minTime, metric.MinTime);
    maxTime = Math.max(maxTime, metric.MaxTime);
    minValue = Math.min(minValue, metric.MinValue);
    maxValue = Math.max(maxValue, metric.MaxValue);
    return {
      label: metric.Name,
      value: metric.CurrentValue,
      color: colors[index % colors.length],
      threshold: metric.Threshold,
      healthy: metric.Healthy,
      historyTimestamps: metric.HistoryTimestamps,
      historyValues: metric.HistoryValues,
    };
  });

  // Calculate the overall health for all metrics.
  var healthy = true;
  metrics.forEach(function(metric) {
    healthy &= metric.healthy;
  });

  var state = hg.state({
    // The timestamp when the data is loaded from the backend server.
    collectionTimestamp: data.collectionTimestamp,

    // The graph title.
    title: data.title,

    // Time range.
    minTime: minTime,
    maxTime: maxTime,

    // Value range.
    minValue: minValue,
    maxValue: maxValue,

    // Metrics shown in this graph.
    metrics: metrics,

    // Overall health.
    healthy: healthy,

    // Keep track of selected metrics.
    // A metric line will be visible if it is selected.
    selectedMetrics: hg.varhash({}),

    // See comments in instance-view/index.js.
    mouseOffsetFactor: data.mouseOffsetFactor,
    hoveredMetric: data.hoveredMetric,

    channels: {
      mouseMove: mouseMove,
      mouseOut: mouseOut,
      mouseClickOnSummaryItem: mouseClickOnSummaryItem
    }
  });

  return state;
}

/** Callback when mouse is moving on the graph. */
function mouseMove(state, data) {
  if (state.mouseOffsetFactor() !== data.f) {
    state.mouseOffsetFactor.set(data.f);
  }
}

/** Callback when mouse moves out of the graph. */
function mouseOut(state) {
  state.mouseOffsetFactor.set(-1);
}

/** Callback when user clicks on a metric's summary item. */
function mouseClickOnSummaryItem(state, label) {
  if (!state.selectedMetrics.get(label)) {
    state.selectedMetrics.put(label, 1);
  } else {
    state.selectedMetrics.delete(label);
  }
}

/** The main render function. */
function render(state) {
  // Render graphs.
  var items = [
    renderContent(state),
    renderThreshold(state)
  ];

  // Render an overlay graph for the metric stored in state.hoveredMetric.
  if (Object.keys(state.hoveredMetric).length !== 0) {
    var metric = state.hoveredMetric;
    var numPoints = metric.historyTimestamps.length;
    var minTime = metric.historyTimestamps[0];
    var maxTime = metric.historyTimestamps[numPoints - 1];
    items.push(svg('svg', {
      'viewBox': '0 0 100 100',
      'preserveAspectRatio': 'none'
    }, [
      Util.renderMetric(state.hoveredMetric,
        minTime, maxTime, metric.minValue, metric.maxValue, 1, '#AAA', 1)
    ]));
  }

  // Render mouse line.
  var offset = 100 * state.mouseOffsetFactor;
  items.push(svg('svg', {
   'class': 'overlay',
   'viewBox': '0 0 100 100',
   'preserveAspectRatio': 'none'
  }, [
    svg('polyline', {
      'points': offset + ',0 ' + offset + ',100'
    })
  ]));

  var endDate = new Date(state.collectionTimestamp * 1000);
  var startDate = new Date((state.collectionTimestamp - 3600) * 1000);
  return h('div.full-graph-container', [
      h('div.full-graph-title', state.title),
      h('div.content-container', [
        // The summary row at the top of the graph.
        renderSummaryRow(state),
        // The main graph.
        h('div.full-graph', {
          'ev-mousemove': new MouseMoveEvent(state.channels.mouseMove),
          'ev-mouseout': hg.send(state.channels.mouseOut)
        }, items),
        // The time axis at the bottom.
        h('div.time', [
          h('div.time-label', dateformat(startDate, 'HH:MM')),
          h('div.time-label', dateformat(endDate, 'HH:MM'))
        ])
      ])
  ]);
}

/** Renders the main graph. */
function renderContent(state) {
  var items = state.metrics.map(function(metric) {
    var strokeOpacity = isMetricVisible(state.selectedMetrics, metric.label) ?
        1 : 0.1;
    var metricLine = Util.renderMetric(
        metric, state.minTime, state.maxTime,
        state.minValue, state.maxValue, 1.5, metric.color, strokeOpacity);
    return metricLine;
  });
  return svg('svg', {
    'class': 'content',
    'viewBox': '0 0 100 100',
    'preserveAspectRatio': 'none'
  }, items);
}

/** Renders the threshold line. */
function renderThreshold(state) {
  var minThreshold = Number.MAX_VALUE;
  state.metrics.forEach(function(metric) {
    minThreshold = Math.min(minThreshold, metric.threshold);
  });
  var thresholdOffset = Util.getOffsetForValue(
      minThreshold, state.minValue, state.maxValue);
  return svg('svg', {
    'class': 'threshold-line',
    'viewBox': '0 0 100 100',
    'preserveAspectRatio': 'none',
  }, [
    svg('path', {
      'd': 'M0 ' + thresholdOffset + ' L 100 ' + thresholdOffset,
      'stroke-dasharray': '2,2'
    })
  ]);
}

/** Renders the summary row. */
function renderSummaryRow(state) {
  var items = state.metrics.map(function(metric) {
    var curValue = Util.formatValue(
        Util.interpolateValue(metric.value, state.mouseOffsetFactor,
          metric.historyTimestamps, metric.historyValues));
    var metricSummaryClassNames = [];
    if (!isMetricVisible(state.selectedMetrics, metric.label)) {
      metricSummaryClassNames.push('hidden');
    }
    if (!metric.healthy) {
      metricSummaryClassNames.push('unhealthy');
    }
    var metricValueClassNames = [];
    if (state.mouseOffsetFactor >= 0) {
      metricValueClassNames.push('historyValue');
    }
    if (!metric.healthy) {
      metricValueClassNames.push('unhealthy');
    }
    return h('div.metric-summary-item', {
      className: metricSummaryClassNames.join(' '),
      'ev-click': hg.send(state.channels.mouseClickOnSummaryItem, metric.label)
    }, [
        h('div.metric-summary-title', Consts.getDisplayName(metric.label)),
        h('div.metric-summary-value', {
          className: metricValueClassNames.join(' ')
        }, curValue),
        h('div.metric-summary-color', {
          'style': {backgroundColor: metric.color}
        })
    ]);
  });
  return h('div.metric-summary-container', items);
}

/**
 * Checks whether the given metric is visible based on the metrics stored in
 * the selectedMetrics object.
 * @param {Object} selectedMetrics - An object indexed by metric names.
 * @param {string} metric - The name of the metric to check.
 * @return {boolean}
 */
function isMetricVisible(selectedMetrics, metric) {
  return Object.keys(selectedMetrics).length === 0 || selectedMetrics[metric];
}
