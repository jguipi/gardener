# SecretBinding Provider Controller

This page describes the process on how to enable the SecretBinding provider controller.

## Overview

With Gardener v1.38.0, the SecretBinding resource now contains a new optional field `.provider.type` (details about the motivation can be found in https://github.com/gardener/gardener/issues/4888). To make the process of setting the new field automated and afterwards to enforce validation on the new field in backwards compatible manner, Gardener features the SecretBinding provider controller and a feature gate - `SecretBindingProviderValidation`.

## Process

A Gardener landscape operator can follow the following steps:

1. Enable the SecretBinding provider controller of Gardener Controller Manager.

   The SecretBinding provider controller is responsible for populating the `.provider.type` field of a SecretBinding based on its current usage by Shoot resources. For example, if a Shoot `crazy-botany` with `.provider.type=aws` is using a SecretBinding `my-secret-binding`, then the SecretBinding provider controller will take care to set the `.provider.type` field of the SecretBinding to the same provider type (`aws`).
   To enable the SecretBinding provider controller, set the `controller.secretBindingProvider.concurentSyncs` field in the ControllerManagerConfiguration (e.g set it to `5`).
   Although that it is not recommended, the API allows Shoots from different provider types to reference the same SecretBinding (assuming that the backing Secret contains data for both of the provider types). To preserve the backwards compatibility for such SecretBindings, the provider controller will maintain the multiple provider types in the field (it will join them with the separator `,` - for example `aws,gcp`).

2. Disable the SecretBinding provider controller and enable the `SecretBindingProviderValidation` feature gate of Gardener API server.

   The `SecretBindingProviderValidation` feature gate of Gardener API server enables a set of validations for the SecretBinding provider field. It forbids creating a Shoot that has a different provider type from the referenced SecretBinding's one. It also enforces immutability on the field.
   After making sure that SecretBinding provider controller is enabled and it populated the `.provider.type` field of a majority of the SecretBindings on a Gardener landscape (the SecretBindings that are unused will have their provider type unset), a Gardener landscape operator has to disable the SecretBinding provider controller and to enable the `SecretBindingProviderValidation` feature gate of Gardener API server. To disable the SecretBinding provider controller, set the `controller.secretBindingProvider.concurentSyncs` field in the ControllerManagerConfiguration to `0`.

## Implementation History

- Gardener v1.38: The SecretBinding resource has a new optional field `.provider.type`. The SecretBinding provider controller is disabled by default. The `SecretBindingProviderValidation` feature gate of Gardener API server is disabled by default.
- Gardener v1.42: The SecretBinding provider controller is enabled by default.
- Gardener v1.51: The `SecretBindingProviderValidation` feature gate of Gardener API server is enabled by default and the SecretBinding provider controller is disabled by default.
- Gardener v1.53: The `SecretBindingProviderValidation` feature gate of Gardener API server is unconditionally enabled (can no longer be disabled).
- Gardener v1.55: The `SecretBindingProviderValidation` feature gate of Gardener API server and the SecretBinding provider controller are removed.
