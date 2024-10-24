# Add A New Etcd Cluster Component

`etcd-druid` defines an [Operator](https://github.com/gardener/etcd-druid/blob/3383e0219a6c21c6ef1d5610db964cc3524807c8/internal/component/types.go#L42) which is responsible for creation, deletion and update of a resource that is created for an `Etcd` cluster. If you want to introduce a new resource for an `Etcd` cluster then you must do the following:

* Add a dedicated `package` for the resource under [component](https://github.com/gardener/etcd-druid/tree/3383e0219a6c21c6ef1d5610db964cc3524807c8/internal/component).

* Implement `Operator` interface.

* Define a new [Kind](https://github.com/gardener/etcd-druid/blob/3383e0219a6c21c6ef1d5610db964cc3524807c8/internal/component/registry.go#L19) for this resource in the operator [Registry](https://github.com/gardener/etcd-druid/blob/3383e0219a6c21c6ef1d5610db964cc3524807c8/internal/component/registry.go#L8).

* Every resource a.k.a `Component` needs to have the following set of default labels:

  * `app.kubernetes.io/name` - value of this label is the name of this component. Helper functions are defined [here](https://github.com/gardener/etcd-druid/blob/master/api/v1alpha1/helper.go) to create the name of each component using the parent `Etcd` resource. Please define a new helper function to generate the name of your resource using the parent `Etcd` resource.
  * `app.kubernetes.io/component` - value of this label is the type of the component. All component type label values are defined [here](https://github.com/gardener/etcd-druid/blob/3383e0219a6c21c6ef1d5610db964cc3524807c8/internal/common/constants.go) where you can add an entry for your component.
  * In addition to the above component specific labels, each resource/component should have default labels defined on the `Etcd` resource. You can use [GetDefaultLabels](https://github.com/gardener/etcd-druid/blob/3383e0219a6c21c6ef1d5610db964cc3524807c8/api/v1alpha1/helper.go#L124) function.

  > These labels are also part of [recommended labels](https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/) by kubernetes.
  > NOTE: Constants for the label keys are already defined [here](https://github.com/gardener/etcd-druid/blob/3383e0219a6c21c6ef1d5610db964cc3524807c8/api/v1alpha1/constants.go).

* Ensure that there is no `wait` introduced in any `Operator` method implementation in your component. In case there are multiple steps to be executed in a sequence then re-queue the event with a special [error code](https://github.com/gardener/etcd-druid/blob/3383e0219a6c21c6ef1d5610db964cc3524807c8/internal/errors/errors.go#L19) in case there is an error or if the pre-conditions check to execute the next step are not yet satisfied.

* All errors should be wrapped with a custom [DruidError](https://github.com/gardener/etcd-druid/blob/3383e0219a6c21c6ef1d5610db964cc3524807c8/internal/errors/errors.go#L24).
