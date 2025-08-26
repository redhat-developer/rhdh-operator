# Troubleshooting Guide

This guide helps you diagnose and resolve common issues when deploying and operating the Red Hat Developer Hub (RHDH) Operator.

## Table of Contents

- [Common Deployment Issues](#common-deployment-issues)
- [Pod Issues](#pod-issues)
- [Database Connection Issues](#database-connection-issues)
- [Configuration Issues](#configuration-issues)
- [Network and Access Issues](#network-and-access-issues)
- [Resource Issues](#resource-issues)
- [Debugging Tools and Commands](#debugging-tools-and-commands)

## Common Deployment Issues

### Operator Installation Fails

**Symptoms:**
- Operator pod fails to start
- CRD installation errors
- RBAC permission denied errors

**Diagnosis:**
```bash
# Check operator pod status
kubectl get pods -n backstage-system

# Check operator logs
kubectl logs -n backstage-system deployment/backstage-controller-manager

# Verify CRDs are installed
kubectl get crd | grep backstage
```

**Solutions:**
1. **RBAC Issues**: Ensure you have cluster-admin privileges
   ```bash
   kubectl auth can-i "*" "*" --all-namespaces
   ```

2. **Insufficient permissions**: Grant cluster-admin role
   ```bash
   kubectl create clusterrolebinding cluster-admin-binding \
     --clusterrole=cluster-admin --user=$(oc whoami)
   ```

3. **Namespace doesn't exist**: Create the operator namespace
   ```bash
   kubectl create namespace backstage-system
   ```

### Backstage CR Fails to Deploy

**Symptoms:**
- Backstage Custom Resource shows error status
- No Backstage pods created
- CR stuck in pending state

**Diagnosis:**
```bash
# Check CR status and events
kubectl describe backstage <backstage-cr-name> -n <namespace>

# Check operator logs for reconciliation errors
kubectl logs -n backstage-system deployment/backstage-controller-manager -f
```

**Common Solutions:**
1. **Invalid configuration**: Validate your CR against the schema
2. **Missing dependencies**: Ensure all required ConfigMaps exist
3. **Resource conflicts**: Check for name collisions with existing resources

## Pod Issues

### Backstage Pod CrashLoopBackOff

**Symptoms:**
- Backstage pod repeatedly crashes
- Pod status shows `CrashLoopBackOff`
- High restart count

**Diagnosis:**
```bash
# Check pod status and events
kubectl describe pod <backstage-pod-name> -n <namespace>

# Check application logs
kubectl logs <backstage-pod-name> -n <namespace>

# Check previous container logs if pod restarted
kubectl logs <backstage-pod-name> -n <namespace> --previous
```

**Common Causes and Solutions:**

1. **Database connection failure**:
   - Verify database credentials in secrets
   - Check database service availability
   - Ensure network connectivity between pods

2. **Missing configuration files**:
   - Verify all required ConfigMaps are created
   - Check file mount paths and permissions
   - Validate app-config.yaml syntax

3. **Plugin errors**:
   - Check dynamic plugin configuration
   - Verify plugin compatibility with Backstage version
   - Review plugin-specific logs

### PostgreSQL Pod Issues

**Symptoms:**
- Database pod fails to start
- Connection refused errors
- Data persistence issues

**Diagnosis:**
```bash
# Check PostgreSQL pod status
kubectl get pods -l app.kubernetes.io/name=backstage-psql -n <namespace>

# Check PostgreSQL logs
kubectl logs <postgres-pod-name> -n <namespace>

# Test database connectivity
kubectl exec -it <postgres-pod-name> -n <namespace> -- psql -U postgres
```

**Solutions:**
1. **Storage issues**: Verify PVC is bound and has sufficient space
2. **Permission issues**: Check pod security context and file permissions
3. **Resource constraints**: Increase memory/CPU limits if needed

## Database Connection Issues

### Connection Refused or Timeout

**Symptoms:**
- "Connection refused" errors in Backstage logs
- Database connectivity timeouts
- Authentication failures

**Diagnosis:**
```bash
# Test database connectivity from Backstage pod
kubectl exec -it <backstage-pod-name> -n <namespace> -- \
  nc -zv <db-service-name> 5432

# Check database service endpoints
kubectl get endpoints <db-service-name> -n <namespace>

# Verify database credentials
kubectl get secret <db-secret-name> -n <namespace> -o yaml
```

**Solutions:**
1. **Service DNS resolution**: Ensure services are in the same namespace or use FQDN
2. **Network policies**: Check if NetworkPolicies are blocking traffic
3. **Firewall rules**: Verify cluster network configuration
4. **Credential mismatch**: Update database secrets with correct credentials

### Database Migration Failures

**Symptoms:**
- Migration errors in logs
- Database schema version mismatches
- "relation does not exist" errors

**Diagnosis:**
```bash
# Check migration logs
kubectl logs <backstage-pod-name> -n <namespace> | grep -i migration

# Connect to database and check schema
kubectl exec -it <postgres-pod-name> -n <namespace> -- \
  psql -U postgres -d backstage_plugin_catalog -c "\dt"
```

**Solutions:**
1. **Manual migration**: Run database migrations manually if needed
2. **Clean slate**: Drop and recreate database for development environments
3. **Version compatibility**: Ensure Backstage version matches expected schema

## Configuration Issues

### Invalid App Configuration

**Symptoms:**
- YAML parsing errors
- Configuration validation failures
- Missing required configuration sections

**Diagnosis:**
```bash
# Validate YAML syntax
kubectl get configmap backstage-appconfig-<cr-name> -n <namespace> -o yaml

# Check for configuration errors in logs
kubectl logs <backstage-pod-name> -n <namespace> | grep -i "config\|error"
```

**Solutions:**
1. **YAML syntax**: Use online YAML validators to check syntax
2. **Required fields**: Refer to Backstage documentation for required configuration
3. **Environment variables**: Ensure all referenced environment variables are set

### Dynamic Plugin Issues

**Symptoms:**
- Plugins fail to load
- Plugin-specific functionality not available
- Plugin installation errors

**Diagnosis:**
```bash
# Check dynamic plugins configuration
kubectl get configmap backstage-dynamic-plugins-<cr-name> -n <namespace> -o yaml

# Look for plugin-specific errors
kubectl logs <backstage-pod-name> -n <namespace> | grep -i plugin
```

**Solutions:**
1. **Plugin compatibility**: Verify plugin versions are compatible
2. **Installation path**: Check plugin installation directory and permissions
3. **Dependencies**: Ensure all plugin dependencies are available

## Network and Access Issues

### Cannot Access Backstage UI

**Symptoms:**
- Browser cannot reach Backstage
- 404 or connection timeout errors
- Service unavailable

**Diagnosis:**
```bash
# Check service status
kubectl get service backstage-<cr-name> -n <namespace>

# For OpenShift, check route
kubectl get route backstage-<cr-name> -n <namespace>

# Test internal connectivity
kubectl port-forward service/backstage-<cr-name> 7007:7007 -n <namespace>
```

**Solutions:**
1. **Service configuration**: Verify service ports and selectors
2. **Ingress/Route**: Check ingress controller or OpenShift route configuration
3. **Firewall**: Ensure required ports are open
4. **DNS**: Verify DNS resolution for external access

### TLS/SSL Certificate Issues

**Symptoms:**
- Certificate validation errors
- HTTPS connection failures
- Browser security warnings

**Solutions:**
1. **Self-signed certificates**: Configure browser to accept self-signed certs for development
2. **Certificate renewal**: Check certificate expiration dates
3. **CA trust**: Ensure proper certificate authority configuration

## Resource Issues

### Insufficient Resources

**Symptoms:**
- Pods stuck in `Pending` state
- Out of memory (OOMKilled) errors
- CPU throttling warnings

**Diagnosis:**
```bash
# Check resource usage
kubectl top pods -n <namespace>
kubectl top nodes

# Check resource requests and limits
kubectl describe pod <pod-name> -n <namespace>

# Check node capacity
kubectl describe nodes
```

**Solutions:**
1. **Increase limits**: Adjust CPU and memory limits in deployment
2. **Node scaling**: Add more nodes to cluster if needed
3. **Resource optimization**: Review and optimize resource usage

### Storage Issues

**Symptoms:**
- PVC stuck in `Pending` state
- Disk space errors
- Data persistence problems

**Diagnosis:**
```bash
# Check PVC status
kubectl get pvc -n <namespace>

# Check storage class
kubectl get storageclass

# Check available storage
kubectl describe pv
```

**Solutions:**
1. **Storage class**: Ensure appropriate storage class is available
2. **Disk space**: Free up space or provision larger volumes
3. **PVC configuration**: Verify PVC size and access modes

## Debugging Tools and Commands

### Essential Debugging Commands

```bash
# Get all resources in namespace
kubectl get all -n <namespace>

# Describe any resource for detailed information
kubectl describe <resource-type> <resource-name> -n <namespace>

# Get events sorted by time
kubectl get events -n <namespace> --sort-by='.lastTimestamp'

# Watch resources in real-time
kubectl get pods -n <namespace> -w

# Execute commands in running pods
kubectl exec -it <pod-name> -n <namespace> -- /bin/bash

# Copy files from/to pods
kubectl cp <pod-name>:/path/to/file ./local-file -n <namespace>
```

### Log Analysis

```bash
# Follow logs from multiple pods
kubectl logs -f deployment/backstage-<cr-name> -n <namespace>

# Get logs from all containers in a pod
kubectl logs <pod-name> -n <namespace> --all-containers=true

# Filter logs for specific patterns
kubectl logs <pod-name> -n <namespace> | grep -i error

# Save logs to file for analysis
kubectl logs <pod-name> -n <namespace> > backstage-logs.txt
```

### Performance Monitoring

```bash
# Check resource usage
kubectl top pods -n <namespace>
kubectl top nodes

# Monitor resource usage over time
watch kubectl top pods -n <namespace>

# Check pod resource limits and requests
kubectl describe pod <pod-name> -n <namespace> | grep -A 10 "Limits\|Requests"
```

## Getting Help

If you continue to experience issues:

1. **Check operator logs** for detailed error messages
2. **Review configuration** against the official documentation
3. **Search existing issues** in the RHDH operator repository
4. **Report issues** using JIRA: https://issues.redhat.com/browse/RHIDP with Component: **Operator**

For urgent production issues, contact Red Hat Support with:
- Cluster information (`kubectl version`, `oc version`)
- Operator version and configuration
- Complete error logs and resource descriptions
- Steps to reproduce the issue