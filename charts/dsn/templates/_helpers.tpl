{{/*
Expand the name of the chart.
*/}}
{{- define "dsn.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "dsn.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
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
{{- define "dsn.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "dsn.labels" -}}
helm.sh/chart: {{ include "dsn.chart" . }}
{{ include "dsn.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "dsn.selectorLabels" -}}
app.kubernetes.io/name: {{ include "dsn.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "dsn.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "dsn.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Return the appropriate apiVersion for ingress.
*/}}
{{- define "dsn.ingress.apiVersion" -}}
{{- if .Values.apiVersion }}
{{- .Values.apiVersion }}
{{- else if semverCompare ">=1.19-0" .Capabilities.KubeVersion.GitVersion }}
{{- print "networking.k8s.io/v1" }}
{{- else if semverCompare ">=1.14-0" .Capabilities.KubeVersion.GitVersion }}
{{- print "networking.k8s.io/v1beta1" }}
{{- else }}
{{- print "extensions/v1beta1" }}
{{- end }}
{{- end }}

{{/*
Return if ingress is stable.
*/}}
{{- define "dsn.ingress.isStable" -}}
{{- eq (include "dsn.ingress.apiVersion" .) "networking.k8s.io/v1" }}
{{- end }}

{{/*
Return the appropriate path type for ingress.
*/}}
{{- define "dsn.ingress.pathType" -}}
{{- if .Values.ingress.pathType }}
{{- .Values.ingress.pathType }}
{{- else if semverCompare ">=1.19-0" .Capabilities.KubeVersion.GitVersion }}
{{- print "Prefix" }}
{{- else }}
{{- print "ImplementationSpecific" }}
{{- end }}
{{- end }}