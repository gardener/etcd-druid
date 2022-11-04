## Quorum loss in ETCD Cluster
[Quorum loss](https://etcd.io/docs/v3.4/op-guide/recovery/) means when majority of ETCD pods(greater than or equal to n/2 + 1) are down simultaneously for some reason. 

There are two types of quorum loss that can happen to [ETCD multinode cluster](https://github.com/gardener/etcd-druid/tree/master/docs/proposals/multi-node) :

1. **Transient quorum loss** - A quorum loss is called transient when majority of ETCD pods are down simultaneously for some time. The pods may be down due to network unavailability, high resource usages etc. When the pods come back after some time, they can re-join to the cluster and the quorum is recovered automatically without any manual intervention. There should not be a permanent failure for majority of etcd pods due to hardware failure or disk corruption.

2. **Permanent quorum loss** - A quorum loss is called permanent when majority of ETCD cluster members experience permanent failure, whether due to hardware failure or disk corruption etc. then the etcd cluster is not going to recover automatically from the quorum loss. A human operator will now need to intervene and execute the following steps to recover the multi-node ETCD cluster.

If permanent quorum loss occurs to a multinode ETCD cluster, the operator needs to note down the PVCs, configmaps, statefulsets, CRs etc related to that ETCD cluster and work on those resources only. Following steps guide a human operator to recover from permanent quorum loss of a ETCD cluster. We assume the name of the ETCD CR for the ETCD cluster is `etcd-main`.

**ETCD cluster in shoot control plane of gardener deployment:**
There are two [ETCD clusters](https://github.com/gardener/etcd-druid/tree/master/docs/proposals/multi-node) running in shoot control plane. One is named as `etcd-events` and another is named `etcd-main`. The operator needs to take care of permanent quorum loss to a specific cluster. If permanent quorum loss occurs to `etcd-events` cluster, the operator needs to note down the PVCs, configmaps, statefulsets, CRs etc related to `etcd-events` cluster and work on those resources only. 

:warning: **Note:** Please note that manually restoring etcd can result in data loss. This guide is the last resort to bring an ETCD cluster up and running again.

   If etcd-druid and etcd-backup-restore is being used with gardener, then 

   Target the control plane of affected shoot cluster via `kubectl`. Alternatively, you can use [gardenctl](https://github.com/gardener/gardenctl-v2) to target the control plane of the affected shoot cluster. You can get the details to target the control plane from the Access tile in the shoot cluster details page on the Gardener dashboard. Ensure that you are targeting the correct namespace.
   
   1. Add the following annotation to the `Etcd` resource `kubectl annotate etcd etcd-main druid.gardener.cloud/ignore-reconciliation="true"`
   2. Note down the configmap name that is attached to the `etcd-main` statefulset. If you describe the statefulset with `kubectl describe sts etcd-main`, look for the lines similar to following lines to identify attached configmap name. It will be needed at later stages:

   ```
     Volumes:
      etcd-config-file:
       Type:      ConfigMap (a volume populated by a ConfigMap)
       Name:      etcd-bootstrap-4785b0
       Optional:  false
   ```

      Alternatively, the related configmap name can be obtained by executing following command as well:
   `kubectl get sts etcd-main -o jsonpath='{.spec.template.spec.volumes[?(@.name=="etcd-config-file")].configMap.name}'`

   3. Scale down the `etcd-main` statefulset replicas to `0`

      `kubectl scale sts etcd-main --replicas=0`
   4. The PVCs will look like the following on listing them with the command `kubectl get pvc` :
      
      ```
      main-etcd-etcd-main-0        Bound    pv-shoot--garden--aws-ha-dcb51848-49fa-4501-b2f2-f8d8f1fad111   80Gi       RWO            gardener.cloud-fast   13d
      main-etcd-etcd-main-1        Bound    pv-shoot--garden--aws-ha-b4751b28-c06e-41b7-b08c-6486e03090dd   80Gi       RWO            gardener.cloud-fast   13d
      main-etcd-etcd-main-2        Bound    pv-shoot--garden--aws-ha-ff17323b-d62e-4d5e-a742-9de823621490   80Gi       RWO            gardener.cloud-fast   13d
      ```
      Delete all PVCs that are attached to `etcd-main` cluster.

      `kubectl delete pvc -l instance=etcd-main`
   5. Edit the `etcd-main` cluster's configmap (ex: `etcd-bootstrap-4785b0`) as follows:
      
      Find the `initial-cluster` field in the configmap. It will look like the following:
      ```
      # Initial cluster
        initial-cluster: etcd-main-0=https://etcd-main-0.etcd-main-peer.default.svc:2380,etcd-main-1=https://etcd-main-1.etcd-main-peer.default.svc:2380,etcd-main-2=https://etcd-main-2.etcd-main-peer.default.svc:2380
      ```

      Change the `initial-cluster` field to have only one member (`etcd-main-0`) in the string. It should now look like this:

      ```
      # Initial cluster
        initial-cluster: etcd-main-0=https://etcd-main-0.etcd-main-peer.default.svc:2380
      ```
   6. Scale up the `etcd-main` statefulset replicas to `1`

      `kubectl scale sts etcd-main --replicas=1`
   7. Wait for the single-member etcd cluster to be completely ready.

      `kubectl get pods etcd-main-0` will give the following output when ready:
      ```
      NAME          READY   STATUS    RESTARTS   AGE
      etcd-main-0   2/2     Running   0          1m
      ```
   8. Add the following annotation to the `etcd-main` statefulset `kubectl annotate sts etcd-main gardener.cloud/scaled-to-multi-node="true"`
   9. Remove the following annotation from the `Etcd` resource `etcd-main`: `kubectl annotate etcd etcd-main druid.gardener.cloud/ignore-reconciliation-`
   10. Finally add the following annotation to the `Etcd` resource `etcd-main`: `kubectl annotate etcd etcd-main gardener.cloud/operation="reconcile"`
   11. Verify that the etcd cluster is formed correctly.

       All the `etcd-main` pods will have outputs similar to following:
       ```
       NAME          READY   STATUS    RESTARTS   AGE
       etcd-main-0   2/2     Running   0          5m
       etcd-main-1   2/2     Running   0          1m
       etcd-main-2   2/2     Running   0          1m
       ```
       Additionally, check if the ETCD CR is ready with `kubectl get etcd etcd-main` :
       ```                                                                                                                                                 ✹
       NAME        READY   AGE
       etcd-main   true    13d
       ```

       Additionally, check the leases for 30 seconds at least. There should be leases starting with `etcd-main` as many as `etcd-main` replicas. One of those leases will have holder identity as `<etcd-member-id>:Leader` and rest of those leases have holder identities as `<etcd-member-id>:Member`. The `AGE` of those leases can also be inspected to identify if those leases were updated in conjunction with the restart of the ETCD cluster: Example:
       
       ```
       NAME        HOLDER                  AGE
       etcd-main-0 4c37667312a3912b:Member 1m
       etcd-main-1 75a9b74cfd3077cc:Member 1m
       etcd-main-2 c62ee6af755e890d:Leader 1m
       ```