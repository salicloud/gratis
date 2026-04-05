{{/*
Expand the name of the chart.
*/}}
{{- define "gratis.name" -}}
{{- .Chart.Name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "gratis.fullname" -}}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels applied to all resources.
*/}}
{{- define "gratis.labels" -}}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Selector labels for the API.
*/}}
{{- define "gratis.api.selectorLabels" -}}
app.kubernetes.io/name: {{ include "gratis.name" . }}-api
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Selector labels for the web frontend.
*/}}
{{- define "gratis.web.selectorLabels" -}}
app.kubernetes.io/name: {{ include "gratis.name" . }}-web
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
