# møbius Diff Report

Commit: `deadbeef`

## Cluster `kube-bravo`

| Added | Removed | Changed |
| ---: | ---: | ---: |
| 0 | 0 | 1 |

Charts with changes: 1

<details>
<summary>Chart `hello-world` · namespace `hello-world` · added 0 · removed 0 · changed 1</summary>

#### Resource `Deployment/hello-world` (changed)

```diff
# Path: spec.replicas (changed)
spec:
-     replicas: 2
+     replicas: 3

# Path: spec.template.spec.containers[name=hello-world].imagePullPolicy (changed)
spec:
    template:
        spec:
            containers:
                - name: hello-world
-                     imagePullPolicy: IfNotPresent
+                     imagePullPolicy: Always
```

</details>
