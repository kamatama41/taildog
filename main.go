package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"text/template"
	"time"
)

var (
	query = flag.String("q", "",
		"Search query. See https://docs.datadoghq.com/logs/explorer/search/ for more details of query.")
	msgFormat = flag.String("f", "{{.Timestamp}} {{.Host}} {{.Service}} {{.Message}}",
		"Message format of entries in Golang's template style.\n"+
			"You can use any field in the \"content\" of the response of the Log Query API.\n"+
			"https://docs.datadoghq.com/api/?lang=bash#get-a-list-of-logs\n")
	interval = flag.Int("i", 15, "Interval time in seconds until the next attempt.")
	limit    = flag.Int("l", 1000, "Number of logs fetched at once.")
	fromStr  = flag.String("from", "", "Minimum timestamp for requested logs, should be an ISO-8601 string.")
	toStr    = flag.String("to", "", "Maximum timestamp for requested logs, should be an ISO-8601 string.")
	version  = flag.Bool("V", false, "Show version of taildog")

	apiKey = getEnv("DD_API_KEY")
	appKey = getEnv("DD_APP_KEY")
)

type config struct {
	query     string
	from      myTime
	to        myTime
	limit     int
	tmpl      *template.Template
	nextLogId string
	follow    bool
}

type logsInfo struct {
	Logs      []logInfo `json:"logs"`
	NextLogId string    `json:"nextLogId"`
	Status    string    `json:"status"`
}

type logInfo struct {
	Id      string     `json:"id"`
	Content logContent `json:"content"`
}

type logContent struct {
	Timestamp  string                 `json:"timestamp"`
	Tags       []string               `json:"tags"`
	Attributes map[string]interface{} `json:"attributes"`
	Host       string                 `json:"host"`
	Service    string                 `json:"service"`
	Message    string                 `json:"message"`
}

type myTime struct {
	time.Time
}

func main() {
	flag.Parse()
	if *version {
		println(VERSION)
		return
	}

	cfg, err := newConfig()
	if err != nil {
		log.Fatal(err)
	}

	cfg, err = showLogs(cfg)
	if err != nil {
		log.Fatal(err)
	}

	for cfg.follow {
		time.Sleep(time.Duration(*interval) * time.Second)

		cfg, err = showLogs(cfg)
		if err != nil {
			log.Fatal(err)
		}
	}
}

func showLogs(cfg *config) (*config, error) {
	reqBody := map[string]interface{}{
		"query": cfg.query,
		"limit": cfg.limit,
		"time": map[string]int64{
			"from": cfg.from.UnixMillis(),
			"to":   cfg.to.UnixMillis(),
		},
		"sort": "asc",
	}
	if cfg.nextLogId != "" {
		reqBody["startAt"] = cfg.nextLogId
	}

	reqBodyJson, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf(
		"https://api.datadoghq.com/api/v1/logs-queries/list?api_key=%s&application_key=%s", apiKey, appKey)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBodyJson))
	if err != nil {
		return nil, err
	}
	req.Header.Add("content-type", "application/json")

	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	resBody, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	if res.StatusCode/100 != 2 {
		err = fmt.Errorf("unexpected status %d: %s", res.StatusCode, string(resBody))
		return nil, err
	}

	logsInfo := &logsInfo{}
	err = json.Unmarshal(resBody, logsInfo)
	if err != nil {
		return nil, err
	}

	for _, l := range logsInfo.Logs {
		err := cfg.tmpl.Execute(os.Stdout, l.Content)
		if err != nil {
			return nil, err
		}
	}

	err = cfg.update(logsInfo)
	return cfg, err
}

func getEnv(key string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	log.Fatalf("Env %s must be sed but not found.", key)
	return ""
}

func now() myTime {
	return newTime(time.Now())
}

func newTime(t time.Time) myTime {
	return myTime{t}
}

func parseTime(str string) (myTime, error) {
	t, err := time.Parse(time.RFC3339, str)
	return newTime(t), err
}

func (t myTime) UnixMillis() int64 {
	return t.UnixNano() / 1000000
}

func (t myTime) Add(d time.Duration) myTime {
	return myTime{t.Time.Add(d)}
}

func newConfig() (*config, error) {
	tmpl, err := template.New("logLine").Parse(*msgFormat + "\n")
	if err != nil {
		return nil, err
	}

	var from, to myTime
	if *fromStr != "" {
		from, err = parseTime(*fromStr)
		if err != nil {
			return nil, err
		}
	}
	if *toStr != "" {
		to, err = parseTime(*toStr)
		if err != nil {
			return nil, err
		}
	}

	if (from.IsZero() && !to.IsZero()) || (!from.IsZero() && to.IsZero()) {
		return nil, fmt.Errorf("both 'from' and 'to' must be set")
	}

	follow := false
	if from.IsZero() && to.IsZero() {
		follow = true
		// First attempt for the follow mode, retrieve logs from 30 seconds ago
		from = now().Add(time.Duration(-30 * time.Second))
		to = now()
	}

	return &config{
		query:  *query,
		from:   from,
		to:     to,
		limit:  *limit,
		follow: follow,
		tmpl:   tmpl,
	}, nil
}

func (cfg *config) update(info *logsInfo) error {
	cfg.nextLogId = info.NextLogId
	if cfg.nextLogId != "" {
		// There are remaining logs (keep last condition)
		return nil
	}

	if len(info.Logs) != 0 {
		ts := info.Logs[len(info.Logs)-1].Content.Timestamp
		t, err := parseTime(ts)
		if err != nil {
			return err
		}
		// (Timestamp of the last log) + 1ms, to avoid to show duplicate logs
		cfg.from = t.Add(time.Duration(1 * time.Millisecond))
	}
	cfg.to = now()

	return nil
}

func (cfg *config) String() string {
	return fmt.Sprintf("%s %s %s", cfg.from.Format(time.RFC3339Nano), cfg.to.Format(time.RFC3339Nano), cfg.nextLogId)
}
