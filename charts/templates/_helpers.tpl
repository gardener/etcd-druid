{{- define "operator.config.data" -}}
config.yaml: |
  ---
  apiVersion: config.druid.gardener.cloud/v1alpha1
  kind: OperatorConfiguration
  clientConnection:
    qps: {{ .Values.operatorConfig.clientConnection.qps }}
    burst: {{ .Values.operatorConfig.clientConnection.burst }}
    {{- if hasKey .Values.operatorConfig.clientConnection "contentType" }}
    contentType: {{ .Values.operatorConfig.clientConnection.contentType }}
    {{- end }}
    {{- if hasKey .Values.operatorConfig.clientConnection "acceptContentType" }}
    acceptContentType: {{ .Values.operatorConfig.clientConnection.acceptContentType }}
    {{- end }}
  leaderElection:
    enabled: {{ .Values.operatorConfig.leaderElection.enabled }}
    leaseDuration: {{ .Values.operatorConfig.leaderElection.leaseDuration }}
    renewDeadline: {{ .Values.operatorConfig.leaderElection.renewDeadline }}
    retryPeriod: {{ .Values.operatorConfig.leaderElection.retryPeriod }}
    resourceLock: {{ .Values.operatorConfig.leaderElection.resourceLock }}
    resourceName: {{ .Values.operatorConfig.leaderElection.resourceName }}
  server:
    webhooks:
      port: {{ .Values.operatorConfig.server.webhooks.port }}
      {{- if hasKey .Values.operatorConfig.server.webhooks "bindAddress" }}
      bindAddress: {{ .Values.operatorConfig.server.webhooks.bindAddress }}
      {{- end }}
      {{- if hasKey .Values.operatorConfig.server.webhooks "serverCertDir" }}
      serverCertDir: {{ .Values.operatorConfig.server.webhooks.serverCertDir }}
      {{- end }}
    metrics:
      port: {{ .Values.operatorConfig.server.metrics.port }}
      {{- if hasKey .Values.operatorConfig.server.metrics "bindAddress" }}
      bindAddress: {{ .Values.operatorConfig.server.metrics.bindAddress }}
      {{- end }}
  controllers:
    etcd:
      concurrentSyncs: {{ .Values.operatorConfig.controllers.etcd.concurrentSyncs }}
      enableEtcdSpecAutoReconcile: {{ .Values.operatorConfig.controllers.etcd.enableEtcdSpecAutoReconcile }}
      disableEtcdServiceAccountAutomount: {{ .Values.operatorConfig.controllers.etcd.disableEtcdServiceAccountAutomount }}
      etcdStatusSyncPeriod: {{ .Values.operatorConfig.controllers.etcd.etcdStatusSyncPeriod }}
      etcdMember:
        notReadyThreshold: {{ .Values.operatorConfig.controllers.etcd.etcdMember.notReadyThreshold }}
        unknownThreshold: {{ .Values.operatorConfig.controllers.etcd.etcdMember.unknownThreshold }}
    compaction:
      enabled: {{ .Values.operatorConfig.controllers.compaction.enabled }}
      concurrentSyncs: {{ .Values.operatorConfig.controllers.compaction.concurrentSyncs }}
      eventsThreshold: {{ .Values.operatorConfig.controllers.compaction.eventsThreshold }}
      triggerFullSnapshotThreshold: {{ .Values.operatorConfig.controllers.compaction.triggerFullSnapshotThreshold }}
      activeDeadlineDuration: {{ .Values.operatorConfig.controllers.compaction.activeDeadlineDuration }}
      metricsScrapeWaitDuration: {{ .Values.operatorConfig.controllers.compaction.metricsScrapeWaitDuration }}
    etcdCopyBackupsTask:
      enabled: {{ .Values.operatorConfig.controllers.etcdCopyBackupsTask.enabled }}
      concurrentSyncs: {{ .Values.operatorConfig.controllers.etcdCopyBackupsTask.concurrentSyncs }}
    secret:
      concurrentSyncs: {{ .Values.operatorConfig.controllers.secret.concurrentSyncs }}
  webhooks:
    etcdComponentProtection:
      enabled: {{ .Values.operatorConfig.webhooks.etcdComponentProtection.enabled }}
      serviceAccountInfo:
        name: {{ .Values.serviceAccount.name }}
        namespace: {{ .Release.Namespace }}
      exemptServiceAccounts:
      {{- toYaml .Values.operatorConfig.webhooks.etcdComponentProtection.exemptServiceAccounts | nindent 8}}
{{- with .Values.operatorConfig.featureGates }}
  featureGates:
  {{- toYaml . | nindent 4 }}
{{- end }}
  logConfiguration:
    logLevel: {{ .Values.operatorConfig.logConfiguration.logLevel }}
    logFormat: {{ .Values.operatorConfig.logConfiguration.logFormat }}
{{- end -}}

{{- define "operator.config.name" -}}
etcd-druid-operator-configmap-{{ include "operator.config.data" . | sha256sum | trunc 8 }}
{{- end -}}

{{- define "webhook.etcdcomponentprotection.enabled" -}}
{{- $webhookEnabled := false -}}
{{- if .Values.enabledOperatorConfig -}}
{{- $webhookEnabled = .Values.operatorConfig.webhooks.etcdComponentProtection.enabled -}}
{{- else -}}
{{- $webhookEnabled = .Values.webhooks.etcdComponentProtection.enabled -}}
{{- end -}}
{{- $webhookEnabled | toString -}}
{{- end -}}

{{- define "webhook.etcdcomponentprotection.reconcilerServiceAccountFQDN" -}}
{{- printf "system:serviceaccount:%s:%s" .Release.Namespace .Values.serviceAccount.name }}
{{- end -}}

{{ define "operator.service.ports" -}}
{{- $metricsPort := "" }}
{{- $webhooksPort := "" }}
{{- if .Values.enabledOperatorConfig }}
{{- $metricsPort = .Values.operatorConfig.server.metrics.port }}
{{- $webhooksPort = .Values.operatorConfig.server.webhooks.port }}
{{- else }}
{{- $metricsPort = .Values.controllerManager.server.metrics.port }}
{{- $webhooksPort = .Values.controllerManager.server.webhook.port }}
{{- end }}
{{- if $metricsPort }}
- name: metrics
  port: {{ $metricsPort }}
  protocol: TCP
  targetPort: {{ $metricsPort }}
{{- end }}
{{- if $webhooksPort }}
- name: webhooks
  port: {{ $webhooksPort }}
  protocol: TCP
  targetPort: {{ $webhooksPort }}
{{- end }}
{{- end -}}