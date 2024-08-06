# Backstage Operator

## The Goal
The Goal of [Backstage](https://backstage.io) Operator project is creating Kubernetes Operator for configuring, installing and synchronizing Backstage instance on Kubernetes/OpenShift. 
The initial target is in support of Red Hat's assemblies of Backstage - specifically supporting [dynamic-plugins](https://github.com/janus-idp/backstage-showcase/blob/main/showcase-docs/dynamic-plugins.md) on OpenShift. This includes [Red Hat Developer Hub (RHDH)](https://developers.redhat.com/rhdh) but may be flexible enough to install any compatible Backstage instance on Kubernetes. See additional information under [Configuration](docs/configuration.md).
The Operator provides clear and flexible configuration options to satisfy a wide range of expectations, from "no  configuration for default quick start" to "highly customized configuration for production".

[More documentation...](#more-documentation)

## Getting Started
You’ll need a Kubernetes or OpenShift cluster. You can use [Minikube](https://minikube.sigs.k8s.io/docs/) or [KIND](https://sigs.k8s.io/kind) for local testing, or deploy to a remote cluster.
**Note:** Your controller will automatically use the current context in your kubeconfig file (i.e. whatever cluster `kubectl cluster-info` shows).

To test it on minikube from the source code:

Both **kubectl** and **minikube** must be installed. See [tools](https://kubernetes.io/docs/tasks/tools/).

1.  Get your copy of Operator from GitHub: 
```sh
git clone https://github.com/redhat-developer/rhdh-operator
```
2. Deploy Operator on the minikube cluster:
```sh
cd <your-rhdh-operator-project-dir>
make deploy
```
you can check if the Operator pod is up by running 
```sh
kubectl get pods -n backstage-system
It should be something like:
NAME                                           READY   STATUS    RESTARTS   AGE
backstage-controller-manager-cfc44bdfd-xzk8g   2/2     Running   0          32s
```
3. Create Backstage Custom resource on some namespace (make sure this namespace exists)
```sh
kubectl -n <your-namespace> apply -f examples/bs1.yaml
```
you can check if the Operand pods are up by running
```sh
kubectl get pods -n <your-namespace>
It should be something like:
NAME                             READY   STATUS    RESTARTS      AGE
backstage-bs1-6c98c4dd6d-9wbgg   1/1     Running   3 (34m ago)   34m
backstage-psql-bs1-0             1/1     Running   0             34m

```

5. Port forward Backstage pod/port (destination port is **17007** in this example) and get URL for access Backstage.
Backstage-pod-name has a following pattern: **backstage-<name-of-CR>-<random-sequence>**
Taking previous item, your backstage-pod-name would be `backstage-bs1-6c98c4dd6d-9wbgg`

```sh
minikube service -n <your-namespace> backstage --url
kubectl port-forward -n <your-namespace> <backstage-pod-name> 17007:7007

Output:
Forwarding from 127.0.0.1:17007 -> 7007
Forwarding from [::1]:17007 -> 7007
```
5. Access your Backstage instance in a browser using http://localhost:17007

## More documentation

- [Openshift deployment](docs/openshift.md)
- [Configuration](docs/configuration.md)
- [Developer Guide](docs/developer.md)
- [Operator Design](docs/developer.md)

## Telemetry data collection

The telemetry data collection feature is enabled by default. The default configuration uses image with backstage-plugin-analytics-provider-segment plugin enabled and configured. To disable this and to learn what data is being collected, see https://github.com/janus-idp/backstage-showcase/blob/main/showcase-docs/getting-started.md#telemetry-collection 

## Contributing and reporting issues

To report issues against this chart, please use JIRA: https://issues.redhat.com/browse/RHIDP

Contributions are welcome! 

## License

Copyright 2023 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

