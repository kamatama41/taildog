package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"hash/fnv"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"reflect"
	"text/template"
	"time"

	"github.com/fatih/color"
)

var (
	query = flag.String("q", "",
		"Search query. See https://docs.datadoghq.com/logs/explorer/search/ for more details of query.")
	headerFormat = flag.String("h", "{{.Timestamp}} {{.Host}}[{{.Service}}]: ",
		"Header format of entries in Golang's template style.\n"+
			"You can use any field in the \"content\" of the response of the Log Query API.\n"+
			"https://docs.datadoghq.com/api/#get-a-list-of-logs\n")
	messageFormat = flag.String("m", "{{.Message}}{{json .Attributes}}",
		"Message format of entries in Golang's template style.\n"+
			"You can use any field in the \"content\" of the response of the Log Query API.\n"+
			"https://docs.datadoghq.com/api/#get-a-list-of-logs\n")
	interval   = flag.Int("i", 15, "Interval time in seconds until the next attempt.")
	limit      = flag.Int("l", 1000, "Number of logs fetched at once.")
	fromStr    = flag.String("from", "", "Minimum timestamp for requested logs. See https://docs.datadoghq.com/api/#get-a-list-of-logs for more details of its format.")
	toStr      = flag.String("to", "", "Maximum timestamp for requested logs. See https://docs.datadoghq.com/api/#get-a-list-of-logs for more details of its format.")
	versionFlg = flag.Bool("version", false, "Show version of taildog.")

	version   = "dev"
	colorList = []*color.Color{
		color.New(color.FgHiRed),
		color.New(color.FgHiGreen),
		color.New(color.FgHiYellow),
		color.New(color.FgHiBlue),
		color.New(color.FgHiMagenta),
		color.New(color.FgHiCyan),
	}
)

type config struct {
	query       string
	from        string
	to          string
	limit       int
	headerTmpl  *template.Template
	messageTmpl *template.Template
	logTmpl     *template.Template
	follow      bool
	lastInfo    *logsInfo
	apiKey      string
	appKey      string
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

type message struct {
	Header      string
	Message     string
	HeaderColor *color.Color
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
	logsInfo, err := getLogs(cfg)
	if err != nil {
		return nil, err
	}

	var logsToDisplay []message
FilterLoop:
	for _, l := range logsInfo.Logs {
		if cfg.lastInfo != nil {
			// Remove duplicate logs
			for _, lastLog := range cfg.lastInfo.Logs {
				if l.Id == lastLog.Id {
					continue FilterLoop
				}
			}
		}

		msg, err := newMessage(cfg, l)
		if err != nil {
			return nil, err
		}
		logsToDisplay = append(logsToDisplay, *msg)
	}

	for _, l := range logsToDisplay {
		err := cfg.logTmpl.Execute(os.Stdout, l)
		if err != nil {
			return nil, err
		}
	}

	return logsInfo, err
}

func getLogs(cfg *config) (*logsInfo, error) {
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

	return logsInfo, nil
}

func newMessage(cfg *config, l logInfo) (*message, error) {
	var hdr bytes.Buffer
	if err := cfg.headerTmpl.Execute(&hdr, l.Content); err != nil {
		return nil, err
	}

	var msg bytes.Buffer
	if err := cfg.messageTmpl.Execute(&msg, l.Content); err != nil {
		return nil, err
	}

	hash := fnv.New32()
	if _, err := hash.Write([]byte(l.Content.Host + l.Content.Service)); err != nil {
		return nil, err
	}
	headerColor := colorList[hash.Sum32()%uint32(len(colorList))]

	return &message{
		Header:      hdr.String(),
		Message:     msg.String(),
		HeaderColor: headerColor,
	}, nil
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

func parseTime(str string) (time.Time, error) {
	return time.Parse(time.RFC3339, str)
}

func newConfig() (*config, error) {
	apiKey := getEnv("DD_API_KEY")
	appKey := getEnv("DD_APP_KEY")

	funcs := map[string]interface{}{
		"json": func(in interface{}) (string, error) {
			if in == nil || reflect.ValueOf(in).IsNil() {
				return "", nil
			}
			b, err := json.Marshal(in)
			if err != nil {
				return "", err
			}
			return string(b), nil
		},
		"color": func(color color.Color, text string) string {
			return color.SprintFunc()(text)
		},
	}

	headerTmpl, err := template.New("logHeader").Funcs(funcs).Parse(*headerFormat)
	if err != nil {
		return nil, err
	}
	messageTmpl, err := template.New("logMessage").Funcs(funcs).Parse(*messageFormat)
	if err != nil {
		return nil, err
	}
	logTmpl, err := template.New("logContent").Funcs(funcs).Parse("{{color .HeaderColor .Header}}{{.Message}}\n")
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
		query:       *query,
		from:        from,
		to:          to,
		limit:       *limit,
		follow:      follow,
		headerTmpl:  headerTmpl,
		messageTmpl: messageTmpl,
		logTmpl:     logTmpl,
		lastInfo:    &logsInfo{},
		apiKey:      apiKey,
		appKey:      appKey,
	}, nil
}

func (cfg *config) update(info *logsInfo) error {
	cfg.lastInfo = info
	if info.NextLogId != "" {
		// There are remaining logs (keep last condition)
		return nil
	}

	if len(info.Logs) != 0 {
		lastTimestamp, err := parseTime(info.Logs[len(info.Logs)-1].Content.Timestamp)
		if err != nil {
			return err
		}
		cfg.from = formatTime(lastTimestamp.Add(time.Duration(-*interval/2) * time.Second))
	}
	cfg.to = formatTime(time.Now())

	return nil
}

func (cfg *config) Debug() {
	println(fmt.Sprintf("[%s] from:%s, to:%s, nextLogId:%s", time.Now().Format(time.RFC3339), cfg.from, cfg.to, cfg.lastInfo.NextLogId))
}
