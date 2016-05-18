// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"io/ioutil"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	cloudmonitoring "google.golang.org/api/monitoring/v3"

	"v.io/jiri"
	"v.io/jiri/collect"
	"v.io/jiri/tool"
	"v.io/x/devtools/internal/cache"
	"v.io/x/devtools/internal/monitoring"
	"v.io/x/lib/cmdline"
	"v.io/x/lib/gcm"
)

const (
	numWorkers                = 64
	resultTypeServiceLatency  = "resultTypeServiceLatency"
	resultTypeServiceQPS      = "resultTypeServiceQPS"
	resultTypeServiceCounters = "resultTypeServiceCounters"
	resultTypeServiceMetadata = "resultTypeServiceMetadata"

	getKubeDataTaskTypePod  = "getKubeDataTaskTypePod"
	getKubeDataTaskTypeNode = "getKubeDataTaskTypeNode"

	logsStyle = "font-family: monospace; font-size: 12px; white-space:pre-wrap; word-wrap: break-word;"
)

var (
	addressFlag   string
	cacheFlag     string
	keyFileFlag   string
	staticDirFlag string
)

type podSpec struct {
	Metadata struct {
		Name   string `json:"name"`
		Uid    string `json:"uid"`
		Labels struct {
			Service string `json:"service"`
			Version string `json:"version"`
		} `json:"labels"`
	} `json:"metadata"`
	Spec struct {
		NodeName   string `json:"nodeName"`
		Containers []struct {
			Name string `json:"name"`
		} `json:"containers"`
	} `json:"spec"`
	Status struct {
		Phase     string `json:"phase"`
		StartTime string `json:"startTime"`
	} `json:"status"`
	zone    string
	project string
}

type nodeSpec struct {
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Spec struct {
		ExternalID string `json:"externalID"`
	} `json:"spec"`
}

type clusterLocation struct {
	project string
	zone    string
}

// Task and result for getKubeData.
type getKubeDataTask struct {
	location clusterLocation
	taskType string
}
type getKubeDataResult struct {
	pods     []*podSpec
	nodes    []*nodeSpec
	taskType string
	err      error
}

// Task and result for getMetricWorker.
type getMetricTask struct {
	resultType  string
	md          *cloudmonitoring.MetricDescriptor
	metricName  string
	pod         *podSpec
	extraLabels map[string]string
}
type getMetricResult struct {
	ResultType        string
	Instance          string
	Zone              string
	Project           string
	MetricName        string
	ExtraLabels       map[string]string
	CurrentValue      float64
	MinValue          float64
	MaxValue          float64
	HistoryTimestamps []int64
	HistoryValues     []float64
	ErrMsg            string
	PodName           string
	PodUID            string
	PodNode           string
	PodStatus         string
	MainContainer     string
	ServiceVersion    string
}

type getMetricResults []getMetricResult

func (m getMetricResults) Len() int           { return len(m) }
func (m getMetricResults) Less(i, j int) bool { return m[i].Instance < m[j].Instance }
func (m getMetricResults) Swap(i, j int)      { m[i], m[j] = m[j], m[i] }
func (m getMetricResults) Sort()              { sort.Sort(m) }

// Final result for getData endpoint.
type getDataResult struct {
	// These fields are indexed by metric names.
	ServiceLatency  map[string]getMetricResults
	ServiceQPS      map[string]getMetricResults
	ServiceCounters map[string]getMetricResults
	ServiceMetadata map[string]getMetricResults

	Instances map[string]string // instances -> external ids
	Oncalls   []string
	MinTime   int64
	MaxTime   int64
}

// Current service locations.
var locations = []clusterLocation{
	{
		project: "vanadium-production",
		zone:    "us-central1-c",
	},
	{
		project: "vanadium-production",
		zone:    "us-east1-c",
	},
	{
		project: "vanadium-auth-production",
		zone:    "us-central1-c",
	},
}

func init() {
	cmdServe.Flags.StringVar(&addressFlag, "address", ":8000", "Listening address for the server.")
	cmdServe.Flags.StringVar(&cacheFlag, "cache", "", "Directory to use for caching files.")
	cmdServe.Flags.StringVar(&keyFileFlag, "key", "", "The path to the service account's JSON credentials file.")
	cmdServe.Flags.StringVar(&staticDirFlag, "static", "", "Directory to use for serving static files.")
}

// cmdServe represents the 'serve' command of the oncall tool.
var cmdServe = &cmdline.Command{
	Runner: cmdline.RunnerFunc(runServe),
	Name:   "serve",
	Short:  "Serve oncall dashboard data from Google Storage",
	Long:   "Serve oncall dashboard data from Google Storage.",
}

