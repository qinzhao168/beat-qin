package add_kubernetes_metadata

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/beats/libbeat/processors/add_kubernetes_metadata"
)

func init() {
	add_kubernetes_metadata.Indexing.AddMatcher(LogPathMatcherName, newLogsPathMatcher)
	cfg := common.NewConfig()

	//Add a container indexer config by default.
	add_kubernetes_metadata.Indexing.AddDefaultIndexerConfig(add_kubernetes_metadata.ContainerIndexerName, *cfg)

	//Add a log path matcher which can extract container ID from the "source" field.
	add_kubernetes_metadata.Indexing.AddDefaultMatcherConfig(LogPathMatcherName, *cfg)
}

const LogPathMatcherName = "logs_path"
const regexMatch = "(.*)-(.{64})\\.log.(.*)"

type LogPathMatcher struct {
	LogsPath string
	reg      *regexp.Regexp
}

func newLogsPathMatcher(cfg common.Config) (add_kubernetes_metadata.Matcher, error) {
	config := struct {
		LogsPath string `config:"logs_path"`
	}{
		LogsPath: "/var/lib/docker/containers/",
	}

	err := cfg.Unpack(&config)
	if err != nil || config.LogsPath == "" {
		return nil, fmt.Errorf("fail to unpack the `logs_path` configuration: %s", err)
	}

	logPath := config.LogsPath
	if logPath[len(logPath)-1:] != "/" {
		logPath = logPath + "/"
	}

	return &LogPathMatcher{LogsPath: logPath, reg: regexp.MustCompile(regexMatch)}, nil
}

func (f *LogPathMatcher) MetadataIndex(event common.MapStr) string {
	if value, ok := event["source"]; ok {
		source := value.(string)
		logp.Debug("kubernetes", "Incoming source value: ", source)
		cid := ""
		if strings.Contains(source, f.LogsPath) {
			//Docker container is 64 chars in length
			cid = source[len(f.LogsPath) : len(f.LogsPath)+64]
		} else if strings.Contains(source, "/var/log/containers/applogs/") {
			matcher := f.reg.FindAllStringSubmatch(source, -1)
			if len(matcher) == 1 {
				if len(matcher[0]) == 4 {
					cid = matcher[0][2]
					event.Put("filepath", matcher[0][3])
				}
			}
		}
		logp.Debug("kubernetes", "Using container id: ", cid)

		if cid != "" {
			return cid
		}
	}

	return ""
}
