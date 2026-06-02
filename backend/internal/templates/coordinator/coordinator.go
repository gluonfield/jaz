package coordinator

import (
	"bytes"
	_ "embed"
	"text/template"
	"time"
)

//go:embed coordinator.tmpl
var coordinatorTemplate string

var tmpl = template.Must(template.New("coordinator").Parse(coordinatorTemplate))

type Section struct {
	Name    string
	Content string
}

type templateData struct {
	Date     string
	Time     string
	Timezone string
	Weekday  string
	Human    string
	Sections []Section
	Skills   string
}

func Render(now time.Time, sections []Section, skills string) (string, error) {
	var out bytes.Buffer
	err := tmpl.Execute(&out, templateData{
		Date:     now.Format("January 2, 2006"),
		Time:     now.Format("15:04:05 MST"),
		Timezone: now.Format("MST (UTCZ07:00)"),
		Weekday:  now.Format("Monday"),
		Human:    now.Format("Monday, January 2, 2006 at 15:04:05 MST"),
		Sections: sections,
		Skills:   skills,
	})
	return out.String(), err
}
