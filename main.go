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
	msgFormat = flag.String("f", "{{.Timestamp}} {{.Host}} {{.Service}} {{.Message}} {{.Attributes}}",
		"Message format of entries in Golang's template style.\n"+
			"You can use any field in the \"content\" of the response of the Log Query API.\n"+
			"https://docs.datadoghq.com/api/#get-a-list-of-logs\n")
	interval   = flag.Int("i", 15, "Interval time in seconds until the next attempt.")
	limit      = flag.Int("l", 1000, "Number of logs fetched at once.")
	fromStr    = flag.String("from", "", "Minimum timestamp for requested logs. See https://docs.datadoghq.com/api/#get-a-list-of-logs for more details of its format.")
	toStr      = flag.String("to", "", "Maximum timestamp for requested logs. See https://docs.datadoghq.com/api/#get-a-list-of-logs for more details of its format.")
	versionFlg = flag.Bool("version", false, "Show version of taildog.")

	version = "dev"
)

type config struct {
	query    string
	from     string
	to       string
	limit    int
	tmpl     *template.Template
	follow   bool
	lastInfo *logsInfo
	apiKey   string
	appKey   string
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
	Timestamp  string     `json:"timestamp"`
	Tags       []string   `json:"tags"`
	Attributes attributes `json:"attributes"`
	Host       string     `json:"host"`
	Service    string     `json:"service"`
	Message    string     `json:"message"`
}

type attributes map[string]interface{}

func (a attributes) String() string {
	j, _ := json.Marshal(a)
	return string(j)
}

func main() {
	flag.Parse()
	if *versionFlg {
		println(version)
		return
	}

	cfg, err := newConfig()
	if err != nil {
		log.Fatal(err)
	}

	logs, err := showLogs(cfg)
	if err != nil {
		log.Fatal(err)
	}

	for cfg.follow {
		time.Sleep(time.Duration(*interval) * time.Second)

		err = cfg.update(logs)
		if err != nil {
			log.Fatal(err)
		}

		logs, err = showLogs(cfg)
		if err != nil {
			log.Fatal(err)
		}
	}
}

func showLogs(cfg *config) (*logsInfo, error) {
	//cfg.Debug()

	reqBody := map[string]interface{}{
		"query": cfg.query,
		"limit": cfg.limit,
		"time": map[string]string{
			"from": cfg.from,
			"to":   cfg.to,
		},
		"sort": "asc",
	}

	lastInfo := cfg.lastInfo
	if lastInfo.NextLogId != "" {
		reqBody["startAt"] = lastInfo.NextLogId
	}

	reqBodyJson, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf(
		"https://api.datadoghq.com/api/v1/logs-queries/list?api_key=%s&application_key=%s", cfg.apiKey, cfg.appKey)
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

	// Remove duplicate logs
	var logsToDisplay []logInfo
FilterLoop:
	for _, l := range logsInfo.Logs {
		if lastInfo != nil {
			for _, lastLog := range lastInfo.Logs {
				if l.Id == lastLog.Id {
					continue FilterLoop
				}
			}
			logsToDisplay = append(logsToDisplay, l)
		}
	}

	for _, l := range logsToDisplay {
		err := cfg.tmpl.Execute(os.Stdout, l.Content)
		if err != nil {
			return nil, err
		}
	}

	return logsInfo, err
}

func getEnv(key string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	log.Fatalf("Env %s must be sed but not found.", key)
	return ""
}

func formatTime(t time.Time) string {
	return t.Format(time.RFC3339Nano)
}

func newConfig() (*config, error) {
	apiKey := getEnv("DD_API_KEY")
	appKey := getEnv("DD_APP_KEY")

	tmpl, err := template.New("logLine").Parse(*msgFormat + "\n")
	if err != nil {
		return nil, err
	}

	from := *fromStr
	to := *toStr

	if (from == "" && to != "") || (from != "" && to == "") {
		return nil, fmt.Errorf("both 'from' and 'to' must be set")
	}

	follow := false
	if from == "" && to == "" {
		follow = true
		// First attempt for the follow mode, retrieve logs from 30 seconds ago
		from = formatTime(time.Now().Add(time.Duration(-30 * time.Second)))
		to = formatTime(time.Now())
	}

	return &config{
		query:    *query,
		from:     from,
		to:       to,
		limit:    *limit,
		follow:   follow,
		tmpl:     tmpl,
		lastInfo: &logsInfo{},
		apiKey:   apiKey,
		appKey:   appKey,
	}, nil
}

func (cfg *config) update(info *logsInfo) error {
	cfg.lastInfo = info
	if info.NextLogId != "" {
		// There are remaining logs (keep last condition)
		return nil
	}

	if len(info.Logs) != 0 {
		cfg.from = info.Logs[len(info.Logs)-1].Content.Timestamp
	}
	cfg.to = formatTime(time.Now())

	return nil
}

func (cfg *config) Debug() {
	println(fmt.Sprintf("[%s] from:%s, to:%s, nextLogId:%s", time.Now().Format(time.RFC3339), cfg.from, cfg.to, cfg.lastInfo.NextLogId))
}
