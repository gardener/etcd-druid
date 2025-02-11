# Change the API

This guide provides detailed information on what needs to be done when the API needs to be changed.

`etcd-druid` API follows the same API conventions and guidelines which Kubernetes defines and adopts. The Kubernetes [API Conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md) as well as [Changing the API](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api_changes.md) topics already provide a good overview and general explanation of the basic concepts behind it. We adhere to the principles laid down by Kubernetes.

## Etcd Druid API

The etcd-druid API is defined [here](https://github.com/gardener/etcd-druid/tree/3383e0219a6c21c6ef1d5610db964cc3524807c8/api).

!!! info
    The current version of the API is `v1alpha1`. We are currently working on migration to `v1beta1` API.

### Changing the API

If there is a need to make changes to the API, then one should do the following:

* If new fields are added then ensure that these are added as `optional` fields. They should have the `+optional` comment and an `omitempty` JSON tag should be added against the field.
* Ensure that all new fields or changing the existing fields are well documented with doc-strings.
* Care should be taken that incompatible API changes should not be made in the same version of the API. If there is a real necessity to introduce a backward incompatible change then a newer version of the API should be created and an [API conversion webhook](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definition-versioning/#webhook-conversion) should be put in place to support more than one version of the API.
* After the changes to the API are finalized, run `make generate` to ensure that the changes are also reflected in the CRD.
* If necessary, implement or adapt the validation for the API.
* If necessary, adapt the [samples](https://github.com/gardener/etcd-druid/tree/master/samples) YAML manifests.
* When opening a pull-request, always add a release note informing the end-users of the changes that are coming in.

### Removing a Field

If field(s) needs to be removed permanently from the API, then one should ensure the following:

* Field should not be directly removed, instead first a deprecation notice should be put which should follow a well-defined deprecation period. Ensure that the release note in the pull-request is properly categorized so that this is easily visible to the end-users and clearly mentiones which field(s) have been deprecated. Clearly suggest a way in which clients need to adapt.
* To allow sufficient time to the end-users to adapt to the API changes, deprecated field(s) should only be removed once the deprecation period is over. It is generally recommended that this be done in 2 stages:
  * *First stage:* Remove the code that refers to the deprecated fields. This ensures that the code no longer has dependency on the deprecated field(s).
  * *Second Stage:* Remove the field from the API.
