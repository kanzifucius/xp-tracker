{{/*
Expand the name of the chart. Defaults align with deploy/base resource names.
*/}}
{{- define "xp-tracker.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "xp-tracker.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else if .Values.nameOverride }}
{{- .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "xp-tracker.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels (includes deploy/base part-of label).
*/}}
{{- define "xp-tracker.labels" -}}
helm.sh/chart: {{ include "xp-tracker.chart" . }}
{{ include "xp-tracker.selectorLabels" . }}
app.kubernetes.io/component: exporter
app.kubernetes.io/part-of: crossplane-metrics-exporter
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "xp-tracker.selectorLabels" -}}
app.kubernetes.io/name: {{ include "xp-tracker.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
ServiceAccount name
*/}}
{{- define "xp-tracker.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "xp-tracker.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Pinned image reference. Refuses empty/:latest tags.
*/}}
{{- define "xp-tracker.image" -}}
{{- $tag := .Values.image.tag | default .Chart.AppVersion -}}
{{- if or (eq $tag "") (eq $tag "latest") }}
{{- fail "image.tag must be a pinned release tag (not empty or 'latest'); set image.tag or Chart.appVersion (e.g. v0.4.0)" }}
{{- end }}
{{- printf "%s:%s" .Values.image.repository $tag }}
{{- end }}
