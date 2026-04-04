## Cluster `kube-bravo`

| Added | Removed | Changed |
| ---: | ---: | ---: |
| 0 | 0 | 1 |

### Chart `hello-world`

- Namespace: `hello-world`

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
