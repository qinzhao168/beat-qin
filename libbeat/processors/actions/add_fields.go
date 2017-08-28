package actions

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/elastic/beats/libbeat/beat"
	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/processors"
)

type addFields struct {
	Fields map[string]string
	reg    *regexp.Regexp
}

func init() {
	processors.RegisterPlugin("add_fields",
		configChecked(newAddFields,
			requireFields("fields"),
			allowedFields("fields", "when")))
}

func newAddFields(c *common.Config) (processors.Processor, error) {
	config := struct {
		Fields map[string]string `config:"fields"`
	}{}
	err := c.Unpack(&config)
	if err != nil {
		return nil, fmt.Errorf("fail to unpack the add_fields configuration: %s", err)
	}

	f := &addFields{Fields: config.Fields, reg: regexp.MustCompile("{(.*)}")}
	return f, nil
}

func (f *addFields) Run(event *beat.Event) (*beat.Event, error) {
	var errors []string
	for field, value := range f.Fields {
		matchers := f.reg.FindAllStringSubmatch(value, -1)
		if len(matchers) == 0 {
			event.PutValue(field, value)
		} else {
			if len(matchers[0]) >= 2 {
				val, err := event.GetValue(strings.Trim(matchers[0][1], " "))
				if err != nil {
					errors = append(errors, err.Error())
				} else {
					event.PutValue(field, val)
				}
			}
		}
	}
	return event, nil
}

func (f *addFields) String() string {
	var fields []string
	for field, _ := range f.Fields {
		fields = append(fields, field)
	}
	return "add_fields=" + strings.Join(fields, ", ")
}