func runServe(env *cmdline.Env, _ []string) (e error) {
	jirix, err := jiri.NewX(env)
	if err != nil {
		return err
	}

	// Set up the root/cache directory.
	root := cacheFlag
	if root == "" {
		tmpDir, err := jirix.NewSeq().TempDir("", "")
		if err != nil {
			return err
		}
		defer collect.Error(func() error { return jirix.NewSeq().RemoveAll(tmpDir).Done() }, &e)
		root = tmpDir
	}

	// Start server.
	http.HandleFunc("/data", func(w http.ResponseWriter, r *http.Request) {
		dataHandler(jirix, root, w, r)
	})
	http.HandleFunc("/logs", func(w http.ResponseWriter, r *http.Request) {
		logsHandler(jirix, root, w, r)
	})
	http.HandleFunc("/cfg", func(w http.ResponseWriter, r *http.Request) {
		cfgHandler(jirix, root, w, r)
	})
	http.HandleFunc("/pic", func(w http.ResponseWriter, r *http.Request) {
		picHandler(jirix, root, w, r)
	})
	staticHandler := http.FileServer(http.Dir(staticDirFlag))
	http.Handle("/", staticHandler)
	if err := http.ListenAndServe(addressFlag, nil); err != nil {
		return fmt.Errorf("ListenAndServe(%s) failed: %v", addressFlag, err)
	}

	return nil
}

