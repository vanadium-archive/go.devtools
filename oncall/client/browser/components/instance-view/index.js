// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
 * The instance view.
 *
 * In this view, we show detailed data of vanadium services or nginx servers for
 * a single GCE instance.
 *
 * The page is divided into several sections:
 * - We first show a large graph for the main metrics we care about the most,
 *   such as "vanadium services request latency".
 * - We then show various "helper" stats that can help people diagnose problems
 *   in the main metrics, such as "mounted nodes in mounttable".
 * - After that, we show metrics for the GCE instance itself, such as CPU usage,
 *   disk usage, etc. It also has links to let user ssh into the machine and see
 *   its log.
 * - For vanadium services, it will also show a table for services'
 *   metadata/buildinfo.
 */

var hg = require('mercury');
var h = require('mercury').h;
var dateformat = require('dateformat');

var Consts = require('../../constants');
var fullGraphComponent = require('../full-graph');
var metricsGroupComponent = require('../metrics-group');
var dataTableComponent = require('../data-table');
var Util = require('../../util');

module.exports = create;
module.exports.render = render;

/** Constructor. */
function create(data) {
  if (data === null) {
    return null;
  }

  var instance = data.appState.instanceLevelInstance;
  var isVanadiumServices = instance.startsWith('vanadium');

  // Main metrics.
  // For vanadium services, we show request latency as the main metrics.
  // For nginx servers, we show its load.
  var mainMetrics = isVanadiumServices ?
      data.data.CloudServiceLatency : data.data.NginxLoad;
  var mainMetricsTitle = isVanadiumServices ? 'REQUEST LATENCY' : 'SERVER LOAD';
  var mainMetricKeys = isVanadiumServices ?
      Consts.metricKeys.latency : Consts.metricKeys.nginxLoad;
  var sortedMainMetrics = mainMetricKeys.map(function(metric) {
    return mainMetrics[metric];
  });

  // Helper metrics.
  // For now, we only have helper metrics for vanadium services, which are
  // various stats from mounttable.
  var helperMetrics = isVanadiumServices ? data.data.CloudServiceStats : null;
  var helperMetricsTitle = isVanadiumServices ? 'STATS' : '';
  var helperMetricKeys = isVanadiumServices ? Consts.metricKeys.stats : null;
  var sortedHelperMetrics = [];
  if (isVanadiumServices) {
    sortedHelperMetrics = helperMetricKeys.map(function(metric) {
      return helperMetrics[metric];
    });
  }

  // GCE metrics.
  var gceMetrics = isVanadiumServices ?
      data.data.CloudServiceGCE : data.data.NginxGCE;
  var gceMetricsTitle = 'GCE';
  var gceMetricKeys = Consts.metricKeys.gce;
  var sortedGCEMetrics = gceMetricKeys.map(function(metric) {
    return gceMetrics[metric];
  });
  var instanceId = data.data.GCEInfo.Id;

  // Data table for showing vanadium services' metadata/buildinfo.
  var dataTableData = data.data.CloudServiceBuildInfo;
  var rows = [];
  if (!Util.isEmptyObj(dataTableData)) {
    rows.push(['SERVICE', 'PRISTINE', 'SNAPSHOT', 'TIME', 'USER', 'LINKS']);
    Object.keys(dataTableData).sort().forEach(function(serviceName) {
      var rowData = dataTableData[serviceName];
      var curRow = [
        Consts.getDisplayName(serviceName),
        rowData.IsPristine, rowData.Snapshot,
        dateformat(new Date(rowData.Time * 1000), 'yyyymmdd-HH:MM'),
        rowData.User,
        h('a', {
          href: genLogsLink(instanceId, 'v-' + rowData.ServiceName + '.info'),
          target: '_blank'
        }, 'log')];
      rows.push(curRow);
    });
  }

  // These two observables keep track of which non-main metric the mouse cursor
  // is currently hovering over, as well as its relative X position.
  //
  // When a non-main metric is hovered over, we will overlay its data points in
  // the main metrics graph for easier comparison. We also render a "mouse line"
  // across all graphs.
  var hoveredMetric = hg.struct({});
  var mouseOffsetFactor = hg.value(-1);

  var state = hg.state({
    data: data.data,
    appState: hg.struct(data.appState),

    mainMetricsGraph: fullGraphComponent({
      title: mainMetricsTitle,
      collectionTimestamp: data.collectionTimestamp,
      metrics: sortedMainMetrics,
      mouseOffsetFactor: mouseOffsetFactor,
      hoveredMetric: hoveredMetric
    }),

    helperMetricsGroup: isVanadiumServices ? metricsGroupComponent({
      title: helperMetricsTitle,
      metrics: sortedHelperMetrics,
      mouseOffsetFactor: mouseOffsetFactor,
      hoveredMetric: hoveredMetric
    }) : null,

    gceMetricsGroup: metricsGroupComponent({
      title: gceMetricsTitle,
      metrics: sortedGCEMetrics,
      mouseOffsetFactor: mouseOffsetFactor,
      hoveredMetric: hoveredMetric,
      links: [
        {
          name: 'ssh',
          link: genCloudSSHLink(data.appState.instanceLevelZone, instance)
        },
        {
          name: 'log',
          link: genLogsLink(instanceId, '')
        }
      ],
    }),

    dataTable: rows.length > 0 ? dataTableComponent({
      title: 'BUILD INFO',
      rows: rows
    }) : null
  });

  return state;
}

/** The main render function. */
function render(state) {
  var items = [
    fullGraphComponent.render(state.mainMetricsGraph)
  ];
  if (state.helperMetricsGroup) {
    items.push(metricsGroupComponent.render(state.helperMetricsGroup));
  }
  items.push(metricsGroupComponent.render(state.gceMetricsGroup));
  if (state.dataTable) {
    items.push(dataTableComponent.render(state.dataTable));
  }
  return h('div.instance-view', items);
}

function genCloudSSHLink(zone, instance) {
  return 'https://cloudssh.developers.google.com/projects/' +
      'vanadium-production/zones/' + zone +
      '/instances/' + instance + '?authuser=0&hl=en_US';
}

function genLogsLink(instanceId, logName) {
  return 'https://pantheon.corp.google.com/project/vanadium-production/logs?' +
      'service=compute.googleapis.com&key1=instance&key2=' + instanceId +
      '&logName=' + logName +
      '&minLogLevel=0&expandAll=false&timezone=America%2FLos_Angeles';
}
