# Enabling OKP on OpenShift

Offline Knowledge Portal (OKP) provides Red Hat Developer Hub product documentation to Lightspeed Core. OKP is optional and disabled by default.

Enable it with the `intelligent-assistant-okp` add-on flavour:

```yaml
spec:
  flavours:
    - name: intelligent-assistant
      enabled: true
    - name: intelligent-assistant-okp
      enabled: true
```

The example lists both flavours explicitly for clarity. Because `intelligent-assistant` is enabled by default, specifying only `intelligent-assistant-okp` with `enabled: true` also enables the complete combination. Do not explicitly disable `intelligent-assistant` while enabling `intelligent-assistant-okp`; the add-on requires the base Intelligent Assistant flavour.

On OpenShift, enabling the add-on causes the Operator to:

- Add the `okp` dependency to the Intelligent Assistant backend plugin.
- Create the OKP Deployment, Service, and edge-terminated Route.
- Select the generated `lightspeed-stack-okp.yaml` LCORE configuration.
- Allow HTTP egress from Lightspeed Core to OKP on ports 80 and 8080.
- Set `OKP_SERVICE_URL` to the Route's HTTP URL.

The OKP image is large, so the first installation can take significantly longer while the image is downloaded. The default HTTP Route requires no additional certificate configuration.

## Use HTTPS with a private router CA

To use the HTTPS Route on a cluster whose router certificate is signed by a private CA, Lightspeed Core must trust the router CA.

### Copy the public router CA

```bash
export RHDH_NAMESPACE=rhdh-test

mkdir -p /tmp/rhdh-okp-router-ca
oc extract configmap/default-ingress-cert \
  --namespace openshift-config-managed \
  --keys=ca-bundle.crt \
  --to=/tmp/rhdh-okp-router-ca \
  --confirm

oc create configmap okp-router-ca \
  --namespace "$RHDH_NAMESPACE" \
  --from-file=ingress-ca.crt=/tmp/rhdh-okp-router-ca/ca-bundle.crt \
  --dry-run=client --output yaml | oc apply -f -
```

The ConfigMap contains only a public CA certificate. Do not copy the router Secret or its private key.

### Verify HTTPS egress

The default RHDH NetworkPolicies allow HTTPS egress from the Backstage pod on TCP port 443. If you replace the default policies, ensure that the replacement permits HTTPS access to the OKP Route.

### Mount and trust the CA

Add the public HTTPS URL, CA ConfigMap, and Lightspeed Core patch to the Backstage CR. Preserve any existing `extraFiles`, `extraEnvs`, and Deployment patch entries.

```yaml
spec:
  application:
    extraEnvs:
      envs:
        - name: OKP_SERVICE_URL
          value: https://intelligent-assistant-okp-developer-hub-rhdh-test.apps.example.com
          containers:
            - lightspeed-core
    extraFiles:
      configMaps:
        - name: okp-router-ca
          key: ingress-ca.crt
          mountPath: /app-root
          containers:
            - lightspeed-core
  deployment:
    patch:
      spec:
        template:
          spec:
            containers:
              - name: lightspeed-core
                command:
                  - /bin/sh
                  - -c
                args:
                  - |
                    cat /etc/pki/tls/certs/ca-bundle.crt /app-root/ingress-ca.crt > /tmp/combined-ca-bundle.crt
                    export SSL_CERT_FILE=/tmp/combined-ca-bundle.crt
                    export REQUESTS_CA_BUNDLE=/tmp/combined-ca-bundle.crt
                    exec /app-root/entrypoint.sh \
                      --config /app-root/lightspeed-stack-okp.yaml \
                      --synthesized-config-output /tmp/.generated/run.yaml
```

The Route can retain `insecureEdgeTerminationPolicy: Allow`; it accepts both the default HTTP URL and this optional HTTPS URL. Recreate `okp-router-ca` if the OpenShift ingress CA changes.

## Verify

```bash
oc get pods,deployments,services,routes,networkpolicies -n "$RHDH_NAMESPACE"
oc get deployment backstage-developer-hub -n "$RHDH_NAMESPACE" \
  -o jsonpath='{.spec.template.spec.containers[?(@.name=="lightspeed-core")].env}'
```

Confirm that the OKP Deployment, Service, and Route are ready and that Lightspeed Core has the expected `OKP_SERVICE_URL`.