func dataHandler(jirix *jiri.X, root string, w http.ResponseWriter, r *http.Request) {
	s, err := gcm.Authenticate(keyFileFlag)
	if err != nil {
		respondWithError(jirix, err, w)
		return
	}

	// Get start and end timestamps.
	if err := r.ParseForm(); err != nil {
		respondWithError(jirix, err, w)
		return
	}
	now := time.Now()
	startTimestamp := now.Unix() - 3600
	endTimestamp := now.Unix()
	strStartTimestamp := r.Form.Get("st")
	if strStartTimestamp != "" {
		var err error
		startTimestamp, err = strconv.ParseInt(strStartTimestamp, 10, 64)
		if err != nil {
			respondWithError(jirix, err, w)
			return
		}
	}
	strEndTimestamp := r.Form.Get("et")
	if strEndTimestamp != "" {
		var err error
		endTimestamp, err = strconv.ParseInt(strEndTimestamp, 10, 64)
		if err != nil {
			respondWithError(jirix, err, w)
			return
		}
	}

	// Get currently running pods and nodes from vanadium production clusters.
	podsByServices, nodes, err := getKubeData(jirix)
	if err != nil {
		respondWithError(jirix, err, w)
		return
	}

	// Create tasks of getting metrics from GCM.
	allTasks := []getMetricTask{}
	mdServiceLatency, err := gcm.GetMetric("service-latency", "vanadium-production")
	if err != nil {
		respondWithError(jirix, err, w)
		return
	}
	mdServiceQPS, err := gcm.GetMetric("service-qps-total", "vanadium-production")
	if err != nil {
		respondWithError(jirix, err, w)
		return
	}
	mdServiceCounters, err := gcm.GetMetric("service-counters", "vanadium-production")
	if err != nil {
		respondWithError(jirix, err, w)
		return
	}
	mdServiceMetadata, err := gcm.GetMetric("service-metadata", "vanadium-production")
	if err != nil {
		respondWithError(jirix, err, w)
		return
	}
	for serviceName, pods := range podsByServices {
		for _, pod := range pods {
			allTasks = append(allTasks,
				// Metadata.
				getMetricTask{
					resultType: resultTypeServiceMetadata,
					md:         mdServiceMetadata,
					metricName: serviceName,
					pod:        pod,
					extraLabels: map[string]string{
						"metadata_name": "build age",
					},
				},
				// QPS.
				getMetricTask{
					resultType: resultTypeServiceQPS,
					md:         mdServiceQPS,
					metricName: serviceName,
					pod:        pod,
				})
			// Latency.
			if serviceName == monitoring.SNIdentity {
				for _, n := range []string{monitoring.SNMacaroon, monitoring.SNBinaryDischarger} {
					allTasks = append(allTasks,
						getMetricTask{
							resultType: resultTypeServiceLatency,
							md:         mdServiceLatency,
							metricName: n,
							pod:        pod,
						})
				}
			} else {
				allTasks = append(allTasks,
					getMetricTask{
						resultType: resultTypeServiceLatency,
						md:         mdServiceLatency,
						metricName: serviceName,
						pod:        pod,
					})
			}
			// Counters.
			if serviceName == monitoring.SNMounttable {
				allTasks = append(allTasks,
					getMetricTask{
						resultType: resultTypeServiceCounters,
						md:         mdServiceCounters,
						metricName: monitoring.MNMounttableMountedServers,
						pod:        pod,
					},
					getMetricTask{
						resultType: resultTypeServiceCounters,
						md:         mdServiceCounters,
						metricName: monitoring.MNMounttableNodes,
						pod:        pod,
					},
				)
			}
		}
	}

	// Send tasks to workers.
	numTasks := len(allTasks)
	tasks := make(chan getMetricTask, numTasks)
	taskResults := make(chan getMetricResult, numTasks)
	for i := 0; i < numWorkers; i++ {
		go getMetricWorker(jirix, s, time.Unix(startTimestamp, 0), time.Unix(endTimestamp, 0), tasks, taskResults)
	}
	for _, task := range allTasks {
		tasks <- task
	}
	close(tasks)

	// Process results.
	result := getDataResult{
		ServiceLatency:  map[string]getMetricResults{},
		ServiceQPS:      map[string]getMetricResults{},
		ServiceCounters: map[string]getMetricResults{},
		ServiceMetadata: map[string]getMetricResults{},
	}
	for i := 0; i < numTasks; i++ {
		r := <-taskResults
		n := r.MetricName
		switch r.ResultType {
		case resultTypeServiceLatency:
			result.ServiceLatency[n] = append(result.ServiceLatency[n], r)
		case resultTypeServiceQPS:
			result.ServiceQPS[n] = append(result.ServiceQPS[n], r)
		case resultTypeServiceCounters:
			result.ServiceCounters[n] = append(result.ServiceCounters[n], r)
		case resultTypeServiceMetadata:
			result.ServiceMetadata[n] = append(result.ServiceMetadata[n], r)
		}
	}
	// Sort metrics by instance names.
	fnSortMetrics := func(m map[string]getMetricResults) {
		for n := range m {
			m[n].Sort()
		}
	}
	fnSortMetrics(result.ServiceLatency)
	fnSortMetrics(result.ServiceQPS)
	fnSortMetrics(result.ServiceCounters)
	fnSortMetrics(result.ServiceMetadata)

	result.MinTime = startTimestamp
	result.MaxTime = endTimestamp
	result.Instances = nodes

	// Get oncalls.
	oncalls, err := getOncalls(jirix)
	if err != nil {
		respondWithError(jirix, err, w)
		return
	}
	result.Oncalls = oncalls

	// Convert results to json and return it.
	b, err := json.MarshalIndent(&result, "", "  ")
	if err != nil {
		respondWithError(jirix, err, w)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
}

func dataHandler2(jirix *jiri.X, root string, w http.ResponseWriter, r *http.Request) {
	b, _ := ioutil.ReadFile("/usr/local/google/home/jingjin/data")
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
}

func getMetricWorker(jirix *jiri.X, s *cloudmonitoring.Service, startTime, endTime time.Time, tasks <-chan getMetricTask, results chan<- getMetricResult) {
	for task := range tasks {
		result := getMetricResult{
			ResultType:     task.resultType,
			Instance:       task.pod.Metadata.Name,
			Zone:           task.pod.zone,
			Project:        task.pod.project,
			MetricName:     task.metricName,
			ExtraLabels:    task.extraLabels,
			CurrentValue:   -1,
			MinValue:       math.MaxFloat64,
			MaxValue:       0,
			PodName:        task.pod.Metadata.Name,
			PodUID:         task.pod.Metadata.Uid,
			PodNode:        task.pod.Spec.NodeName,
			PodStatus:      task.pod.Status.Phase,
			MainContainer:  task.pod.Spec.Containers[0].Name,
			ServiceVersion: task.pod.Metadata.Labels.Version,
		}
		filters := []string{
			fmt.Sprintf("metric.type=%q", task.md.Type),
			fmt.Sprintf("metric.label.metric_name=%q", task.metricName),
			fmt.Sprintf("metric.label.gce_instance=%q", task.pod.Metadata.Name),
			fmt.Sprintf("metric.label.gce_zone=%q", task.pod.zone),
		}
		for labelKey, labelValue := range task.extraLabels {
			filters = append(filters, fmt.Sprintf("metric.label.%s=%q", labelKey, labelValue))
		}
		nextPageToken := ""
		points := []*cloudmonitoring.Point{}
		timestamps := []int64{}
		values := []float64{}
		for {
			resp, err := s.Projects.TimeSeries.List("projects/vanadium-production").
				IntervalStartTime(startTime.UTC().Format(time.RFC3339)).
				IntervalEndTime(endTime.UTC().Format(time.RFC3339)).
				Filter(strings.Join(filters, " AND ")).
				PageToken(nextPageToken).Do()
			if err != nil {
				result.ErrMsg = fmt.Sprintf("List() failed: %v", err)
				break
			}
			if len(resp.TimeSeries) > 0 {
				// We should only get one timeseries.
				ts := resp.TimeSeries[0]
				points = append(points, ts.Points...)
			}
			if result.ErrMsg != "" {
				break
			}
			nextPageToken = resp.NextPageToken
			if nextPageToken == "" {
				break
			}
		}
		for i := len(points) - 1; i >= 0; i-- {
			pt := points[i]
			epochTime, err := time.Parse(time.RFC3339, pt.Interval.EndTime)
			if err != nil {
				result.ErrMsg = fmt.Sprintf("Parse(%s) failed: %v", pt.Interval.EndTime)
				break
			}
			timestamp := epochTime.Unix()
			timestamps = append(timestamps, timestamp)
			value := pt.Value.DoubleValue
			values = append(values, value)
			result.MaxValue = math.Max(result.MaxValue, value)
			result.MinValue = math.Min(result.MinValue, value)
		}
		result.HistoryTimestamps = timestamps
		result.HistoryValues = values
		if len(values) > 0 {
			result.CurrentValue = values[len(values)-1]
		}
		results <- result
	}
}

// getKubeData gets:
// - pod data indexed by service names.
// - node external ids indexed by names.
func getKubeData(jirix *jiri.X) (map[string][]*podSpec, map[string]string, error) {
	retPods := map[string][]*podSpec{}
	retNodes := map[string]string{}

	// Get pods data.
	numTasks := len(locations) * 2
	tasks := make(chan getKubeDataTask, numTasks)
	taskResults := make(chan getKubeDataResult, numTasks)
	for i := 0; i < numTasks; i++ {
		go getKubeDataWorker(jirix, tasks, taskResults)
	}
	for _, loc := range locations {
		tasks <- getKubeDataTask{
			location: loc,
			taskType: getKubeDataTaskTypePod,
		}
		tasks <- getKubeDataTask{
			location: loc,
			taskType: getKubeDataTaskTypeNode,
		}
	}
	close(tasks)

	pods := []*podSpec{}
	nodes := []*nodeSpec{}
	for i := 0; i < numTasks; i++ {
		result := <-taskResults
		switch result.taskType {
		case getKubeDataTaskTypePod:
			pods = append(pods, result.pods...)
		case getKubeDataTaskTypeNode:
			nodes = append(nodes, result.nodes...)
		}
	}

	// Index pods data by service name.
	for _, pod := range pods {
		switch pod.Metadata.Labels.Service {
		case "auth":
			retPods[monitoring.SNIdentity] = append(retPods[monitoring.SNIdentity], pod)
		case "benchmarks":
			retPods[monitoring.SNBenchmark] = append(retPods[monitoring.SNBenchmark], pod)
		case "mounttable":
			retPods[monitoring.SNMounttable] = append(retPods[monitoring.SNMounttable], pod)
		case "proxy":
			retPods[monitoring.SNProxy] = append(retPods[monitoring.SNProxy], pod)
		case "role":
			retPods[monitoring.SNRole] = append(retPods[monitoring.SNRole], pod)
		}
	}
	// Index nodes names by ids.
	for _, node := range nodes {
		retNodes[node.Metadata.Name] = node.Spec.ExternalID
	}

	return retPods, retNodes, nil
}

func getKubeDataWorker(jirix *jiri.X, tasks <-chan getKubeDataTask, results chan<- getKubeDataResult) {
	for task := range tasks {
		kubeCtlArgs := []string{"get", "pods", "-o=json"}
		if task.taskType == getKubeDataTaskTypeNode {
			kubeCtlArgs = []string{"get", "nodes", "-o=json"}
		}
		var podItems struct {
			Items []*podSpec `json:"items"`
		}
		var nodeItems struct {
			Items []*nodeSpec `json:"items"`
		}
		out, err := runKubeCtl(jirix, task.location, kubeCtlArgs)
		if err != nil {
			results <- getKubeDataResult{
				err: err,
			}
			continue
		}
		switch task.taskType {
		case getKubeDataTaskTypePod:
			if err := json.Unmarshal(out, &podItems); err != nil {
				results <- getKubeDataResult{
					err: fmt.Errorf("Unmarshal() failed: %v", err),
				}
				continue
			}
			for _, item := range podItems.Items {
				item.zone = task.location.zone
				item.project = task.location.project
			}
			results <- getKubeDataResult{
				taskType: getKubeDataTaskTypePod,
				pods:     podItems.Items,
			}
		case getKubeDataTaskTypeNode:
			if err := json.Unmarshal(out, &nodeItems); err != nil {
				results <- getKubeDataResult{
					err: fmt.Errorf("Unmarshal() failed: %v", err),
				}
				continue
			}
			results <- getKubeDataResult{
				taskType: getKubeDataTaskTypeNode,
				nodes:    nodeItems.Items,
			}
		}
	}
}

func runKubeCtl(jirix *jiri.X, location clusterLocation, kubeCtlArgs []string) ([]byte, error) {
	f, err := ioutil.TempFile("", "")
	if err != nil {
		return nil, nil
	}
	defer jirix.NewSeq().RemoveAll(f.Name())
	s := tool.NewContext(tool.ContextOpts{
		Env: map[string]string{
			"KUBECONFIG": f.Name(),
		},
	}).NewSeq()
	var out bytes.Buffer
	getCredsArgs := []string{
		"container",
		"clusters",
		"get-credentials",
		"vanadium",
		"--project",
		location.project,
		"--zone",
		location.zone,
	}
	if err := s.Run("gcloud", getCredsArgs...).Capture(&out, &out).Last("kubectl", kubeCtlArgs...); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func getOncalls(jirix *jiri.X) ([]string, error) {
	s := jirix.NewSeq()
	var out bytes.Buffer
	if err := s.Capture(&out, nil).Last("jiri", "oncall"); err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimSpace(out.String()), ","), nil
}

func logsHandler(jirix *jiri.X, root string, w http.ResponseWriter, r *http.Request) {
	// Parse project, zone, pod name, and container.
	f, err := parseForm(r, "p", "z", "d", "c")
	if err != nil {
		respondWithError(jirix, err, w)
		return
	}
	project, zone, pod, container := f["p"], f["z"], f["d"], f["c"]
	out, err := runKubeCtl(jirix, clusterLocation{
		project: project,
		zone:    zone,
	}, []string{"logs", pod, container})
	if err != nil {
		respondWithError(jirix, err, w)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	content := fmt.Sprintf("<pre style='%s'>%s</pre>", logsStyle, html.EscapeString(string(out)))
	w.Write([]byte(content))
}

func cfgHandler(jirix *jiri.X, root string, w http.ResponseWriter, r *http.Request) {
	// Parse project, zone, and pod name.
	f, err := parseForm(r, "p", "z", "d")
	if err != nil {
		respondWithError(jirix, err, w)
		return
	}
	project, zone, pod := f["p"], f["z"], f["d"]
	out, err := runKubeCtl(jirix, clusterLocation{
		project: project,
		zone:    zone,
	}, []string{"get", "pods", pod, "-o=json"})
	if err != nil {
		respondWithError(jirix, err, w)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	content := fmt.Sprintf("<pre style='%s'>%s</pre>", logsStyle, html.EscapeString(string(out)))
	w.Write([]byte(content))
}

func picHandler(jirix *jiri.X, root string, w http.ResponseWriter, r *http.Request) {
	// Parameter "id" specifies the id of the pic.
	f, err := parseForm(r, "id")
	if err != nil {
		respondWithError(jirix, err, w)
		return
	}
	id := f["id"]

	// Read picture file from Google Storage.
	cachedFile, err := cache.StoreGoogleStorageFile(jirix, root, bucketPics, id+".png")
	if err != nil {
		// Read "_unknown.jpg" as fallback.
		cachedFile, err = cache.StoreGoogleStorageFile(jirix, root, bucketPics, "_unknown.jpg")
		if err != nil {
			respondWithError(jirix, err, w)
			return
		}
	}
	bytes, err := jirix.NewSeq().ReadFile(cachedFile)
	if err != nil {
		respondWithError(jirix, err, w)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-control", "public, max-age=2592000")
	w.Write(bytes)
}

func parseForm(r *http.Request, fields ...string) (map[string]string, error) {
	m := map[string]string{}
	r.ParseForm()
	for _, f := range fields {
		value := r.Form.Get(f)
		if value == "" {
			return nil, fmt.Errorf("parameter %q not found", f)
		}
		m[f] = value
	}
	return m, nil
}

func respondWithError(jirix *jiri.X, err error, w http.ResponseWriter) {
	fmt.Fprintf(jirix.Stderr(), "%v\n", err)
	http.Error(w, fmt.Sprintf("500 internal server error\n\n%v", err), http.StatusInternalServerError)
}
