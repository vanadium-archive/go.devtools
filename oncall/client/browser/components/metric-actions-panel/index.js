// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

var dateformat = require('dateformat');
var hg = require('mercury');
var h = require('mercury').h;

var Consts = require('../../constants');
var MouseMoveHandler = require('../../mousemove-handler.js');
var Util = require('../../util');

module.exports = create;
module.exports.render = render;

var VANADIUM_PRODUCTION_NAMESPACE_ID = '4b20e0dc-cacf-11e5-87ec-42010af0020b';
var AUTH_PRODUCTION_NAMESPACE_ID = '2b6d405a-b4a6-11e5-9776-42010af000a6';

/** Constructor. */
function create(data) {
  if (!data) {
    return null;
  }

  return hg.state({
    selectedMetric: hg.struct(data.selectedMetric),
    selectedMetricIndex: hg.value(data.selectedMetricIndex),
    visible: hg.value(data.visible),

    mouseOffsetFactor: hg.value(-1),

    channels: {
      mouseClickOnMetric: mouseClickOnMetric,
      closeMetricActionsPanel: closeMetricActionsPanel,
      mouseMoveOnSparkline: mouseMoveOnSparkline,
      mouseOutOfSparkline: mouseOutOfSparkline
    }
  });
}

/** Callback for moving mouse on sparkline. */
function mouseClickOnMetric(state, data) {
  state.selectedMetricIndex.set(data.index);
}

