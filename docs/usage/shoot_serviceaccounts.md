# `ServiceAccount` Configurations for Shoot Clusters

The `Shoot` specification allows to configure some of the settings for the handling of `ServiceAccount`s:

```yaml
apiVersion: core.gardener.cloud/v1beta1
kind: Shoot
spec:
  kubernetes:
    kubeAPIServer:
      serviceAccountConfig:
        issuer: foo
        acceptedIssuers:
        - foo1
        - foo2
        extendTokenExpiration: true
        maxTokenExpiration: 45d
...
```

## Issuer and Accepted Issuers

The `.spec.kubernetes.kubeAPIServer.serviceAccountConfig.{issuer,acceptedIssuers}` fields are translated to the `--service-account-issuer` flag for the `kube-apiserver`.
The issuer will assert its identifier in the `iss` claim of the issued tokens.
According to the [upstream specification](https://kubernetes.io/docs/reference/command-line-tools-reference/kube-apiserver/), values need to meet the following requirements:

> This value is a string or URI. If this option is not a valid URI per the OpenID Discovery 1.0 spec, the ServiceAccountIssuerDiscovery feature will remain disabled, even if the feature gate is set to true. It is highly recommended that this value comply with the OpenID spec: https://openid.net/specs/openid-connect-discovery-1_0.html. In practice, this means that service-account-issuer must be an https URL. It is also highly recommended that this URL be capable of serving OpenID discovery documents at {service-account-issuer}/.well-known/openid-configuration.

By default, Gardener uses the internal cluster domain as issuer (e.g., `https://api.foo.bar.example.com`).
If you specify the `issuer`, then this default issuer will always be part of the list of accepted issuers (you don't need to specify it yourself).

⚠️ Caution: If you change from the default issuer to a custom `issuer`, all previously issued tokens will still be valid/accepted.
However, if you change from a custom `issuer` `A` to another `issuer` `B` (custom or default), then you have to add `A` to the `acceptedIssuers` so that previously issued tokens are not invalidated.
Otherwise, the control plane components as well as system components and your workload pods might fail.
You can remove `A` from the `acceptedIssuers` when all currently active tokens have been issued solely by `B`.
This can be ensured by using [projected token volumes](https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/#service-account-token-volume-projection) with a short validity, or by rolling out all pods.
Additionally, all [`ServiceAccount` token secrets](https://kubernetes.io/docs/concepts/configuration/secret/#service-account-token-secrets) should be recreated.
Apart from this, you should wait for at least `12h` to make sure the control plane and system components have received a new token from Gardener.

## Token Expirations

The `.spec.kubernetes.kubeAPIServer.serviceAccountConfig.extendTokenExpiration` configures the `--service-account-extend-token-expiration` flag of the `kube-apiserver`.
It is enabled by default and has the following specification:

> Turns on projected service account expiration extension during token generation, which helps safe transition from legacy token to bound service account token feature. If this flag is enabled, admission injected tokens would be extended up to 1 year to prevent unexpected failure during transition, ignoring value of service-account-max-token-expiration.

The `.spec.kubernetes.kubeAPIServer.serviceAccountConfig.maxTokenExpiration` configures the `--service-account-max-token-expiration` flag of the `kube-apiserver`.
It has the following specification:

> The maximum validity duration of a token created by the service account token issuer. If an otherwise valid TokenRequest with a validity duration larger than this value is requested, a token will be issued with a validity duration of this value.

⚠️ Note that the value for this field must be in the `[30d,90d]` range.
The background for this limitation is that all Gardener components rely on the `TokenRequest` API and the Kubernetes service account token projection feature with short-lived, auto-rotating tokens.
Any values lower than `30d` risk impacting the SLO for shoot clusters, and any values above `90d` violate security best practices with respect to maximum validity of credentials before they must be rotated.
Given that the field just specifies the upper bound, end-users can still use lower values for their individual workload by specifying the `.spec.volumes[].projected.sources[].serviceAccountToken.expirationSeconds` in the `PodSpec`s.
