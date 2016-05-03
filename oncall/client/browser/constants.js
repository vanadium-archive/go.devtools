// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/** Constants for all the available zone names. */
var zones = Object.freeze({
  US_CENTRAL1_C: 'us-central1-c',
  US_CENTRAL1_A: 'us-central1-a',
  US_CENTRAL1_B: 'us-central1-b',
  US_CENTRAL1_F: 'us-central1-f',
  EUROPE_WEST1_B: 'europe-west1-b',
  EUROPE_WEST1_C: 'europe-west1-b',
  EUROPE_WEST1_D: 'europe-west1-d',
  ASIA_EAST1_A: 'asia-east1-a',
  ASIA_EAST1_B: 'asia-east1-b',
  ASIA_EAST1_C: 'asia-east1-c',
});

/** Constants for all the metric names in the raw data. */
var metricNames = Object.freeze({
  MN_BINARY_DISCHARGER: 'binary discharger',
  MN_BENCHMARKS: 'benchmark service',
  MN_IDENTITY: 'identity service',
  MN_MACAROON: 'macaroon service',
  MN_MOUNTTABLE: 'mounttable',
  MN_PROXY: 'proxy service',
  MN_ROLE: 'role service',
  MN_MT_MOUNTED_SERVERS: 'mounttable mounted servers',
  MN_MT_NODES: 'mounttable nodes'
});

/** Constants for all the keys in raw data. */
var dataKeys = Object.freeze({
  DK_SERVICE_LATENCY: 'ServiceLatency',
  DK_SERVICE_COUNTERS: 'ServiceCounters',
  DK_SERVICE_QPS: 'ServiceQPS',
  DK_SERVICE_METADATA: 'ServiceMetadata'
});

/** A map from long name strings to their shorter forms. */
var displayNames = Object.freeze({
  'active-connections': 'ACTIVE CONN',
  'application repository': 'APPS',
  'application repository latency': 'APPS',
  'applicationd': 'APPS',
  'benchmark service': 'BENCHMARK',
  'binary discharger': 'DISCHARGER',
  'binary discharger latency': 'DISCHARGER',
  'binary repository': 'BINS',
  'binary repository latency': 'BINS',
  'binaryd': 'BINS',
  'cpu-usage': 'CPU%',
  'disk-usage': 'DISK%',
  'groups service': 'GROUPS',
  'groups service latency': 'GROUPS',
  'healthCheckLatency': 'HEALTH LAT',
  'identityd': 'IDEN',
  'identity service': 'IDENTITY',
  'macaroon service': 'MACAROON',
  'macaroon service latency': 'MACAROON',
  'memory-usage': 'RAM%',
  'mounttable': 'MOUNTTABLE',
  'mounttabled': 'MTB',
  'mounttable latency': 'MTB',
  'mounttable mounted servers': 'MTB SERVERS',
  'mounttable nodes': 'MTB NODES',
  'mounttable qps': 'MTB QPS',
  'ping': 'PING',
  'proxy service': 'PROXY',
  'proxy service latency': 'PROXY',
  'proxyd': 'PROXY',
  'qps': 'QPS',
  'reading-connections': 'READING CONN',
  'role service': 'ROLES',
  'role service latency': 'ROLES',
  'roled': 'ROLED',
  'tcpconn': 'TCP CONN',
  'waiting-connections': 'WAITING CONN',
  'writing-connections': 'WRITING CONN'
});

/**
 * Gets the display name of the given name string.
 * @param {string} name.
 */
function getDisplayName(name) {
  var displayName = displayNames[name];
  if (!displayName) {
    return name;
  }
  return displayName;
}

/** Metric names used in retrieved data. */
var metricKeys = {
  latency: [
    'mounttable latency',
    'application repository latency',
    'binary repository latency',
    'binary discharger latency',
    'google identity service latency',
    'macaroon service latency',
    'groups service latency',
    'role service latency',
    'proxy service latency'
  ],
  stats: [
    'mounttable mounted servers',
    'mounttable nodes',
    'mounttable qps'
  ],
  gce: [
    'cpu-usage',
    'disk-usage',
    'memory-usage',
    'ping',
    'tcpconn'
  ],
  nginxLoad: [
    'healthCheckLatency',
    'qps',
    'active-connections',
    'reading-connections',
    'writing-connections',
    'waiting-connections'
  ]
};

