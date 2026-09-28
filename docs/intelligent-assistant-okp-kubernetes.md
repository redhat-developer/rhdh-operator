# Enabling OKP on Kubernetes

The `intelligent-assistant-okp` flavour creates the OKP Deployment and Service in the same namespace as the `Backstage` custom resource, mounts `lightspeed-stack-okp.yaml`, and selects it as the LCORE configuration. The RHDH Operator does not manage Kubernetes Ingress, registry credentials, or the public OKP hostname.

The public hostname must be reachable by both Lightspeed Core inside the cluster and users' browsers so that retrieval and clickable citation links use the same URL.

Set values used by the examples:

```bash
export NAMESPACE=rhdh-test
export BACKSTAGE_NAME=developer-hub
export OKP_HOST=okp.example.com
```

## Configure registry authentication

Create a pull secret for `registry.redhat.io` and attach it to the namespace's default ServiceAccount:

```bash
kubectl create secret docker-registry rh-registry-secret \
  --namespace "$NAMESPACE" \
  --docker-server=registry.redhat.io \
  --docker-username='<registry-username>' \
  --docker-password='<registry-password>'

kubectl patch serviceaccount default \
  --namespace "$NAMESPACE" \
  --type merge \
  --patch '{"imagePullSecrets":[{"name":"rh-registry-secret"}]}'
```

Preserve any existing `imagePullSecrets` entries when applying this patch.

## Enable the add-on flavour

```yaml
spec:
  flavours:
    - name: intelligent-assistant
      enabled: true
    - name: intelligent-assistant-okp
      enabled: true
```

## Create an Ingress

Create a TLS Secret for the public hostname:

```bash
kubectl create secret tls okp-tls \
  --namespace "$NAMESPACE" \
  --cert=path/to/tls.crt \
  --key=path/to/tls.key
```

Create an Ingress. Replace the class, hostname, and Service name for your environment:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: intelligent-assistant-okp
  namespace: rhdh-test
spec:
  ingressClassName: nginx
  tls:
    - hosts:
        - okp.example.com
      secretName: okp-tls
  rules:
    - host: okp.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: ia-okp-developer-hub
                port:
                  number: 8080
```

The default RHDH NetworkPolicies already allow HTTPS egress from the Backstage pod on TCP port 443. The OKP add-on additionally allows HTTP egress on ports 80 and 8080. If you replace the default policies, ensure that the replacement permits the port used by `OKP_SERVICE_URL`.

## Configure the public OKP URL

Inject the Ingress URL into Lightspeed Core through the Backstage CR:

```yaml
spec:
  application:
    extraEnvs:
      envs:
        - name: OKP_SERVICE_URL
          value: https://okp.example.com
          containers:
            - lightspeed-core
```

If the Ingress certificate uses a private CA, mount its public CA certificate into `lightspeed-core` and configure `SSL_CERT_FILE` and `REQUESTS_CA_BUNDLE` with a combined system and private CA bundle. Do not place private keys in a ConfigMap.

The Operator also does not create an Ingress for RHDH itself on Kubernetes. Configure the RHDH Ingress and matching `app.baseUrl`, `backend.baseUrl`, and CORS origin separately.

## Verify

```bash
kubectl get pods,deployments,services,ingresses,networkpolicies -n "$NAMESPACE"
kubectl get deployment "backstage-${BACKSTAGE_NAME}" -n "$NAMESPACE" \
  -o jsonpath='{.spec.template.spec.containers[?(@.name=="lightspeed-core")].env}'
curl -I "https://${OKP_HOST}/"
```

Confirm that the OKP Deployment and Service are ready, Lightspeed Core has the public `OKP_SERVICE_URL`, and citations returned by Intelligent Assistant open in a browser.
