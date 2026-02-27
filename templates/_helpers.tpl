{{/*
Expand the name of the chart.
*/}}
{{- define "cloud-price-exporter.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "cloud-price-exporter.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "cloud-price-exporter.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Common labels.
*/}}
{{- define "cloud-price-exporter.labels" -}}
helm.sh/chart: {{ include "cloud-price-exporter.chart" . }}
{{ include "cloud-price-exporter.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{/*
Selector labels.
*/}}
{{- define "cloud-price-exporter.selectorLabels" -}}
app.kubernetes.io/name: {{ include "cloud-price-exporter.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
Create the name of the service account to use.
*/}}
{{- define "cloud-price-exporter.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "cloud-price-exporter.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{/*
CLI arguments for the exporter binary.
Renders one flag per line (no leading dash prefix duplication).
Used by deployment.yaml for both direct args and sidecarWait shell wrapper.
*/}}
{{- define "cloud-price-exporter.cliArgs" -}}
-listen-address=:{{ .Values.service.port }}
-log-level={{ .Values.exporter.logLevel }}
-cache={{ .Values.exporter.cache }}
{{- if .Values.exporter.instanceRegexes }}
-instance-regexes={{ .Values.exporter.instanceRegexes }}
{{- end }}
-aws-enabled={{ .Values.exporter.aws.enabled }}
{{- if .Values.exporter.aws.enabled }}
-lifecycle={{ .Values.exporter.aws.lifecycle }}
-product-descriptions={{ .Values.exporter.aws.productDescriptions }}
-operating-systems={{ .Values.exporter.aws.operatingSystems }}
{{- if .Values.exporter.aws.regions }}
-regions={{ .Values.exporter.aws.regions }}
{{- end }}
{{- if .Values.exporter.aws.savingPlanTypes }}
-saving-plan-types={{ .Values.exporter.aws.savingPlanTypes }}
{{- end }}
{{- end }}
-azure-enabled={{ .Values.exporter.azure.enabled }}
{{- if .Values.exporter.azure.enabled }}
{{- if .Values.exporter.azure.regions }}
-azure-regions={{ .Values.exporter.azure.regions }}
{{- end }}
-azure-operating-systems={{ .Values.exporter.azure.operatingSystems }}
{{- if .Values.exporter.azure.instanceRegexes }}
-azure-instance-regexes={{ .Values.exporter.azure.instanceRegexes }}
{{- end }}
{{- end }}
{{- end -}}
