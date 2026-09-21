# Administrator Guide

This guide is intended for the Backstage Operator administrator who:

* Possesses sufficient knowledge and rights to configure Kubernetes clusters and cluster-scoped objects.
* Has acquired enough understanding to configure and support the Backstage Operator (with the assistance of this document).
* Is not necessarily an expert in Backstage functionality and configuration.

## Default Backstage instance configuration

The Backstage Operator operates at the cluster level, enabling management of multiple Backstage instances (Custom Resources).

Each Backstage Custom Resource (CR) governs the creation, modification, and deletion of a set of Kubernetes objects.

The default shape of these objects is configured at the Operator level using YAML files containing Kubernetes manifests.

Default Configuration is implemented as a ConfigMap named `backstage-default-config`, deployed within the Kubernetes namespace where the operator is installed (usually `backstage-system` or `rhdh-operator`). This ConfigMap is mounted to the `/default-config` directory of the Backstage controller container.

See [Configuration](configuration.md) -> Default Configuration for more details.

### Operator Bundle configuration 

With Backstage Operator's Makefile you can generate bundle descriptor using *make bundle* command

Along with CSV manifest it generates default-config ConfigMap manifest, which can be modified and applied to Backstage Operator.

[//]: # (TODO: document how an administrator can make changes to the default operator configuration, using their own configuration file (perhaps based on the generated one), and apply it using `kubectl` or `oc`.

### Kustomize deploy configuration

Make sure use the current context in your kubeconfig file is pointed to correct place, change necessary part of your config/manager/default-config or just replace some of the file(s) with yours and run
``
make deploy
``

### Direct ConfigMap configuration

You can change default configuration by directly changing the default-config ConfigMap with kubectl like:

 - retrieve the current `default-config` from the cluster

``
kubectl get -n backstage-system configmap default-config > my-config.yaml
``

- modify the file in your editor of choice

- apply the updated configuration to your cluster

``
  kubectl apply -n backstage-system -f my-config.yaml
``

It has to be re-applied to the controller's container after being reconciled by kubernetes processes.

### Recommended Namespace for Operator Installation
It is recommended to deploy the Backstage Operator in a dedicated default namespace `backstage-system`. The cluster administrator can restrict access to the operator resources through RoleBindings or ClusterRoleBindings. On OpenShift, you can choose to deploy the operator in the `openshift-operators` namespace instead. However, you should keep in mind that the Backstage Operator shares the namespace with other operators and therefore any users who can create workloads in that namespace can get their privileges escalated from all operators' service accounts.

### Use Cases

#### Airgapped environment

When creating the Backstage CR, the Operator will try to create a Backstage Pod, deploying:
- Backstage Container from the image, configured in *(deployment.yaml).spec.template.spec.Containers[].image*
- Init Container (applied for RHDH configuration, usually the same as Backstage Container)

Also, if Backstage CR configured with *EnabledLocalDb*,  it will create a PostgreSQL container pod, configured in *(db-statefulset.yaml).spec.template.spec.Containers[].image*

By default, the Backstage Operator is configured to use publicly available images.
If you plan to deploy to a [restricted environment](https://docs.openshift.com/container-platform/4.14/operators/admin/olm-restricted-networks.html),
you will need to configure your cluster or network to allow these images to be pulled.
For the list of related images deployed by the Operator, see the `RELATED_IMAGE_*` env vars or `relatedImages` section of the [CSV](../bundle/manifests/backstage-operator.clusterserviceversion.yaml).
See also https://docs.openshift.com/container-platform/4.14/operators/admin/olm-restricted-networks.html

#### Plugin Registry Mirroring

In air-gapped environments, Backstage dynamic plugins distributed as OCI artifacts need to be mirrored to internal registries. The Backstage Operator supports optional plugin registry mirroring that automatically transforms plugin package URLs to use your internal mirror registries.

The mirror configuration uses the same format and matching rules as OpenShift's [ImageDigestMirrorSet (IDMS)](https://docs.openshift.com/container-platform/4.14/openshift_images/image-configuration.html#images-configuration-registry-mirror_image-configuration), making it familiar to OpenShift administrators and allowing direct reuse of existing IDMS configurations.

**How it works:**

- The operator reads mirror configuration from an optional ConfigMap mounted at `/plugins-mirror/mirrors.yaml`
- Plugin package URLs with `oci://` prefix are transformed using IDMS-style matching rules before being written to the init container's package list
- Only OCI URLs are transformed - npm packages, HTTP URLs, and file paths remain unchanged
- The transformed URLs are visible in the Backstage CR status under `status.plugins`

**Configuration:**

1. Create a ConfigMap named `plugin-registry-mirror` in the operator's namespace (usually `backstage-system` or `rhdh-operator`):

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: plugin-registry-mirror
  namespace: rhdh-operator  # Must be in operator namespace
data:
  mirrors.yaml: |
    # Same structure as OpenShift ImageDigestMirrorSet
    imageDigestMirrors:
    # Mirror quay.io to internal registry
    - source: quay.io
      mirrors:
      - my-registry.example.com/quay-mirror

    # Mirror Red Hat registry
    - source: registry.redhat.io
      mirrors:
      - my-registry.example.com/redhat-mirror

    # Multiple mirrors (first is used, others ignored)
    - source: ghcr.io
      mirrors:
      - primary-mirror.example.com/ghcr
      - backup-mirror.example.com/ghcr
```

2. Restart the operator pod to load the configuration:

```bash
kubectl rollout restart deployment rhdh-operator -n rhdh-operator
```

3. Verify the transformed URLs in your Backstage CR status:

```bash
kubectl get backstage my-backstage -o jsonpath='{.status.plugins}' | jq
```

**Mirroring rules (same as IDMS):**

- **Most specific namespace match**: When multiple sources match a plugin URL, the longest/most specific source wins. For example, if you have both `quay.io` and `quay.io/rhdh` configured, a plugin from `oci://quay.io/rhdh/plugin:1.0` will use the `quay.io/rhdh` mirror configuration.
- **Only OCI URLs are mirrored**: Plugins specified as `oci://registry.example.com/plugin:tag` are transformed. npm packages (`@scope/package`), HTTP URLs (`https://...`), and file paths are not affected.
- **First mirror is used**: If multiple mirrors are specified for a source, only the first one is used (fallback support may be added in future versions).

**For OpenShift users with existing IDMS:**

You can directly copy your existing ImageDigestMirrorSet configuration to the ConfigMap format:

```bash
# Extract IDMS config
kubectl get imagedigestmirrorset rhdh-plugins -o jsonpath='{.spec.imageDigestMirrors}' | \
  yq -P 'imageDigestMirrors: .' > mirrors.yaml

# Create ConfigMap from extracted config
kubectl create configmap plugin-registry-mirror \
  --from-file=mirrors.yaml \
  -n rhdh-operator
```

**Important notes:**

- The ConfigMap must be created **before** the operator starts, or the operator must be restarted after creating/updating the ConfigMap
- Configuration changes require operator pod restart (acceptable for air-gap scenarios where mirrors rarely change)
- If the ConfigMap doesn't exist, no mirroring is applied (default behavior)
- The operator deployment already includes the volume mount configuration with `optional: true`, so the operator will start successfully whether or not the ConfigMap exists


### Installing Operator on Openshift cluster
https://docs.openshift.com/container-platform/4.15/operators/admin/olm-adding-operators-to-cluster.html 

## Memory Optimization for Large Clusters

The Backstage Operator watches Secrets and ConfigMaps to detect changes to external configuration referenced by Backstage CRs. While the controller uses predicates to filter which resources trigger reconciliation events, the underlying controller-runtime cache loads metadata for ALL Secrets and ConfigMaps in the cluster regardless of labels. On large clusters with tens of thousands of Secrets/ConfigMaps, this metadata-only cache can consume significant memory, even though only a small fraction of these resources are actually used by Backstage instances.
To address this, the Backstage Operator supports an optional cache-level label filtering mechanism. 
This allows the operator to only cache Secrets and ConfigMaps that are explicitly labeled for use with Backstage, significantly reducing memory consumption on clusters with many unused resources.

**NOTE: Enabling cache label filtering is a breaking change. Once enabled, any Secret or ConfigMap that should be visible to the operator must be labeled accordingly. Existing Backstage instances that reference unlabeled resources will fail to function correctly until those resources are labeled.**

#### Configuration

Cache-level label filtering can be enabled through:
* `ENABLE_CACHE_LABEL_FILTER=true` environment variable
* `--enable-cache-label-filter` command-line flag.

When enabled, the manager's cache is configured to only store Secrets and ConfigMaps that have the label `rhdh.redhat.com/external-config=true`. This happens at the cache initialization level, preventing unwanted resources from ever being loaded into memory, rather than just filtering events after caching.

**For OLM-based deployments:**

**Option 1:**
Edit the Subscription to add the environment variable:

```yaml
spec:
  config:
    env:
      - name: "ENABLE_CACHE_LABEL_FILTER"
        value: "true"
```

**Option 2:**
Edit the CSV to add the argument:
```yaml
spec:
  template:
    spec:
      containers:
      - name: manager
        args:
        - --enable-cache-label-filter
```

**For direct deployments (kustomize):**

Edit `config/manager/manager.yaml`:
```yaml
spec:
  template:
    spec:
      containers:
      - name: manager
        args:
        - --enable-cache-label-filter
```

Then deploy:
```bash
make deploy
```

**For local testing:**

```bash
make run ARGS="--enable-cache-label-filter"
```

#### Labeling Resources

When cache label filtering is enabled, you must label any Secret or ConfigMap that should be visible to the operator. In particular all the ConfigMaps and Secrets referenced in a Backstage CR spec.application, such as those used for app-config, external configuration, database credentials, etc should be labeled:

```yaml
metadata:
  name: my-backstage-secret
  namespace: my-namespace
  labels:
    rhdh.redhat.com/external-config: "true"
...
```

## Resource Deletion Policy

When the Backstage CR configuration changes in a way that makes certain resources no longer needed (for example, switching from local database to external database), the operator **does not automatically delete** those orphaned resources.

This is by design: automatic deletion of resources could lead to unexpected data loss. For example, deleting a local PostgreSQL PersistentVolumeClaim would permanently destroy all Backstage data stored in that database.

**Users are responsible for manually cleaning up resources they no longer need.**

To identify resources created by the operator for a specific Backstage instance, look for resources with matching labels in the same namespace:

```bash
oc get all,configmap,pvc,secret -l app.kubernetes.io/name=backstage,app.kubernetes.io/instance=<cr-name> -n <namespace>
```

This command queries multiple resource types at once: `all` covers common resources (Pods, Services, Deployments, StatefulSets), while `configmap`, `pvc` and `secret` are added explicitly as they're not included in `all`.

**Note:** The local PostgreSQL PVC (created via StatefulSet volumeClaimTemplates) uses different labels and is not included in the above query. To find it:

```bash
oc get pvc -n <namespace> | grep backstage-psql-<cr-name>
```

Review carefully before deleting, especially PersistentVolumeClaims which contain data.

## Instance Idling

The Operator supports idling and waking Backstage instances via the `rhdh.redhat.com/idle` annotation on the Backstage CR. When set to `"true"`, the Operator scales all managed workloads (Backstage Deployment or StatefulSet, and the local DB StatefulSet if enabled) to zero replicas in the same namespace as the CR.

When the annotation is removed, the next reconciliation restores replicas to their normal values.

When the local DB is disabled (`spec.database.enableLocalDb: false`), only the Backstage Deployment is affected.

### Idling an instance

```bash
kubectl annotate backstage <cr-name> rhdh.redhat.com/idle=true
```

After reconciliation, the status condition will show:

```yaml
- type: Deployed
  status: "True"
  reason: Deployed
  message: "0/0 replicas ready (Idled)"
```

> **Note:** To determine if an instance is idled, check the `rhdh.redhat.com/idle` annotation on the Backstage CR. Absence of this annotation means the instance is not idled.

### Waking an instance

```bash
kubectl annotate backstage <cr-name> rhdh.redhat.com/idle-
```

The status condition transitions back to its normal deployed state.