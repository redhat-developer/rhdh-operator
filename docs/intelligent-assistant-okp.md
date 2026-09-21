# Offline Knowledge Portal for Intelligent Assistant

Offline Knowledge Portal (OKP) provides Red Hat Developer Hub product documentation to Lightspeed Core. OKP is optional and disabled by default on both OpenShift and Kubernetes.

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

Enabling the add-on causes the Operator to:

- Add the `okp` dependency to the Intelligent Assistant backend plugin.
- Create the OKP Deployment and Service.
- Select the generated `lightspeed-stack-okp.yaml` LCORE configuration.
- Allow HTTP egress from Lightspeed Core to OKP on ports 80 and 8080.
- On OpenShift only, create an OKP Route and set `OKP_SERVICE_URL` to its HTTP URL.

The OKP image is large, so the first installation can take significantly longer while the image is downloaded.

For platform-specific configuration, see:

- [Enabling OKP on Kubernetes](intelligent-assistant-okp-kubernetes.md)
- [Using HTTPS for OKP on OpenShift](intelligent-assistant-okp-openshift-https.md)
