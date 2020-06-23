package loki

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/gliderlabs/logspout/router"
	lokiclient "github.com/livepeer/loki-client/client"
	"github.com/livepeer/loki-client/model"
)

const glogTimeFormat = "20060102 15:04:05.999999"

var (
	levels       = []string{"info", "warning", "error", "fatal"}
	errShortLine = errors.New("Too short line")
	year         string
)

func init() {
	year = strconv.FormatInt(int64(time.Now().Year()), 10)
	router.AdapterFactories.Register(NewLokiAdapter, "loki")
}

// LokiAdapter is an adapter that streams logs to Loki.
type LokiAdapter struct {
	route  *router.Route
	client *lokiclient.Client
}

func logger(v ...interface{}) {
	fmt.Println(v...)
}

// NewLokiAdapter creates a LokiAdapter.
func NewLokiAdapter(route *router.Route) (router.LogAdapter, error) {
	baseLabels := model.LabelSet{}
	lokiURL := "http://" + route.Address + "/api/prom/push"
	fmt.Printf("Using Loki url: %s\n", lokiURL)
	client, err := lokiclient.NewWithDefaults(lokiURL, baseLabels, logger)
	if err != nil {
		return nil, err
	}
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt)
	go waitExit(client, c)

	return &LokiAdapter{
		route:  route,
		client: client,
	}, nil
}

// Stream implements the router.LogAdapter interface.
func (a *LokiAdapter) Stream(logstream chan *router.Message) {
	defer a.client.Stop()

	lastLineTime := time.Now()
	for m := range logstream {

		name := m.Container.Name[1:]
		np := strings.Split(name, ".")
		if len(np) == 3 && np[1] == "1" {
			// special case for services with one instance only
			name = np[0]
		}
		labels := model.LabelSet{"name": name, "host": m.Container.Config.Hostname}
		line := strings.TrimSpace(m.Data)
		if len(line) > 0 {
			ts, level, file, caller, message, err := parseLine(line)
			if err != nil {
				fmt.Printf("ERROR PARSING LOG LINE: %v\n", err)
				fmt.Printf("Original line:`%s`\n", line)
				a.client.Handle(labels, lastLineTime, line)
			} else if len(message) > 0 {
				lastLineTime = ts
				labels := labels.Merge(model.LabelSet{"level": level, "file": file})
				f := "level=" + level + " caller=" + caller + ` msg="` + strings.Replace(message, `"`, `\"`, -1) + `"`
				a.client.Handle(labels, ts, f)
			}
		}
	}
}

func waitExit(client *lokiclient.Client, c chan os.Signal) {
	<-c
	client.Stop()
}

func parseLine(line string) (time.Time, string, string, string, string, error) {
	var t time.Time
	var level = "unknown"
	var file, caller, message string
	if len(line) < 31 {
		return t, level, file, caller, message, errShortLine
	}
	switch line[0] {
	case 'I':
		level = levels[0]
	case 'W':
		level = levels[1]
	case 'E':
		level = levels[2]
	case 'F':
		level = levels[3]
	}
	t, err := time.ParseInLocation(glogTimeFormat, year+line[1:21], time.Local)
	if err != nil {
		return t, level, file, caller, message, err
	}
	ll := line[30:]
	file = ll[:strings.IndexByte(ll, ':')]
	bi := strings.IndexByte(ll, ']')
	if bi+3 >= len(ll) {
		return t, level, file, caller, message, errShortLine
	}
	caller = ll[:bi]
	message = ll[bi+2:]
	if message[len(message)-1] == '\n' {
		message = message[:len(message)-1]
	}

	return t, level, file, caller, message, nil
}
