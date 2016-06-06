// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/**
 * A set of utility functions.
 */

var svg = require('virtual-dom/virtual-hyperscript/svg');

module.exports = {
  interpolateValue: interpolateValue,
  renderMetric: renderMetric,
  genPolylinePoints: genPolylinePoints,
  getOffsetForValue: getOffsetForValue,
  formatValue: formatValue,
  isEmptyObj: isEmptyObj,
  renderSparkline: renderSparkline,
  renderMouseLine: renderMouseLine
};

/**
 * The default logical size of svg graphs.
 *
 * In this project, we always render svg graphs to a logical 100*100 area with
 * "non-scaling-stroke" property. Then the graphs will be scaled in DOM to their
 * final display sizes.
 *
 * @const
 */
var SVG_LOGICAL_SIZE = 100;

/**
 * Used to control how much padding to add to line graphs.
 * @const
 */
var PADDING_FACTOR = 10;

/**
 * Interpolates value Y for the given X in the given time series data.
 * @param {number} curValue - The current (latest) value in the time series.
 * @param {number} xFactor - The fraction (0-1) of X between minX and maxX to
 *     interpolate value for.
 * @param {Array<number>} xs - The X coordinates of the time series.
 * @param {Array<number>} ys - The Y coordinates of the time series.
 * @return {number}
 */
function interpolateValue(curValue, xFactor, xs, ys) {
  if (xFactor < 0 || xFactor > 1) {
    return curValue;
  }
  var numPoints = xs.length;
  var minX = xs[0];
  var maxX = xs[numPoints-1];
  var x = (maxX - minX) * xFactor + minX;
  var xIndex0 = 0;
  for (var i = 0; i < numPoints - 1; i++) {
    var curX = xs[i];
    var nextX = xs[i+1];
    if (x >= curX && x <= nextX) {
      xIndex0 = i;
      break;
    }
  }
  var xIndex1 = xIndex0 + 1;
  var x0 = xs[xIndex0];
  var x1 = xs[xIndex1];
  var y0 = ys[xIndex0];
  var y1 = ys[xIndex1];
  var f = (x - x0) / (x1 - x0);
  return (y1 - y0) * f + y0;
}

/**
 * Renders a line graph for the time series data in the given metric using the
 * given properties.
 * @param {Object} metric - The metric to render.
 * @param {number} minTime - The minimum timestamp of the line graph.
 * @param {number} maxTime - The maximum timestamp of the line graph.
 * @param {number} minValue - The minimum value of the line graph.
 * @param {number} maxValue - The maximum value of the line graph.
 * @param {number} strokeWidth - The width of the line stroke.
 * @param {string} strokeColor - The color of the stroke.
 * @param {number} strokeOpacity - The opacity of the stroke.
 * @param {VirtualNode}
 */
function renderMetric(metric, minTime, maxTime, minValue, maxValue,
    strokeWidth, strokeColor, strokeOpacity) {
  var points = genPolylinePoints(metric.historyTimestamps, metric.historyValues,
      minTime, maxTime, minValue, maxValue);
  return svg('polyline', {
    'points': points,
    'style': {
      'stroke-width': strokeWidth,
      'stroke': strokeColor,
      'stroke-opacity': strokeOpacity
    }
  });
}

/**
 * Generates polyline points for the given time series.
 * @param {Array<number} timestamps - The timestamps of the time series.
 * @param {Array<number>} values - The values of the time series.
 * @param {number} minTime - The minimum timestamp of the polyline.
 * @param {number} maxTime - The maximum timestamp of the polyline.
 * @param {number} minValue - The minimum value of the polyline.
 * @param {number} maxValue - The maximum value of the polyline.
 * @return {string} Polyline points in the form of 'x1,y1 x2,y2 ...'.
 */
function genPolylinePoints(timestamps, values, minTime, maxTime,
    minValue, maxValue) {
  var valuePadding = getValuePadding(minValue, maxValue);
  maxValue += valuePadding;
  minValue -= valuePadding;

  var points = [];
  var numPoints = timestamps.length;
  for (var i = 0; i < numPoints; i++) {
    var t = timestamps[i];
    t = (t - minTime) / (maxTime - minTime) * SVG_LOGICAL_SIZE;
    var v = values[i];
    v = SVG_LOGICAL_SIZE -
        (v - minValue) / (maxValue - minValue) * SVG_LOGICAL_SIZE;
    points.push(t + ',' + v);
  }
  return points.join(' ');
}

/**
 * Calculates the padding for the given value range. This is used to add
 * paddings to line graphs so that the polylines are not too closed to the graph
 * edges.
 * @param {number} minValue - The minimum value of the value range.
 * @param {number} maxValue - The maximum value of the value range.
 * @return {number}
 */
function getValuePadding(minValue, maxValue) {
  var valuePadding = (maxValue - minValue) / PADDING_FACTOR;
  if (valuePadding === 0) {
    valuePadding = maxValue / PADDING_FACTOR;
  }
  return valuePadding;
}

/**
 * Calculates logical Y offset for the given value in the given value range.
 * @param {number} value - The value to calculate offset for.
 * @param {number} minValue - The minimum value of the value range.
 * @param {number} maxValue - The maximum value of the value range.
 * @return {number}
 */
function getOffsetForValue(value, minValue, maxValue) {
  if (value < 0) {
    return -1;
  }
  var valuePadding = getValuePadding(minValue, maxValue);
  maxValue += valuePadding;
  minValue -= valuePadding;
  return SVG_LOGICAL_SIZE -
      (value - minValue) / (maxValue - minValue) * SVG_LOGICAL_SIZE;
}

/**
 * Formats the given value for display purpose.
 * @param {number} value - The value to format.
 * @return {string}
 */
function formatValue(value) {
  if (isNaN(value)) {
    return '?';
  }
  if (value < 1) {
    value = value.toFixed(2);
  } else if (value < 10) {
    value = value.toFixed(1);
  } else {
    value = Math.floor(value);
  }
  if (value < 0.0001) {
    value = 0;
  }
  return value.toString();
}

/**
 * Checks whether the given object is empty.
 * @param {Object} obj - The object to check.
 * @return {boolean}
 */
function isEmptyObj(obj) {
  return Object.keys(obj).length === 0;
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
