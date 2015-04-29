// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

function mouseOverIncidentItem(incidentEle, details) {
  var rect = incidentEle.getBoundingClientRect();
  var ele = document.getElementById("incident-details");
  ele.style.display = "block";
  ele.innerText = details;
  ele.style.top = (rect.top + 41) + "px";
  ele.style.left = (rect.left) + "px";
}

function mouseOutIncidentItem() {
  var ele = document.getElementById("incident-details");
  ele.style.display = "none";
}
