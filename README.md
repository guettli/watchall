# Watch all Resources in a Kubernetes Cluster

`watchall` is a tool which records changes to Kubernetes resources.

## Step 1: Record

Be sure that `KUBECONFIG` is set correctly.

```bash
go run github.com/guettli/watchall@latest record
```

The `record` command will dump all resources of the cluster into the directory
`watchall-output/your-cluster:port`.

```log
Watching "" "serviceaccounts"
Watching "" "configmaps"
...
ADDED Node /apo-e2e-control-plane
ADDED ConfigMap argocd/kube-root-ca.crt
...
```

Then the tool waits for changes:

```log
MODIFIED Event foo-system/foo-manager-64756cd977-gbnk5.182813b2d92d9874
...
```

You can have a look at the manifests:

```log
find watchall-output/

watchall-output/127.0.0.1:41209/ConfigMap
watchall-output/127.0.0.1:41209/ConfigMap/kube-node-lease
watchall-output/127.0.0.1:41209/ConfigMap/kube-node-lease/kube-root-ca.crt
watchall-output/127.0.0.1:41209/ConfigMap/kube-node-lease/kube-root-ca.crt/20250227-120853.212.yaml
...
```

As soon as a resource gets changed, the tool creates a new file with a new timestamp.

Data in secrets get redacted with the sha256 hash.

## Step 2: Show Deltas

If you are interested how resources change over time, use the `deltas` sub-command:

```text
❯ go run github.com/guettli/watchall@latest deltas -h

This reads the files from the local disk and shows the changes. No connection to a cluster is needed.

Usage:
  watchall deltas dir [flags]

Flags:
  -h, --help           help for deltas
      --only strings   comma separated list of regex patterns to show
      --skip strings   comma separated list of regex patterns to skip

Global Flags:
  -o, --outdir string   Directory to store output (default "watchall-output")
  -v, --verbose         Create more output
```

Example:

```sh
go run github.com/guettli/watchall@latest deltas watchall-output/127.0.0.1:41209/
```

```diff
Using "watchall-output/127.0.0.1:41209/record-20250227-152147.53092" as start timestamp
Diff of "Pod/foo-system/foo-manager-64756cd977-jx76c/20250227-152147.59958.yaml" "20250227-152234.11902.yaml"
--- 20250227-152147.59958.yaml
+++ 20250227-152234.11902.yaml
@@ -16,7 +16,7 @@
     kind: ReplicaSet
     name: foo-manager-64756cd977
     uid: 5106005f-af0b-435a-b152-12475e442f4a
-  resourceVersion: "724012"
+  resourceVersion: "725443"
   uid: 73e53f49-9f41-4bb5-a233-69f00e2dd2d0
 spec:
   containers:
@@ -159,24 +159,23 @@
     status: "True"
     type: PodScheduled
   containerStatuses:
-  - containerID: containerd://f607f6a8c928d143a77fd593c4fc7f49be8dcb0a8e7fc73b4cfc06da9b90ff00
-    image: ghcr.io/example/foo-manager:v1.4.0-beta.5
-    imageID: ghcr.io/example/foo-manager@sha256:cc899be1d48d5f61a784f240a4db63302d546a401929dda3fd46528e3e535e6e
-    lastState:
-      terminated:
-        containerID: containerd://f607f6a8c928d143a77fd593c4fc7f49be8dcb0a8e7fc73b4cfc06da9b90ff00
-        exitCode: 1
-        finishedAt: "2025-02-27T14:17:24Z"
-        reason: Error
-        startedAt: "2025-02-27T14:17:05Z"
-    name: manager
-    ready: false
-    restartCount: 14
-    started: false
-    state:
-      waiting:
-        message: back-off 5m0s restarting failed container=manager pod=foo-manager-64756cd977-jx76c_foo-system(73e53f49-9f41-4bb5-a233-69f00e2dd2d0)
+  - containerID: containerd://1a655f083606a3b0347e05966762c255baa1826842b58369b00bce543571c3a7
+    image: ghcr.io/example/foo-manager:v1.4.0-beta.5
+    imageID: ghcr.io/example/foo-manager@sha256:cc899be1d48d5f61a784f240a4db63302d546a401929dda3fd46528e3e535e6e
+    lastState:
+      terminated:
+        containerID: containerd://f607f6a8c928d143a77fd593c4fc7f49be8dcb0a8e7fc73b4cfc06da9b90ff00
+        exitCode: 1
+        finishedAt: "2025-02-27T14:17:24Z"
+        reason: Error
+        startedAt: "2025-02-27T14:17:05Z"
+    name: manager
+    ready: false
+    restartCount: 15
+    started: true
+    state:
+      running:
-        reason: CrashLoopBackOff
+        startedAt: "2025-02-27T14:22:34Z"
   hostIP: 172.18.0.2
   hostIPs:
   - ip: 172.18.0.2

...
```

Every time you start `record` a new record-TIMESTAMP file gets created. When you run `deltas` only
the last changes get shown.

TODO: Command line argument to define custom starttimestamps, or make the user choose one.

## Usage

[Usage](https://github.com/guettli/watchall/blob/main/usage.md)

## Related

[guettli/check-conditions: Check Conditions of all Kubernets Resources](https://github.com/guettli/check-conditions)
