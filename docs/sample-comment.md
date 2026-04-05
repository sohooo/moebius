# møbius Diff Report

**Status:** changes detected

Commit: `deadbeef`

## Review Summary

| Clusters | Charts | Resources | Added | Removed | Changed |
| ---: | ---: | ---: | ---: | ---: | ---: |
| 1 | 1 | 1 | 0 | 0 | 1 |

**Severity:** high 1

**Highlights**

- Cluster `kube-bravo` · `Deployment/hello-world` [high]: replicas changed 2 -> 3

## Cluster `kube-bravo`

| Added | Removed | Changed |
| ---: | ---: | ---: |
| 0 | 0 | 1 |

Charts with changes: 1

<details>
<summary>Chart `hello-world` · namespace `hello-world` · severity `high` · added 0 · removed 0 · changed 1</summary>

- Kinds affected: Deployment
- Scope: value-level tweaks only
- Severity summary: high 1
- Notable changes:
  - `Deployment/hello-world` [high]: replicas changed 2 -> 3

#### Resource `Deployment/hello-world` (changed, severity: high)

- replicas changed 2 -> 3

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
