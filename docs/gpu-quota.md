# GPU quota limitation

## Purpose

- Provides a QoS for GPU resource (guarantee the number of GPU devices or the specific type GPU
 devices).
 
- While scheduling pods, this component will consider the quota configuration, prevent the situation
like some users preempt others GPU resources. It's simple for accounting the cost for each user.


## Configure

### 1. label the node with GPU type

Suppose that the type of all GPU devices on node `foo` are `Tesla M40`, we label node `foo` with
 key `tkestack.io/gpu-model` by the following command:
 
```
kubectl label node foo tkestack.io/gpu-model=M40
```

### 2. label the node with pool name 

****any node can only belong to one resource pool****

Suppose that we want to add node `foo` to resource pool `public`, we can do this command:

```
kubectl label node foo tkestack.io/gpu-pool=public
```

### 3. configure the quota for each resource pool

Suppose that namespace `foo` has `8` GPU device of type `M40` on resource pool `public`, then you
 should create such a configmap like following:

```
apiVersion: v1
data:
  gpu_quota: '{"foo":{"quota":{"M40":8},"pool":["public"]}}'
kind: ConfigMap
metadata:
  name: gpuquota
  namespace: kube-system
```

