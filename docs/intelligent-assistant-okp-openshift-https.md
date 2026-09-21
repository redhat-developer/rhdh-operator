# Using HTTPS for OKP on OpenShift

When `intelligent-assistant-okp` is enabled on OpenShift, the Operator creates an edge-terminated Route and configures Lightspeed Core to use its HTTP URL. This requires no additional certificate configuration.

To use the HTTPS Route on a cluster whose router certificate is signed by a private CA, Lightspeed Core must trust the router CA.

## Copy the public router CA

```bash
export RHDH_NAMESPACE=rhdh-test
export BACKSTAGE_NAME=developer-hub

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

## Verify HTTPS egress

The default RHDH NetworkPolicies allow HTTPS egress from the Backstage pod on TCP port 443. If you replace the default policies, ensure that the replacement permits HTTPS access to the OKP Route.

## Mount and trust the CA

Add the public HTTPS URL, CA ConfigMap, and Lightspeed Core patch to the Backstage CR. Preserve any existing `extraFiles`, `extraEnvs`, and Deployment patch entries.

```yaml
spec:
  application:
    extraEnvs:
      envs:
        - name: OKP_SERVICE_URL
          value: https://lightspeed-okp-developer-hub-rhdh-test.apps.example.com
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
