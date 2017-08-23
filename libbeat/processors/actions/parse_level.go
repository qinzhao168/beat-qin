package actions

import (
	"fmt"
	"strings"

	"github.com/elastic/beats/libbeat/beat"
	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/processors"
)

type parseLevel struct {
	Levels []string
	Field  string
}

func init() {
	processors.RegisterPlugin("parse_level",
		configChecked(newParseLevel,
			requireFields("levels"),
			requireFields("field")))
}

func newParseLevel(c *common.Config) (processors.Processor, error) {
	config := struct {
		Levels []string `config:"levels"`
		Field  string   `config:"field"`
	}{}
	err := c.Unpack(&config)
	if err != nil {
		return nil, fmt.Errorf("fail to unpack the parse_level configuration: %s", err)
	}

	f := &parseLevel{Levels: config.Levels, Field: config.Field}
	return f, nil
}

func (f *parseLevel) Run(event *beat.Event) (*beat.Event, error) {
	value, err := event.GetValue(f.Field)
	if err != nil {
		return event, nil
	}

	lIndex := -1
	levelStr := ""
	for _, level := range f.Levels {
		index := strings.Index(strings.ToLower(fmt.Sprint(value)), strings.ToLower(level))
		if index == -1 {
			continue
		}
		if lIndex == -1 {
			lIndex = index
			levelStr = level
			continue
		}
		if index < lIndex {
			lIndex = index
			levelStr = level
		}
	}
	event.PutValue("level", levelStr)

	return event, nil
}

func (f *parseLevel) String() string {
	return "parse_level=" + strings.Join(f.Levels, ", ")
}
