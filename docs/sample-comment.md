# møbius Diff Report

**Status:** changes detected

---

**Navigation**

- [kube-bravo](#cluster-kube-bravo) · added 0 · removed 0 · changed 1

<a id="cluster-kube-bravo"></a>
## Cluster `kube-bravo`

Charts with changes: 1

<a id="chart-kube-bravo-hello-world"></a>
<details>
<summary>Chart `hello-world` · namespace `hello-world` · severity `high` · added 0 · removed 0 · changed 1</summary>

| Signal | Details |
| --- | --- |
| **Summary** | 1 resource affected · highest severity 🟠 high |
| **Kinds** | `Deployment` |
| **Change mix** | +0 · -0 · ~1 |
| **Surface** | workload |
| **Scope** | value-level tweaks only |
| **Severity** | 🟠 high 1 |
| **Validation** | 0 errors · 0 warnings · 0 unvalidated |

**Changes**
- 🟠 [`Deployment/hello-world`](#resource-kube-bravo-deployment-hello-world) **high** · replicas changed 2 -> 3

<a id="resource-kube-bravo-deployment-hello-world"></a>
#### Resource `kube-bravo · Deployment/hello-world` (changed, severity: high)

- changed · severity 🟠 high · validation: validated via embedded · [up](#chart-kube-bravo-hello-world)

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

---

_Report compares merge-base and current MR state | validation: clean | commit: `deadbeef` | [pipeline](https://gitlab.example/pipelines/123)._
