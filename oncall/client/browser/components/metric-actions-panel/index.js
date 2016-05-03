// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

var hg = require('mercury');
var h = require('mercury').h;

var Util = require('../../util');

module.exports = {
  render: render
};

var VANADIUM_PRODUCTION_NAMESPACE_ID = '4b20e0dc-cacf-11e5-87ec-42010af0020b';
var AUTH_PRODUCTION_NAMESPACE_ID = '2b6d405a-b4a6-11e5-9776-42010af000a6';

/** The main render function. */
function render(state, selectedMetric, data) {
  var namespaceId = VANADIUM_PRODUCTION_NAMESPACE_ID;
  if (selectedMetric.metricData.Project === 'vanadium-auth-production') {
    namespaceId = AUTH_PRODUCTION_NAMESPACE_ID;
  }
  var panel = h('div.metric-actions-content', [
      h('div.row', [
        h('div.item-label', 'Service Name'),
        h('div.item-value', selectedMetric.serviceName)
      ]),
      h('div.row', [
        h('div.item-label', 'Service Version'),
        h('div.item-value', selectedMetric.metricData.ServiceVersion)
      ]),
      h('div.row', [
        h('div.item-label', 'Metric Type'),
        h('div.item-value',
          selectedMetric.metricData.ResultType.replace('resultType', ''))
      ]),
      h('div.row', [
        h('div.item-label', 'Metric Name'),
        h('div.item-value', selectedMetric.metricData.MetricName)
      ]),
      h('div.row', [
        h('div.item-label', 'Current Value'),
        h('div.item-value',
          Util.formatValue(selectedMetric.metricData.CurrentValue))
      ]),
      h('div.row', [
        h('div.item-label', 'Logs'),
        h('div.item-value', h('a', {
          href: 'logs?p=' + selectedMetric.metricData.Project +
              '&z=' + selectedMetric.metricData.Zone +
              '&d=' + selectedMetric.metricData.PodName +
              '&c=' + selectedMetric.metricData.MainContainer,
          target: '_blank'
        }, selectedMetric.metricData.MainContainer)),
      ]),
      h('div.space'),
      h('div.row', [
        h('div.item-label', 'Pod Name'),
        h('div.item-value', h('a', {
          href: 'https://app.google.stackdriver.com/gke/pod/1009941:vanadium:' +
              namespaceId + ':' + selectedMetric.metricData.PodUID,
          target: '_blank'
        }, selectedMetric.metricData.Instance)),
      ]),
      h('div.row', [
        h('div.item-label', 'Pod Node'),
        h('div.item-value', h('a', {
          href: 'https://app.google.stackdriver.com/instances/' +
              data.Instances[selectedMetric.metricData.PodNode],
          target: '_blank'
        }, selectedMetric.metricData.PodNode)),
      ]),
      h('div.row', [
        h('div.item-label', 'Pod Config'),
        h('div.item-value', h('a', {
          href: 'cfg?p=' + selectedMetric.metricData.Project +
              '&z=' + selectedMetric.metricData.Zone +
              '&d=' + selectedMetric.metricData.PodName,
          target: '_blank'
        }, 'cfg')),
      ]),
      h('div.row', [
        h('div.item-label', 'Pod Status'),
        h('div.item-value', selectedMetric.metricData.PodStatus)
      ]),
      h('div.row', [
        h('div.item-label', 'Zone'),
        h('div.item-value', selectedMetric.metricData.Zone)
      ]),
      h('div.btn-close', {
        'ev-click': hg.send(state.channels.closeMetricActionsPanel)
      }, 'Close')
  ]);
  return h('div.metric-actions-container', panel);
}
