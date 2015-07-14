// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/**
 * A metric item is a compact graph in the instance view. It only shows the
 * title, current/pass value, and a sparkline for the metric's data points.
 */

var hg = require('mercury');
var h = require('mercury').h;
var svg = require('virtual-dom/virtual-hyperscript/svg');
var uuid = require('uuid');

var Consts = require('../../constants');
var MouseMoveEvent = require('../../mousemove-handler');
var Util = require('../../util');

module.exports = create;
module.exports.render = render;

function create(data) {
  var state = hg.state({
    // Graph title.
    label: data.metric.Name,

    // Current value.
    value: data.metric.CurrentValue,

    // Value range.
    minValue: data.metric.MinValue,
    maxValue: data.metric.MaxValue,

    // Data points
    historyTimestamps: data.metric.HistoryTimestamps,
    historyValues: data.metric.HistoryValues,

    // Threshold value.
    threshold: data.metric.Threshold,

    // Current health.
    healthy: data.metric.Healthy,

    // The id of a mask for masking the mouse line within the graph area.
    svgMaskId: uuid.v1(),

    // See comments in instance-view/index.js.
    mouseOffsetFactor: data.mouseOffsetFactor,
    hoveredMetric: data.hoveredMetric,

    channels: {
      mouseMove: mouseMove,
      mouseOut: mouseOut,
      mouseOver: mouseOver
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

/** Callback when mouse is out of the graph. */
function mouseOut(state) {
  state.mouseOffsetFactor.set(-1);
  state.hoveredMetric.set({});
}

/** Callback when mouse is over the graph. */
function mouseOver(state) {
  state.hoveredMetric.set({
    label: state.label,
    value: state.value,
    minValue: state.minValue,
    maxValue: state.maxValue,
    historyTimestamps: state.historyTimestamps,
    historyValues: state.historyValues
  });
}

/** The main render function. */
function render(state) {
  var curValue = Util.formatValue(Util.interpolateValue(
      state.value, state.mouseOffsetFactor,
      state.historyTimestamps, state.historyValues));
  var valueClassNames = [];
  if (state.mouseOffsetFactor >= 0) {
    valueClassNames.push('historyValue');
  }
  if (!state.healthy) {
    valueClassNames.push('unhealthy');
  }
  return h('div.metric-item', {
    className: state.healthy ? '' : 'unhealthy',
    'ev-mouseout': hg.send(state.channels.mouseOut),
    'ev-mouseover': hg.send(state.channels.mouseOver)
  }, [
      h('div.metric-item-title', Consts.getDisplayName(state.label)),
      h('div.metric-item-value', {
        className: valueClassNames.join(' ')
      }, curValue),
      renderGraph(state)
  ]);
}

/** Renders the main graph. */
function renderGraph(state) {
  var mouseOffset = 100 * state.mouseOffsetFactor;
  var minTime = state.historyTimestamps[0];
  var maxTime = state.historyTimestamps[state.historyTimestamps.length - 1];
  var points = Util.genPolylinePoints(
      state.historyTimestamps, state.historyValues,
      minTime, maxTime, state.minValue, state.maxValue);

  var items = [
    renderSparkline(points),
    renderMouseLine(points, mouseOffset, state)
  ];
  if (state.threshold >= 0) {
    items.push(renderThresholdLine(state));
  }

  return h('div.sparkline', {
    'ev-mousemove': new MouseMoveEvent(state.channels.mouseMove),
  }, items);
}

/** Renders the sparkline for the metric's data points. */
function renderSparkline(points) {
  return svg('svg', {
    'class': 'content',
    'viewBox': '0 0 100 100',
    'preserveAspectRatio': 'none'
  }, [
    svg('polyline', {'points': points}),
    svg('polygon', {'points': '0,100 ' + points + ' 100,100 0,100'})
  ]);
}

/**
 * Renders the mouse line at the given offset.
 *
 * For better appearance, we mask the mouse line within the graph area.
 */
function renderMouseLine(points, mouseOffset, state) {
  var maskId = 'mask-' + state.svgMaskId;
  return svg('svg', {
    'class': 'mouse-line',
    'viewBox': '0 0 100 100',
    'preserveAspectRatio': 'none'
  }, [
    svg('defs', [
      svg('mask', {
        'id': maskId,
        'x': 0,
        'y': 0,
        'width': 100,
        'height': 100
      }, [
        svg('polygon', {
          'points': '0,100 ' + points + ' 100,100 0,100',
          'style': {'fill': '#ffffff'}
        })
      ])
    ]),
    svg('polyline', {
      'points': mouseOffset + ',0 ' + mouseOffset + ',100',
      'mask': 'url(#' + maskId + ')'
    })
  ]);
}

/** Renders the threshold line. */
function renderThresholdLine(state) {
  var thresholdOffset = Util.getOffsetForValue(
      state.threshold, state.minValue, state.maxValue);
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
