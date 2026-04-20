# møbius Diff Report

**Status:** changes detected

Commit: `deadbeef`

## Review Summary

| Clusters | Charts | Resources | Added | Removed | Changed |
| ---: | ---: | ---: | ---: | ---: | ---: |
| 1 | 1 | 1 | 0 | 0 | 1 |

**Severity:** high 1

**Validation:** 0 errors, 0 warnings, 0 unvalidated

**Highlights**

| Severity | Cluster | Resource | Finding |
| --- | --- | --- | --- |
| 🟠 high | `kube-bravo` | [`Deployment/hello-world`](#resource-kube-bravo-deployment-hello-world) | replicas changed 2 -> 3 |

**Navigation**

- [kube-bravo](#cluster-kube-bravo) · added 0 · removed 0 · changed 1

<a id="cluster-kube-bravo"></a>
## Cluster `kube-bravo`

| Added | Removed | Changed |
| ---: | ---: | ---: |
| 0 | 0 | 1 |

Charts with changes: 1

<a id="chart-kube-bravo-hello-world"></a>
<details>
<summary>Chart `hello-world` · namespace `hello-world` · severity `high` · added 0 · removed 0 · changed 1</summary>

- Summary: 1 resource affected · highest severity high
- Kinds affected: Deployment
- Scope: value-level tweaks only
- Severity summary: high 1
- Notable changes:
  - `Deployment/hello-world` [high]: replicas changed 2 -> 3

<a id="resource-kube-bravo-deployment-hello-world"></a>
#### Resource `kube-bravo · Deployment/hello-world` (changed, severity: high)

- validation coverage: validated via embedded
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
