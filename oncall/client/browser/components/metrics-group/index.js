// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/**
 * A metric group shows a set of metric items in multiple rows where each row
 * has "numColumns" items. It can also show a set of links (optionally) after
 * the group title.
 */

var hg = require('mercury');
var h = require('mercury').h;

var metricItemComponent = require('../metric-item');

module.exports = create;
module.exports.render = render;

var numColumns = 5;

/** Constructor. */
function create(data) {
  // Transform data.metrics to an array of metricItemComponents.
  var metrics = data.metrics.map(function(metric) {
    return metricItemComponent({
      metric: metric,
      mouseOffsetFactor: data.mouseOffsetFactor,
      hoveredMetric: data.hoveredMetric
    });
  });

  // Calculate overall health.
  var healthy = true;
  metrics.forEach(function(metric) {
    healthy &= metric.healthy;
  });

  var state = hg.state({
    title: data.title,
    links: data.links,
    healthy: healthy,
    metricItems: hg.array(metrics)
  });

  return state;
}

/** The main render function. */
function render(state) {
  // Organize metric items in rows and fill the empty space with fillers.
  var itemSize = state.metricItems.length;
  var paddedItemSize = (Math.ceil(itemSize / numColumns)) * numColumns;
  var rows = [];
  var curRow = null;
  for (var i = 0; i < paddedItemSize; i++) {
    if (i % numColumns  === 0) {
      if (curRow !== null) {
        rows.push(h('div.metrics-group-row', curRow));
      }
      curRow = [];
    }
    if (i > itemSize - 1) {
      curRow.push(h('div.metric-item-filler'));
    } else {
      curRow.push(metricItemComponent.render(state.metricItems[i]));
    }
  }
  rows.push(h('div.metrics-group-row', curRow));

  // Group title and links.
  var titleItems = [
      h('div.metrics-group-title', h('span', state.title))
  ];
  if (state.links) {
    var links = state.links.map(function(link) {
      return h('a', {
        href: link.link,
        target: '_blank'
      }, link.name);
    });
    titleItems.push(h('div.link-container', links));
  }

  return h('div.metrics-group', [
      h('div.metrics-group-title-container', titleItems),
      h('div.metrics-group-items-container', [
        h('div.metrics-group-items', rows)
      ])
  ]);
}
