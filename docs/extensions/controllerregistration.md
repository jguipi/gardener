---
title: ControllerRegistration
---

# Registering Extension Controllers

Extensions are registered in the garden cluster via [`ControllerRegistration`](../../example/25-controllerregistration.yaml) resources.
Deployment for respective extensions are specified via [`ControllerDeployment`](../../example/25-controllerdeployment.yaml) resources.
Gardener evaluates the registrations and deployments and creates [`ControllerInstallation`](../../example/25-controllerinstallation.yaml) resources which describe the request "please install this controller `X` to this seed `Y`".

Similar to how `CloudProfile` or `Seed` resources get into the system, the Gardener administrator must deploy the `ControllerRegistration` and `ControllerDeployment` resources (this does not happen automatically in any way - the administrator decides which extensions shall be enabled).

The specification mainly describes which of Gardener's extension CRDs are managed, for example:

```yaml
apiVersion: core.gardener.cloud/v1beta1
kind: ControllerDeployment
metadata:
  name: os-gardenlinux
type: helm
providerConfig:
  chart: H4sIFAAAAAAA/yk... # <base64-gzip-chart>
  values:
    foo: bar
---
apiVersion: core.gardener.cloud/v1beta1
kind: ControllerRegistration
metadata:
  name: os-gardenlinux
spec:
  deployment:
    deploymentRefs:
    - name: os-gardenlinux
  resources:
  - kind: OperatingSystemConfig
    type: gardenlinux
    primary: true
```

This information tells Gardener that there is an extension controller that can handle `OperatingSystemConfig` resources of type `gardenlinux`.
A reference to the shown `ControllerDeployment` specifies how the deployment of the extension controller is accomplished.

Also, it specifies that this controller is the primary one responsible for the lifecycle of the `OperatingSystemConfig` resource.
Setting `primary` to `false` would allow to register additional, secondary controllers that may also watch/react on the `OperatingSystemConfig/coreos` resources, however, only the primary controller may change/update the main `status` of the extension object (that are used to "communicate" with the gardenlet).
Particularly, only the primary controller may set `.status.lastOperation`, `.status.lastError`, `.status.observedGeneration`, and `.status.state`.
Secondary controllers may contribute to the `.status.conditions[]` if they like, of course.
 
Secondary controllers might be helpful in scenarios where additional tasks need to be completed which are not part of the reconciliation logic of the primary controller but separated out into a dedicated extension. 

⚠️ There must be exactly one primary controller for every registered kind/type combination.
Also, please note that the `primary` field cannot be changed after creation of the `ControllerRegistration`.

## Deploying Extension Controllers

Submitting the above `ControllerDeployment` and `ControllerRegistration` will create a `ControllerInstallation` resource:

```yaml
apiVersion: core.gardener.cloud/v1beta1
kind: ControllerInstallation
metadata:
  name: os-gardenlinux
spec:
  deploymentRef:
    name: os-gardenlinux
  registrationRef:
    name: os-gardenlinux
  seedRef:
    name: aws-eu1
```

This resource expresses that Gardener requires the `os-gardenlinux` extension controller to run on the `aws-eu1` seed cluster.

