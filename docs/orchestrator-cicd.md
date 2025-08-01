# GitOps and Pipeline Configuration for Orchestrator

Using CI/CD can streamline and ease the workflow development lifecycle, by building workflow images with a Tekton
pipeline while ArgoCD monitors changes in the workflow repository and automatically triggers the appropriate Tekton
pipelines when updates are detected.

## Install the Operators

There are three methods to install GitOps/Pipelines Operator

### Method 1: Install the Operators using Scripts

Refer to the `RHDH helper script` section in [orchestrator guide](orchestrator.md) and set the `--with-cicd` flag to
true when running the script.

### Method 2: Install the Operators from Demo Charts

You can use the Janus IDP Demo repository to install the `Red Hat OpenShift Pipelines` and `Red Hat OpenShift GitOps`
operators. This repository contains automation scripts to install the Janus IDP Demo and its supporting components. Note
that a fork of this repository has been created to remove the configuration excluding Tekton resources from being
managed by ArgoCD applications. More details can be found in
this [discussion](https://github.com/argoproj/argo-cd/discussions/8674#discussioncomment-2318554).

#### Install OpenShift Pipelines Operator

1. Clone the repository:

    ```bash
    git clone https://github.com/rhdhorchestrator/janus-idp-bootstrap.git
    ```

2. Navigate to the charts directory:

    ```bash
    cd janus-idp-bootstrap/charts
    ```
3. Install the OpenShift Pipelines operator:

    ```bash
    helm upgrade --install orchestrator-pipelines pipelines-operator/ -f pipelines-operator/values.yaml -n orchestrator-gitops --create-namespace --set operator.channel=pipelines-1.17
    ```

#### Install OpenShift GitOps Operator

1. Install and configure the OpenShift GitOps operator:

    ```bash
    helm upgrade --install orchestrator-gitops gitops-operator/ -f gitops-operator/values.yaml -n orchestrator-gitops --create-namespace --set namespaces={orchestrator-gitops}
    ```

### Method 3: Install the Operators from OpenShift OperatorHub

#### Install OpenShift Pipelines Operator

The OpenShift Pipelines Operator can be installed directly from the OperatorHub. Select the operator from the list and
install it without any special configuration.

Make sure you install an OpenShift Pipelines Operator's version compatible with your RHDH Operator's version:

#### Install OpenShift GitOps Operator

To install the OpenShift GitOps Operator with custom configuration:

1. Add the following configuration to the Subscription used to install the operator:

    ```yaml
    config:
      env:
      - name: DISABLE_DEFAULT_ARGOCD_INSTANCE
        value: "true"
      - name: ARGOCD_CLUSTER_CONFIG_NAMESPACES
        value: << ARGOCD_NAMESPACE >>
    ```
   Ensure to update the ARGOCD_NAMESPACE with the correct value.
   Detailed information about these environment variables can be found in
   the [OpenShift GitOps Usage Guide](https://github.com/redhat-developer/gitops-operator/blob/master/docs/OpenShift%20GitOps%20Usage%20Guide.md#installation-of-openshift-gitops-without-ready-to-use-argo-cd-instance-for-rosaosd)
   and
   the [ArgoCD Operator Documentation](https://argocd-operator.readthedocs.io/en/latest/usage/basics/#cluster-scoped-instance).

2. Create an ArgoCD instance in the `orchestrator-gitops` namespace:

    ```bash
    oc new-project orchestrator-gitops
    oc apply -f https://raw.githubusercontent.com/redhat-developer/rhdh-operator/main/config/profile/rhdh/plugin-infra/argocd-cr.yaml
    ```

   Alternatively, if creating a default ArgoCD instance, ensure to exclude Tekton resources from its specification:

    ```yaml
    resourceExclusions: |
      - apiGroups:
        - tekton.dev
        clusters:
        - '*'
        kinds:
        - TaskRun
        - PipelineRun
    ```

These steps will set up the required CI/CD environment using either method. Ensure to follow the steps carefully to
achieve a successful installation.

## Installing docker credentials

The Tekton pipeline deployed by the Orchestrator is responsible for building a workflow image and pushing it to Quay.io.
There is a need to create a single K8s secret combined with the following secrets:

1. A secret for Quay.io organization to push the images built by the pipeline:
    - Create or edit
      a [Robot account](https://access.redhat.com/documentation/en-us/red_hat_quay/3.3/html/use_red_hat_quay/use-quay-manage-repo)
      and grant it `Write` permissions to the newly created repository
    - Download the credentials as Kubernetes secret.
2. A secret for _registry.redhat.io_. To build workflow images, the pipeline uses
   the [builder image](https://github.com/rhdhorchestrator/serverless-workflows/blob/main/pipeline/workflow-builder.Dockerfile)
   from [registry.redhat.io](https://registry.redhat.io).
    - Generate a token [here](https://access.redhat.com/terms-based-registry/create), and download it as OCP secret.

Those two K8s secrets should be merged into a single secret named `docker-credentials` in `orchestrator-gitops`
namespace in the cluster that runs the pipelines.
You may use
this [helper script](https://github.com/rhdhorchestrator/orchestrator-go-operator/blob/main/hack/merge_secrets.sh) to
merge the secrets or choose another method of downloading the credentials and merging them.

## Define the SSH credentials

The pipeline uses SSH to push the deployment configuration to the `gitops` repository containing the `kustomize`
deployment configuration.

The GitOps repository can be configured on GitHub or GitLab.

### Configuring on GitHub

Follow these steps to properly configure the credentials in the namespace:

- Generate default SSH keys under the `github_ssh` folder

```console
mkdir -p github_ssh
ssh-keygen -t rsa -b 4096 -f github_ssh/id_rsa -N "" -C git@github.com -q
```

- Add the SSH key to your GitHub account using the gh CLI or using the [SSH keys](https://github.com/settings/keys)
  setting:

```console
gh ssh-key add github_ssh/id_rsa.pub --title "Tekton pipeline"
```

- Create a `known_hosts` file by scanning the GitHub's SSH public key:

```console
ssh-keyscan github.com > github_ssh/known_hosts
```

- Create the default `config` file:

```console
echo "Host github.com
  HostName github.com
  IdentityFile ~/.ssh/id_rsa" > github_ssh/config
```

- Create the secret that the Pipeline uses to store the SSH credentials:

```console
oc create secret -n orchestrator-gitops generic github-ssh-credentials \
  --from-file=github_ssh/id_rsa \
  --from-file=github_ssh/config \
  --from-file=github_ssh/known_hosts
```

Note: if you change the SSH key type from the default value `rsa`, you need to update the `config` file accordingly

### Configuring on GitLab

Follow these steps to properly configure the credentials in the namespace:

- Generate default SSH keys under the `gitlab_ssh` folder

Replace <GitLabHost> with the GitLab instance URL where your GitOps project will be hosted.

```console
mkdir -p gitlab_ssh
ssh-keygen -t rsa -b 4096 -f gitlab_ssh/id_rsa -N "" -C git@<GitLabHost> -q
```

- Add the SSH key to your GitLab account using the glab CLI or using the SSH keys setting:
  You can find those setting in https://\<GitLabHost\>/-/user_settings/ssh_keys
- Make sure to authenticate to your GitLab instance using your personal access token / Web authentication prior to using
  glab CLI.

```console
glab ssh-key add gitlab_ssh/id_rsa.pub --title "Tekton pipeline"
```

- Create a `known_hosts` file by scanning the GitLab host's SSH public key:

```console
ssh-keyscan <GitLabHost> > gitlab_ssh/known_hosts
```

- Create the default `config` file:

```console
echo "Host <GitLabHost>
HostName <GitLabHost>
IdentityFile ~/.ssh/id_rsa" > gitlab_ssh/config
```

- Create the secret that the Pipeline uses to store the SSH credentials:

```console
oc create secret -n orchestrator-gitops generic gitlab-ssh-credentials \
  --from-file=gitlab_ssh/id_rsa \
  --from-file=gitlab_ssh/config \
  --from-file=gitlab_ssh/known_hosts
```

Note: if you change the SSH key type from the default value `rsa`, you need to update the `config` file accordingly

## Setting up GitHub Integration

To begin serverless workflow development using the "Basic workflow bootstrap project" software template with GitHub as
the target source control, you'll need to configure organization settings to allow read and write permissions for GitHub
workflows. Follow these steps to enable the necessary permissions:

1. Navigate to your organization settings on GitHub.
2. Locate the section for managing organization settings related to GitHub Actions.
3. Enable read and write permissions for workflows by adjusting the settings accordingly.
4. For detailed instructions and exact steps, refer to the GitHub guide
   available [here](https://docs.github.com/en/enterprise-server@3.9/organizations/managing-organization-settings/disabling-or-limiting-github-actions-for-your-organization#configuring-the-default-github_token-permissions).

## Setting up GitLab Integration

To begin serverless workflow development using the "Basic workflow bootstrap project" software template with GitLab as
the target source control, you'll need to configure project settings to allow read and write permissions for GitLab
CI/CD pipelines. Follow these steps to enable the necessary permissions:

1. Navigate to your project settings on GitLab
    - Go to your GitLab instance (e.g., gitlab.cee.redhat.com).
    - Open the Project where you want to enable workflows.
    - On the left sidebar, click Settings → CI/CD.
2. Locate the section for managing CI/CD permissions
    - Scroll down to the Runners section and ensure a runner is configured.
    - Check the Pipeline permissions settings under the "General pipelines" section.
    - Enable read and write permissions for workflows by customizing your pipeline configuration, variables, and
      artifacts according to your needs.

3. For detailed instructions and exact steps, refer to the GitLab guide available [here](https://docs.gitlab.com/ci/)

## Setting up Authentication for Workflow CICD

The `gitops-secret-setup.sh` script helps to setup the environment variable by creating the required authentication secret for RHDH.

1. Create a namespace for the RHDH instance if it does not already exist:

   ```console
   oc new-project rhdh
   ```

1. Download the gitops-secret-setup script from the github repository and run it to create the RHDH secret:

   ```console
   wget https://raw.githubusercontent.com/redhat-developer/rhdh-operator/main/plugin-infra/gitops-secret-setup.sh -O /tmp/gitops-secret-setup.sh && chmod u+x /tmp/gitops-secret-setup.sh
   ```

1. Run the script:
   ```console
   /tmp/gitops-secret-setup.sh --use-default
   ```

**NOTE:** If you don't want to use the default values, omit the `--use-default` and the script will prompt you for
input.

A secret ref called `backstage-backend-auth-secret` should be created containing keys: `ARGOCD_URL`, `ARGOCD_USERNAME`
`ARGOCD_PASSWORD`, etc. Also, this secret should be mounted when applying the backstage CR as seen in
this [example](examples/orchestrator-cicd.yaml).

## Update the Dynamic Plugin ConfigMap for RHDH

To enable the ArgoCD and tekton plugins, update your Dynamic Plugins ConfigMap by adding the following:

```yaml
    includes:
      - dynamic-plugins.default.yaml
    plugins:
       ......
       - package: ./dynamic-plugins/dist/backstage-community-plugin-redhat-argocd
         disabled: false
         dependencies:
            - ref: argocd
       - package: ./dynamic-plugins/dist/backstage-community-plugin-tekton
         disabled: false
         dependencies:
            - ref: tekton
       - package: ./dynamic-plugins/dist/roadiehq-backstage-plugin-argo-cd-backend-dynamic
         disabled: false
       - package: ./dynamic-plugins/dist/roadiehq-scaffolder-backend-argocd-dynamic
         disabled: false
```

This should create the ArgoCD AppProject resource and Tekton pipeline called `workflow-deployment`, for RHDH
Orchestrator workflow. Please refer to this [example](examples/orchestrator-cicd.yaml) for a complete configuration.

Compatibility Matrix

| OpenShift Pipelines Operator version | RHDH Operator Version |
|--------------------------------------|-----------------------|
| 4.17                                 | 1.8.x                 |