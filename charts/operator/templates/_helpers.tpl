{{- define "workbenches-operator.name" -}}
workbenches-operator
{{- end }}

{{- define "workbenches-operator.fullname" -}}
{{ .Release.Name }}-workbenches-operator
{{- end }}

{{- define "workbenches-operator.labels" -}}
app.kubernetes.io/name: {{ include "workbenches-operator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
control-plane: controller-manager
{{- end }}

{{- define "workbenches-operator.selectorLabels" -}}
control-plane: controller-manager
app.kubernetes.io/name: {{ include "workbenches-operator.name" . }}
{{- end }}