The Gardener Controller Manager does automatically determine which extension is required on which seed cluster and will only create `ControllerInstallation` objects for those.
Also, it will automatically delete `ControllerInstallation`s referencing extension controllers that are no longer required on a seed (e.g., because all shoots on it have been deleted).
There are additional configuration options, please see the [Deployment Configuration Options section](#deployment-configuration-options).

## How do extension controllers get deployed to seeds?

After Gardener has written the `ControllerInstallation` resource, some component must satisfy this request and start deploying the extension controller to the seed.
Depending on the complexity of the controller's lifecycle management, configuration, etc., there are two possible scenarios:

### Scenario 1: Deployed by Gardener

In many cases, the extension controllers are easy to deploy and configure.
It is sufficient to simply create a Helm chart (standardized way of packaging software in the Kubernetes context) and deploy it together with some static configuration values.
Gardener supports this scenario and allows to provide arbitrary deployment information in the `ControllerDeployment` resource's `.providerConfig` section:

```yaml
...
type: helm
providerConfig:
  chart: H4sIFAAAAAAA/yk...
  values:
    foo: bar
```

If `.type=helm`, then Gardener itself will take over the responsibility the deployment.
It base64-decodes the provided Helm chart (`.providerConfig.chart`) and deploys it with the provided static configuration (`.providerConfig.values`).
The chart and the values can be updated at any time - Gardener will recognize and re-trigger the deployment process.

In order to allow extensions to get information about the garden and the seed cluster, Gardener does mix-in certain properties into the values (root level) of every deployed Helm chart:

```yaml
gardener:
  garden:
    identifier: <uuid-of-gardener-installation>
  seed:
    identifier: <seed-name>
    region: europe
    spec: <complete-seed-spec>
```

Extensions can use this information in their Helm chart in case they require knowledge about the garden and the seed environment.
The list might be extended in the future.

:information_source: Gardener uses the UUID of the `garden` `Namespace` object in the `.gardener.garden.identifier` property.

### Scenario 2: Deployed by a (Non-Human) Kubernetes Operator

Some extension controllers might be more complex and require additional domain-specific knowledge wrt. lifecycle or configuration.
In this case, we encourage to follow the Kubernetes operator pattern and deploy a dedicated operator for this extension into the garden cluster.
The `ControllerDeployments`'s `.type` field would then not be `helm`, and no Helm chart or values need to be provided there.
Instead, the operator itself knows how to deploy the extension into the seed.
It must watch `ControllerInstallation` resources and act one those referencing a `ControllerRegistration` the operator is responsible for.

In order to let Gardener know that the extension controller is ready and running in the seed, the `ControllerInstallation`'s `.status` field supports two conditions: `RegistrationValid` and `InstallationSuccessful` - both must be provided by the responsible operator:

```yaml
...
status:
  conditions:
  - lastTransitionTime: "2019-01-22T11:51:11Z"
    lastUpdateTime: "2019-01-22T11:51:11Z"
    message: Chart could be rendered successfully.
    reason: RegistrationValid
    status: "True"
    type: Valid
  - lastTransitionTime: "2019-01-22T11:51:12Z"
    lastUpdateTime: "2019-01-22T11:51:12Z"
    message: Installation of new resources succeeded.
    reason: InstallationSuccessful
    status: "True"
    type: Installed
```

Additionally, the `.status` field has a `providerStatus` section into which the operator can (optionally) put any arbitrary data associated with this installation.

## Extensions in the Garden Cluster Itself

The `Shoot` resource itself will contain some provider-specific data blobs.
As a result, some extensions might also want to run in the garden cluster, e.g., to provide `ValidatingWebhookConfiguration`s for validating the correctness of their provider-specific blobs:

```yaml
apiVersion: core.gardener.cloud/v1beta1
kind: Shoot
metadata:
  name: johndoe-aws
  namespace: garden-dev
spec:
  ...
  cloud:
    type: aws
    region: eu-west-1
    providerConfig:
      apiVersion: aws.cloud.gardener.cloud/v1alpha1
      kind: InfrastructureConfig
      networks:
        vpc: # specify either 'id' or 'cidr'
        # id: vpc-123456
          cidr: 10.250.0.0/16
        internal:
        - 10.250.112.0/22
        public:
        - 10.250.96.0/22
        workers:
        - 10.250.0.0/19
      zones:
      - eu-west-1a
...
```

In the above example, Gardener itself does not understand the AWS-specific provider configuration for the infrastructure.
However, if this part of the `Shoot` resource should be validated, then you should run an AWS-specific component in the garden cluster that registers a webhook. You can do it similarly if you want to default some fields of a resource (by using a `MutatingWebhookConfiguration`).

Again, similar to how Gardener is deployed to the garden cluster, these components must be deployed and managed by the Gardener administrator.

### `Extension` Resource Configurations

The `Extension` resource allows injecting arbitrary steps into the shoot reconciliation flow that are unknown to Gardener.
Hence, it is slightly special and allows further configuration when registering it:

```yaml
apiVersion: core.gardener.cloud/v1beta1
kind: ControllerRegistration
metadata:
  name: extension-foo
spec:
  resources:
  - kind: Extension
    type: foo
    primary: true
    globallyEnabled: true
    reconcileTimeout: 30s
    lifecycle:
      reconcile: AfterKubeAPIServer
      delete: BeforeKubeAPIServer
      migrate: BeforeKubeAPIServer
```

The `globallyEnabled=true` option specifies that the `Extension/foo` object shall be created by default for all shoots (unless they opted out by setting `.spec.extensions[].enabled=false` in the `Shoot` spec).

The `reconcileTimeout` tells Gardener how long it should wait during its shoot reconciliation flow for the `Extension/foo`'s reconciliation to finish.

#### `Extension` Lifecycle
The `lifecycle` field tells Gardener when to perform a certain action on the `Extension` resource during the reconciliation flows. If omitted, then the default behaviour will be applied. Please find more information on the defaults in the explanation below. Possible values for each control flow are `AfterKubeAPIServer` and `BeforeKubeAPIServer`. Let's take the following configuration and explain it.

```yaml
    ...
    lifecycle:
      reconcile: AfterKubeAPIServer
      delete: BeforeKubeAPIServer
      migrate: BeforeKubeAPIServer
```

 - `reconcile: AfterKubeAPIServer` means that the extension resource will be reconciled after the successful reconciliation of the `kube-apiserver` during shoot reconciliation. This is also the default behaviour if this value is not specified. During shoot hibernation, the opposite rule is applied, meaning that in this case the reconciliation of the extension will happen before the `kube-apiserver` is scaled to 0 replicas. On the other hand, if the extension needs to be reconciled before the `kube-apiserver` and scaled down after it, then the value `BeforeKubeAPIServer` should be used.
- `delete: BeforeKubeAPIServer` means that the extension resource will be deleted before the `kube-apiserver` is destroyed during shoot deletion. This is the default behaviour if this value is not specified.
- `migrate: BeforeKubeAPIServer` means that the extension resource will be migrated before the `kube-apiserver` is destroyed in the source cluster during [control plane migration](../usage/control_plane_migration.md). This is the default behaviour if this value is not specified. The restoration of the control plane follows the reconciliation control flow.

### Deployment Configuration Options

The `.spec.deployment` resource allows to configure a deployment `policy`.
There are the following policies:

* `OnDemand` (default): Gardener will demand the deployment and deletion of the extension controller to/from seed clusters dynamically. It will automatically determine (based on other resources like `Shoot`s) whether it is required and decide accordingly.
* `Always`: Gardener will demand the deployment of the extension controller to seed clusters independent of whether it is actually required or not. This might be helpful if you want to add a new component/controller to all seed clusters by default. Another use-case is to minimize the durations until extension controllers get deployed and ready in case you have highly fluctuating seed clusters.
* `AlwaysExceptNoShoots`: Similar to `Always`, but if the seed does not have any shoots, then the extension is not being deployed. It will be deleted from a seed after the last shoot has been removed from it.

Also, the `.spec.deployment.seedSelector` allows to specify a label selector for seed clusters.
Only if it matches the labels of a seed, then it will be deployed to it.
Please note that a seed selector can only be specified for secondary controllers (`primary=false` for all `.spec.resources[]`).
