{{/* Common labels applied to every object. */}}
{{- define "mediaforge.labels" -}}
app.kubernetes.io/part-of: mediaforge
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
{{- end -}}

{{/* Fully-qualified image reference for a service image name. */}}
{{- define "mediaforge.image" -}}
{{- $root := .root -}}
{{- printf "%s/%s:%s" $root.Values.image.registry .image $root.Values.image.tag -}}
{{- end -}}