/** Metric objects for vanadium cloud services. */
var cloudServiceMetrics = [
  createMetric('MOUNTTABLE', 'CloudServiceLatency', metricKeys.latency[0],
      false, false, 'V REQUEST LATENCY'),
  createMetric('DISCHARGER', 'CloudServiceLatency', metricKeys.latency[3]),
  createMetric('IDENTITY', 'CloudServiceLatency', metricKeys.latency[4]),
  createMetric('MACAROON', 'CloudServiceLatency', metricKeys.latency[5]),
  createMetric('ROLES', 'CloudServiceLatency', metricKeys.latency[7]),
  createMetric('PROXY', 'CloudServiceLatency', metricKeys.latency[8], true),
  createMetric('MOUNTED SERVERS', 'CloudServiceStats', metricKeys.stats[0],
      false, false, 'V STATS'),
  createMetric('MOUNTED NODES', 'CloudServiceStats', metricKeys.stats[1]),
  createMetric('MTB QPS', 'CloudServiceStats', metricKeys.stats[2], true)
];

/** Metric objects for cloud services GCE. */
var cloudServiceGCEMetrics = [
  createMetric('CPU%', 'CloudServiceGCE', metricKeys.gce[0],
      false, false, 'V GCE'),
  createMetric('RAM%', 'CloudServiceGCE', metricKeys.gce[1]),
  createMetric('DISK%', 'CloudServiceGCE', metricKeys.gce[2]),
  createMetric('PING', 'CloudServiceGCE', metricKeys.gce[3]),
  createMetric('TCP CONN', 'CloudServiceGCE', metricKeys.gce[4], false, true)
];

/** Metric objects for Nginx. */
var nginxMetrics = [
  createMetric('HEALTH LAT', 'NginxLoad', metricKeys.nginxLoad[0],
      false, false, 'NGINX SERVER LOAD'),
  createMetric('QPS', 'NginxLoad', metricKeys.nginxLoad[1]),
  createMetric('ACTIVE CONN', 'NginxLoad', metricKeys.nginxLoad[1]),
  createMetric('READING CONN', 'NginxLoad', metricKeys.nginxLoad[2]),
  createMetric('WRITING CONN', 'NginxLoad', metricKeys.nginxLoad[3]),
  createMetric('WAITING CONN', 'NginxLoad', metricKeys.nginxLoad[4], true)
];

/** Metric objects for Nginx GCE. */
var nginxGCEMetrics = [
  createMetric('CPU%', 'NginxGCE', metricKeys.gce[0],
      false, false, 'NGINX GCE'),
  createMetric('RAM%', 'NginxGCE', metricKeys.gce[1]),
  createMetric('DISK%', 'NginxGCE', metricKeys.gce[2]),
  createMetric('PING', 'NginxGCE', metricKeys.gce[3]),
  createMetric('TCP CONN', 'NginxGCE', metricKeys.gce[4])
];

/** All available metric objects. */
var mainMetrics = cloudServiceMetrics.concat(nginxMetrics);

/**
 * Creates a metric.
 *
 * The data loaded from the backend server looks like this:
 * nginx-worker-4: {
 *   NginxLoad: {
 *     active-connections: {...},
 *     qps: {...}
 *   },
 *   NginxGCE: {
 *     cpu-usage: {...},
 *     disk-usage: {...}
 *   },
 *   ...
 * }
 *
 * Keys like "NginxLoad" and "NginxGCE" are data keys.
 * Keys like "active-connections" and "cpu-usage" are metric keys.
 *
 * @param {string} label - The display label.
 * @param {string} dataKey - See above.
 * @param {string} metricKey - See above.
 * @param {boolean} addMinorDivider - Whether to add a minor divider in the
 *     data table.
 * @param {boolean} addMajorDivider - Whether to add a major divider in the
 *     data table.
 * @param {string} sectionLabel - The label to show above a data table section.
 * @return {Object}
 */
function createMetric(label, dataKey, metricKey,
    addMinorDivider, addMajorDivider, sectionLabel) {
  return {
    label: label,
    dataKey: dataKey,
    metricKey: metricKey,
    addMinorDivider: addMinorDivider,
    addMajorDivider: addMajorDivider,
    sectionLabel: sectionLabel
  };
}

module.exports = {
  zones: zones,
  metricNames: metricNames,
  dataKeys: dataKeys,
  orderedZones: [
    zones.US_CENTRAL1_C,
    zones.US_CENTRAL1_A,
    zones.US_CENTRAL1_B,
    zones.US_CENTRAL1_F,
    zones.EUROPE_WEST1_B,
    zones.EUROPE_WEST1_C,
    zones.EUROPE_WEST1_D,
    zones.ASIA_EAST1_A,
    zones.ASIA_EAST1_B,
    zones.ASIA_EAST1_C
  ],
  getDisplayName: getDisplayName,
  metricKeys: metricKeys,
  cloudServiceMetrics: cloudServiceMetrics,
  cloudServiceGCEMetrics: cloudServiceGCEMetrics,
  nginxMetrics: nginxMetrics,
  nginxGCEMetrics: nginxGCEMetrics,
  mainMetrics: mainMetrics
};
