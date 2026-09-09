// Package templates renders the HTML metrics dashboard page.
package templates

import (
	"html/template"
	"io"

	"github.com/AVZotov/metrics/internal/model/dto"
)

const metricsPageHTML = `<!DOCTYPE html>
<html lang="en">
	<head>
		<meta charset="UTF-8"/>
		<meta name="viewport" content="width=device-width, initial-scale=1.0"/>
		<title>Document</title>
	</head>
	<body>
		<div>
			{{range .}}<p>{{.String}}</p>
			{{end}}
		</div>
	</body>
</html>
`

var metricsPage = template.Must(template.New("metrics-page").Parse(metricsPageHTML))

// MetricsPage renders the metrics dashboard page listing m to w.
func MetricsPage(w io.Writer, m []dto.MetricsDTO) error {
	return metricsPage.Execute(w, m)
}
