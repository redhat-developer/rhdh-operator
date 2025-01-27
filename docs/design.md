# Backstage Operator Design

The goal of Backstage Operator is to deploy Backstage workload to the Kubernetes namespace and keep this workload synced with the desired state defined by configuration. 

## Backstage Kubernetes Runtime

Backstage Kubernetes workload consists of set of Kubernetes resources (Runtime Objects).
Approximate set of Runtime Objects necessary for Backstage server on Kubernetes is shown on the diagram below:

![Backstage Kubernetes Runtime](images/backstage_kubernetes_runtime.jpg)

The most important object is Backstage Pod created by Backstage Deployment. That is where we run 'backstage-backend' container with Backstage application inside.
This Backstage application is a web server which can be reached using Backstage Service.
Actually, those 2 are the core part of Backstage workload. 

Backstage application uses SQL database as a data storage and it is possible to install PostgreSQL DB on the same namespace as Backstage instance.
It brings PostgreSQL StatefulSet/Pod, Service to connect to Backstage and PV/PVC to store the data.

For providing external access to Backstage server it is possible, depending on underlying infrastructure, to use Openshift Route or
K8s Ingress on top of Backstage Service.
Note that in versions up to 0.0.2, only Route configuration is supported by the Operator.

Finally, the Backstage Operator supports all the [Backstage configuration](https://backstage.io/docs/conf/writing) options, which can be provided by creating dedicated 
ConfigMaps and Secrets, then contributing them to the Backstage Pod as mounted volumes or environment variables (see [Configuration](configuration.md) guide for details).  

## Configuration

### Configuration layers

The Backstage Operator can be configured to customize the deployed workload.
With no changes to the default configuration, an admin user can deploy a Backstage instance to try it out for a local, personal, or small group test deployment.

When you do want to customize your Backstage instance, there are 3 layers of configuration available.

![Backstage Operator Configuration Layers](images/backstage_operator_configuration_layers.jpg)

As shown in the picture above:

- There is an Operator (Cluster) level Default Configuration implemented as a ConfigMap inside Backstage system namespace
  (where Backstage controller is launched). It allows to choose some optimal for most cases configuration which will be applied 
if there are no other config to override (i.e. Backstage CR is empty). 
- Another layer overriding default is instance (Backstage CR) scoped, implemented as a ConfigMap which
has the same as default structure but inside Backstage instance's namespace. The name of theis ConfigMap 
is specified on Backstage.Spec.RawConfig field. It offers very flexible way to configure certain Backstage instance  
- And finally, there are set of fields on Backstage.Spec to override configuration made on level 1 and 2.
It offers simple configuration of some parameters. So, user is not required to understand the
overall structure of Backstage runtime object and is able to simply configure "the most important" parameters.
  (see [configuration](configuration.md) for more details)

### Backstage Application

Backstage Application comes with advanced configuration features.

As per the [Backstage configuration](https://backstage.io/docs/conf/writing), a user can define and overload multiple _app-config.yaml_
files and flexibly configure them by including environment variables.
Backstage Operator supports this flexibility allowing to define these configurations components in all the configuration levels
(default, raw and CR)

![Backstage App with Advanced Configuration](images/backstage_application_advanced_config.jpg)

#### Updating mounted files

As you can see, the Operator mounts ConfigMaps and Secrets to the Backstage container
* As a part of Default/Raw configuration, configuring certan configuration files
* As a part of Backstage CR configuration, using spec.application field

In either case ConfigMaps/Secrets data's key/value is transformed to file's name/content on Backstage CR creating time and the general expectation is to be able to update the file contents by updating the corresponding ConfigMap/Secret.
Kubernetes [allows this updating](https://kubernetes.io/docs/tasks/configure-pod-container/configure-pod-configmap/#mounted-configmaps-are-updated-automatically) but only if volume mount does not contain subPath. In turn, using subPath allows to mount certain file individually on certain container's directory not worrying about directories overlapping, which is beneficial for some cases. 

Historically, the Operator actively uses subPath option which allows to make "convenient" Backstage App file structure (e g all the app-config files in the same directory). In this case file(s) are mounted to default directory or to the **spec.application.(appConfig|extraFiles).mountPath** field if specified. 
Also, in a case if user need only certain key (file) to be mounted, the only choice is to use subPath.
In order to be able to update file(s) for mounted with subPath volumes case Operator watches ConfigMaps/Secrets and refreshes (recreates) Backstage Pod in a case of changes.
Technically this approach is working in any case (with or without subPath) but it has certan disadvantages:
* recreating of Pod is quite slow
* it disables in fact using Backstage's file watching mechanism. Indeed, configuration changing causes file-system rebooting, so file-system watchers have no effect.  

Another option, implemented in version 0.4.0, is to specify the **mountPath** and not specify the key (filename). In this case, Operator relies on the automatic update provided by Kubernetes, it simply mounts a directory with all key/value files at the specified path.

#### Updating injected environment variables

As you can see, the Operator injects environment variables to the Backstage container with ConfigMaps and Secrets 
* As a part of Default/Raw configuration, configuring certan configuration files
* As a part of Backstage CR configuration, using spec.application field
Kubernetes doesn’t allow you to change environment variables after a Pod has been created, so in order to update enviromnent variables when ConfigMap or Secret changed Operator refreshes the Pod the same way as mentioned [here](#updating-mounted-files) 


## Status

Backstage Custom Resource contains **Deployed** condition in the Status field. 
It is updated by the Operator and can have the following values:
- **DeployInProgress** - Backstage Deployment is not available yet. The current state of Deployment can be seen in the message field
- **Deployed** - Backstage Deployment is being created and application is available
- **DeployFailed** - Backstage Deployment creation failed. The actual error can be seen in the message field 

