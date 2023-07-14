# Feature Flags in Etcd-Druid

This page contains an overview of the various feature flags an administrator can specify on etcd-druid.

## Overview

Feature flags are a set of key=value pairs that describe etcd-druid features. You can turn these features on or off by passing them as CLI flags to the etcd-druid command.

The following tables are a summary of the feature flags that you can set on etcd-druid.

* The “CLI Flag” column contains the CLI flag name that must be passed to the etcd-druid command.
* The “Since” column contains the etcd-druid release when a feature is introduced or its release stage is changed.
* The “Until” column, if not empty, contains the last etcd-druid release in which you can still use a feature flag.
* If a feature is in the *Alpha* or *Beta* state, you can find the feature listed in the Alpha/Beta feature flag table.
* If a feature is stable you can find all stages for that feature listed in the Graduated/Deprecated feature flag table.
* The Graduated/Deprecated feature flag table also lists deprecated and withdrawn features.

## Feature Flags for Alpha or Beta Features

| Feature          | CLI Flag           | Default | Stage   | Since  | Until |
|------------------|--------------------|---------|---------|--------|-------|
| `UseEtcdWrapper` | `use-etcd-wrapper` | `false` | `Alpha` | `0.19` |       |

## Feature Flags for Graduated or Deprecated Features

| Feature | Default | Stage | Since | Until |
|---------|---------|-------|-------|-------|

## Using a Feature

A feature can be in *Alpha*, *Beta* or *GA* stage.
An *Alpha* feature means:

* Disabled by default.
* Might be buggy. Enabling the feature may expose bugs.
* Support for feature may be dropped at any time without notice.
* The API may change in incompatible ways in a later software release without notice.
* Recommended for use only in short-lived testing clusters, due to increased
  risk of bugs and lack of long-term support.

A *Beta* feature means:

* Enabled by default.
* The feature is well tested. Enabling the feature is considered safe.
* Support for the overall feature will not be dropped, though details may change.
* The schema and/or semantics of objects may change in incompatible ways in a
  subsequent beta or stable release. When this happens, we will provide instructions
  for migrating to the next version. This may require deleting, editing, and
  re-creating API objects. The editing process may require some thought.
  This may require downtime for applications that rely on the feature.
* Recommended for only non-critical uses because of potential for
  incompatible changes in subsequent releases.

> Please do try *Beta* features and give feedback on them!
> After they exit beta, it may not be practical for us to make more changes.

A *General Availability* (GA) feature is also referred to as a *stable* feature. It means:

* The feature is always enabled; you cannot disable it.
* The corresponding feature flag is no longer needed.
* Stable versions of features will appear in released software for many subsequent versions.

## List of Feature Flags

| Feature          | Description                                                                                                                                                                                   |
|------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `UseEtcdWrapper` | Enables the use of etcd-wrapper image and a compatible version of etcd-backup-restore, along with component-specific configuration changes necessary for the usage of the etcd-wrapper image. |