function closeMetricActionsPanel(state) {
  state.visible.set(false);
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
function render(state, curData) {
  var colData = state.selectedMetric.colData;
  var metricsData = curData[colData.dataKey][colData.metricName];

  var list = renderReplicaList(state, metricsData, curData);
  var panel = renderMetric(
      state, metricsData[state.selectedMetricIndex],
      state.selectedMetric.serviceName, curData);
  return h('div.metric-actions-container',
      h('div.inner-container', [list, panel])
  );
}

function renderReplicaList(state, metricsData, curData) {
  var colData = state.selectedMetric.colData;
  var list = metricsData.map(function(metricData, index) {
    var curIndex = index;

    var points = '0,100 100,100';
    var timestamps = metricData.HistoryTimestamps;
    var values = metricData.HistoryValues;
    if (timestamps.length > 0 && values.length > 0) {
      points = Util.genPolylinePoints(
        timestamps, values,
        curData.MinTime, curData.MaxTime,
        metricData.MinValue, metricData.MaxValue);
    }
    var curValue = Util.formatValue(metricData.CurrentValue);
    var extraCurValueClass = '';
    var mouseOffset = 100 * state.mouseOffsetFactor;
    if (mouseOffset >= 0) {
      curValue = Util.formatValue(Util.interpolateValue(
          metricData.CurrentValue, state.mouseOffsetFactor,
          timestamps, values));
      extraCurValueClass = '.history';
    }

    // Handle error when getting time series.
    var hasErrors = metricData.ErrMsg !== '';
    var extraColMetricClass = '';
    if (hasErrors) {
      curValue = '?';
      extraColMetricClass = '.warning';
    }
    // Handle current value over threshold.
    var overThreshold = (
        colData.threshold && metricData.CurrentValue >= colData.threshold);
    var thresholdValue = -100;
    if (overThreshold) {
      extraColMetricClass = '.fatal';
      thresholdValue = (colData.threshold-metricData.MinValue)/
          (metricData.MaxValue-metricData.MinValue)*100.0;
    }
    // Handle stale data.
    var tsLen = timestamps.length;
    if (tsLen > 0) {
      var lastTimestamp = timestamps[tsLen - 1];
      if (curData.MaxTime - lastTimestamp >
          Consts.stableDataThresholdInSeconds) {
        extraColMetricClass = '.warning';
      }
    } else {
      extraColMetricClass = '.warning';
    }

    if (index === state.selectedMetricIndex) {
      extraColMetricClass += '.selected';
    }

    var sparkline = h('div.col-metric' + extraColMetricClass, {
      'ev-click': hg.send(state.channels.mouseClickOnMetric, {
        index: curIndex
      })
    }, [
      h('div.highlight-overlay'),
      Util.renderMouseLine(mouseOffset),
      h('div.sparkline', {
        'ev-mousemove': new MouseMoveHandler(
            state.channels.mouseMoveOnSparkline),
        'ev-mouseout': hg.send(state.channels.mouseOutOfSparkline)
      }, [
        //renderThreshold(thresholdValue),
        Util.renderSparkline(points)
      ]),
      h('div.cur-value' + extraCurValueClass, [curValue])
    ]);
    return sparkline;
  });
  return h('div.metric-actions-list', list);
}

function renderMetric(state, metricData, serviceName, curData) {
  var namespaceId = VANADIUM_PRODUCTION_NAMESPACE_ID;
  if (metricData.Project === 'vanadium-auth-production') {
    namespaceId = AUTH_PRODUCTION_NAMESPACE_ID;
  }

  // Calculate current timestamp.
  var curTimestamp = curData.MaxTime;
  var extraCurTimestampClass = '';
  if (metricData.HistoryTimestamps && metricData.HistoryTimestamps.length > 0) {
    curTimestamp = metricData.HistoryTimestamps[
        metricData.HistoryTimestamps.length - 1];
  }
  if (state.mouseOffsetFactor >= 0) {
    curTimestamp =
        (curData.MaxTime - curData.MinTime) * state.mouseOffsetFactor +
        curData.MinTime;
    extraCurTimestampClass = '.history';
  }

  // Calculate current value.
  var curValue = Util.formatValue(metricData.CurrentValue);
  var extraCurValueClass = '';
  var mouseOffset = 100 * state.mouseOffsetFactor;
  if (mouseOffset >= 0) {
    curValue = Util.formatValue(Util.interpolateValue(
        metricData.CurrentValue, state.mouseOffsetFactor,
        metricData.HistoryTimestamps, metricData.HistoryValues));
    extraCurValueClass = '.history';
  }

  return h('div.metric-actions-content', [
      h('div.row', [
        h('div.item-label', 'Service Name'),
        h('div.item-value', serviceName)
      ]),
      h('div.row', [
        h('div.item-label', 'Service Version'),
        h('div.item-value', metricData.ServiceVersion)
      ]),
      h('div.row', [
        h('div.item-label', 'Metric Type'),
        h('div.item-value',
          metricData.ResultType.replace('resultType', ''))
      ]),
      h('div.row', [
        h('div.item-label', 'Metric Name'),
        h('div.item-value', metricData.MetricName)
      ]),
      h('div.row', [
        h('div.item-label', 'Current Value'),
        h('div.item-value' + extraCurValueClass, curValue)
      ]),
      h('div.row', [
        h('div.item-label', 'Current Time'),
        h('div.item-value' + extraCurTimestampClass,
          curTimestamp ===
            '?' ? '?' : dateformat(new Date(curTimestamp * 1000)))
      ]),
      h('div.row', [
        h('div.item-label', 'Logs'),
        h('div.item-value', h('a', {
          href: 'logs?p=' + metricData.Project +
              '&z=' + metricData.Zone +
              '&d=' + metricData.PodName +
              '&c=' + metricData.MainContainer,
          target: '_blank'
        }, metricData.MainContainer)),
      ]),
      h('div.space'),
      h('div.row', [
        h('div.item-label', 'Pod Name'),
        h('div.item-value', h('a', {
          href: 'https://app.google.stackdriver.com/gke/pod/1009941:vanadium:' +
              namespaceId + ':' + metricData.PodUID,
          target: '_blank'
        }, metricData.Instance)),
      ]),
      h('div.row', [
        h('div.item-label', 'Pod Node'),
        h('div.item-value', h('a', {
          href: 'https://app.google.stackdriver.com/instances/' +
              curData.Instances[metricData.PodNode],
          target: '_blank'
        }, metricData.PodNode)),
      ]),
      h('div.row', [
        h('div.item-label', 'Pod Status'),
        h('div.item-value', h('a', {
          href: 'cfg?p=' + metricData.Project +
              '&z=' + metricData.Zone +
              '&d=' + metricData.PodName,
          target: '_blank'
        }, 'status')),
      ]),
      h('div.row', [
        h('div.item-label', 'Zone'),
        h('div.item-value', metricData.Zone)
      ]),
      h('div.btn-close', {
        'ev-click': hg.send(state.channels.closeMetricActionsPanel)
      }, 'Close')
  ]);
}
