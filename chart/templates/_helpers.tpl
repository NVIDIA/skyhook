{{/*
Expand the name of the chart.
*/}}
{{- define "chart.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Resource name used by Deployment, ServiceAccount, PDB, etc. (anything that
includes `chart.fullname`).

The conventional Helm `<release>-<chart>` prefix is intentionally dropped:
NodeWright is a singleton per namespace (cluster-scoped CRDs, webhook
configurations, finalizers), so a release-name prefix on resource names
adds no value and just makes them noisy. Truncated at 63 chars for the DNS
name limit. `fullnameOverride` is still honored for users who really do
need a custom name.
*/}}
{{- define "chart.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "chart.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "chart.labels" -}}
helm.sh/chart: {{ include "chart.chart" . }}
{{ include "chart.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "chart.selectorLabels" -}}
app.kubernetes.io/name: {{ include "chart.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Selector for the controller-manager Deployment.

Shared by the Deployment's spec.selector and the selector-migration pre-upgrade
hook so both reason about the identical label set. spec.selector is immutable,
so when the chart name or release name changes these labels (e.g. the
skyhook-operator -> nodewright rename), the hook deletes the stale Deployment
and lets helm recreate it rather than failing the upgrade.
*/}}
{{- define "chart.managerSelectorLabels" -}}
control-plane: controller-manager
{{ include "chart.selectorLabels" . }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "chart.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "chart.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}
