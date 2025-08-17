# Metrics Monitoring for RHDH Operator

The RHDH provides a `/metrics` endpoint on port `9464` that provides OpenTelemetry metrics about your Backstage application. This endpoint can be used to monitor your Backstage instance using OpenTelemetry and Grafana.

When deploying RHDH using the [RHDH Operator](https://github.com/janus-idp/operator), monitoring and logging for your RHDH instance can be configured using the following steps.

## Prerequisites

- Kubernetes 1.19+
- PV provisioner support in the underlying infrastructure
- RHDH Operator deployed in your cluster

## Metrics Monitoring

### Automatic ServiceMonitor Creation

**NEW**: Starting with operator version 1.8.0, the RHDH operator supports automatic creation of OpenShift `ServiceMonitor` resources when monitoring is enabled through the Custom Resource (CR) configuration.

The operator can automatically create and manage ServiceMonitor resources for your Backstage instances by setting the `spec.monitoring.enabled` field to `true` in your Backstage Custom Resource.

#### Enabling Automatic Monitoring

To enable automatic ServiceMonitor creation, configure your Backstage Custom Resource as follows:

```yaml
apiVersion: rhdh.redhat.com/v1alpha4
kind: Backstage
metadata:
  name: my-rhdh
  namespace: my-project
spec:
  # Enable automatic ServiceMonitor creation
  monitoring:
    enabled: true
  
  # Other configuration fields...
  application:
    appConfig:
      configMaps:
        - name: my-app-config
```

When `monitoring.enabled` is set to `true`, the operator will automatically:

1. **Create a ServiceMonitor resource** named `<backstage-name>-metrics` in the same namespace as your Backstage instance
2. **Configure the ServiceMonitor** to scrape metrics from the `/metrics` endpoint on port `9464`
3. **Set appropriate labels** for Prometheus discovery (`app.kubernetes.io/instance` and `app.kubernetes.io/name`)
4. **Manage the lifecycle** of the ServiceMonitor (create, update, delete) along with your Backstage instance

**Note**: The operator automatically configures the Backstage service to expose a port named `http-metrics` mapping to port `9464`, so the ServiceMonitor can correctly scrape the `/metrics` endpoint. No additional service configuration is required.

#### ServiceMonitor Configuration Details

The automatically created ServiceMonitor will have the following configuration:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: <backstage-name>-metrics
  namespace: <backstage-namespace>
  labels:
    app.kubernetes.io/instance: <backstage-name>
    app.kubernetes.io/name: backstage
spec:
  namespaceSelector:
    matchNames:
      - <backstage-namespace>
  selector:
    matchLabels:
      app.kubernetes.io/instance: <backstage-name>
      app.kubernetes.io/name: backstage
  endpoints:
  - port: http-metrics
    path: '/metrics'
```

#### Disabling Monitoring

To disable monitoring and remove the ServiceMonitor, either:

1. **Set `monitoring.enabled` to `false`**:
```yaml
apiVersion: rhdh.redhat.com/v1alpha4
kind: Backstage
metadata:
  name: my-rhdh
spec:
  monitoring:
    enabled: false
```

2. **Remove the monitoring section entirely** (defaults to disabled):
```yaml
apiVersion: rhdh.redhat.com/v1alpha4
kind: Backstage
metadata:
  name: my-rhdh
spec:
  # monitoring section omitted - defaults to disabled
  application:
    # other config...
```

When monitoring is disabled, the operator will automatically clean up any existing ServiceMonitor resources.

### Enabling Metrics Monitoring on OpenShift

To enable metrics monitoring on OpenShift, ensure you have enabled [monitoring for user-defined projects](https://docs.redhat.com/en/documentation/openshift_container_platform/latest/html/monitoring/configuring-user-workload-monitoring#preparing-to-configure-the-monitoring-stack-uwm) for the metrics to be ingested by the built-in Prometheus instances.

#### Complete Example

Here's a complete example of a Backstage Custom Resource with monitoring enabled:

```yaml
apiVersion: rhdh.redhat.com/v1alpha4
kind: Backstage
metadata:
  name: developer-hub
  namespace: backstage
spec:
  # Enable automatic ServiceMonitor creation
  monitoring:
    enabled: true
  
  application:
    appConfig:
      configMaps:
        - name: app-config
    extraEnvs:
      envs:
        - name: LOG_LEVEL
          value: info
    route:
      enabled: true
      host: developer-hub.apps.cluster.example.com
  
  database:
    enableLocalDb: true
```

#### Verification

You can verify that the ServiceMonitor has been created successfully using the following commands:

```bash
# Check if the ServiceMonitor was created
$ oc get servicemonitor -n <namespace>
NAME                           AGE
<backstage-name>-metrics      1m

# View the ServiceMonitor details
$ oc describe servicemonitor <backstage-name>-metrics -n <namespace>
```

You can then verify metrics are being captured by navigating to the OpenShift Console:
1. Go to **Developer** Mode
2. Change to the namespace where your Backstage instance is deployed
3. Select **Observe** and navigate to the **Metrics** tab
4. Create PromQL queries to query the metrics being captured by OpenTelemetry

![OpenShift Metrics](./images/openshift-metrics.png)

### Manual ServiceMonitor Creation (Legacy Method)

**Note**: With the automatic ServiceMonitor creation feature, manual creation is no longer necessary in most cases. However, you can still manually create ServiceMonitor resources if needed for custom configurations.

For manual ServiceMonitor creation, you can use the following template:

```bash
# Set your Custom Resource name and namespace
$ CR_NAME=my-rhdh
$ MY_PROJECT=my-project

$ cat <<EOF > /tmp/${CR_NAME}.ServiceMonitor.yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: ${CR_NAME}-metrics
  namespace: ${MY_PROJECT}
  labels:
    app.kubernetes.io/instance: ${CR_NAME}
    app.kubernetes.io/name: backstage
spec:
  namespaceSelector:
    matchNames:
      - ${MY_PROJECT}
  selector:
    matchLabels:
      app.kubernetes.io/instance: ${CR_NAME}
      app.kubernetes.io/name: backstage
  endpoints:
  - port: http-metrics
    path: '/metrics'
EOF

$ oc apply -f /tmp/${CR_NAME}.ServiceMonitor.yaml
```

### Enabling Metrics Monitoring on Azure Kubernetes Service (AKS)

To enable metrics monitoring for RHDH on Azure Kubernetes Service (AKS), you can use the [Azure Monitor managed service for Prometheus](https://learn.microsoft.com/en-us/azure/azure-monitor/essentials/prometheus-metrics-overview). The AKS cluster will need to have an associated [Azure Monitor workspace](https://learn.microsoft.com/en-us/azure/azure-monitor/containers/prometheus-metrics-enable?tabs=azure-portal).

For AKS deployments, you may need to add pod annotations for metrics scraping. You can configure this through the Backstage Custom Resource using the `deployment.patch` field:

```yaml
apiVersion: rhdh.redhat.com/v1alpha4
kind: Backstage
metadata:
  name: my-rhdh
spec:
  # Enable monitoring
  monitoring:
    enabled: true
  
  # Add pod annotations for AKS monitoring
  deployment:
    patch:
      spec:
        template:
          metadata:
            annotations:
              prometheus.io/scrape: 'true'
              prometheus.io/path: '/metrics'
              prometheus.io/port: '9464'
              prometheus.io/scheme: 'http'
```

## Troubleshooting

### ServiceMonitor Not Created

If the ServiceMonitor is not being created automatically:

1. **Check the monitoring configuration**:
   ```bash
   $ oc get backstage <backstage-name> -o yaml | grep -A 2 monitoring
   ```

2. **Verify the operator logs**:
   ```bash
   $ oc logs -n <operator-namespace> deployment/backstage-operator
   ```

3. **Check if the Prometheus Operator CRDs are installed**:
   ```bash
   $ oc get crd servicemonitors.monitoring.coreos.com
   ```

### Metrics Not Appearing in Prometheus

If metrics are not appearing in Prometheus:

1. **Verify the ServiceMonitor is targeting the correct service**:
   ```bash
   $ oc get servicemonitor <backstage-name>-metrics -o yaml
   $ oc get service -l app.kubernetes.io/instance=<backstage-name>
   ```

2. **Check Prometheus configuration** to ensure user workload monitoring is enabled

3. **Verify the Backstage pod is exposing metrics**:
   ```bash
   $ oc port-forward pod/<backstage-pod> 9464:9464
   $ curl http://localhost:9464/metrics
   ```

## Migration from Manual to Automatic ServiceMonitor

If you previously created ServiceMonitor resources manually and want to migrate to the automatic creation:

1. **Delete the existing manual ServiceMonitor**:
   ```bash
   $ oc delete servicemonitor <manual-servicemonitor-name>
   ```

2. **Update your Backstage Custom Resource** to enable automatic monitoring:
   ```yaml
   spec:
     monitoring:
       enabled: true
   ```

3. **Verify the new ServiceMonitor is created** by the operator with the naming convention `<backstage-name>-metrics`.

The operator-managed ServiceMonitor will have the same functionality as your manually created one, but will be automatically managed throughout the lifecycle of your Backstage instance.