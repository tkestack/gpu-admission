# GPU admission

It is a [scheduler extender](https://github.com/kubernetes/community/blob/master/contributors/design-proposals/scheduling/scheduler_extender.md) for GPU admission.
It provides the following features:

- provides quota limitation according to GPU device type
- avoids fragment allocation of node by working with [gpu-manager](https://github.com
/tkestack/gpu-manager)

> For more details, please refer to the documents in `docs` directory in this project


## 1. Build

```
$ make build
```

## 2. Run

### 2.1 Run gpu-admission.

```
$ bin/gpu-admission --address=127.0.0.1:3456 --v=4 --config=build/gpu-admission.config --master=127.0.0.1:8080
```

### 2.2 Configure kube-scheduler policy file, and run a kubernetes cluster.

Example for scheduler-policy-config.json:
```
{
"kind" : "Policy",
"apiVersion" : "v1",
"extenders" : [
  	{
          "urlPrefix": "http://127.0.0.1:3456/scheduler",
          "filterVerb": "predicates",
          "enableHttps": false,
          "nodeCacheCapable": false
  	}
    ],
"hardPodAffinitySymmetricWeight" : 10,
"alwaysCheckAllPredicates" : false
}
```

Do not forget to add config for scheduler: `--policy-config-file=XXX --use-legacy-policy-config=true`.
Keep this extender as the last one of all scheduler extenders.
