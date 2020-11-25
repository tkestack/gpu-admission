# GPU admission

It is a [scheduler extender](https://github.com/kubernetes/community/blob/master/contributors/design-proposals/scheduling/scheduler_extender.md) for GPU admission.
It provides the following features:

- provides quota limitation according to GPU device type
- avoids fragment allocation of node by working with [gpu-manager](https://github.com/tkestack/gpu-manager)

> For more details, please refer to the documents in `docs` directory in this project


## 1. Build

```
$ make build
```

## 2. Run

### 2.1 Run gpu-admission.

```
$ bin/gpu-admission --address=127.0.0.1:3456 --v=4 --kubeconfig <your kubeconfig> --logtostderr=true
```

Other options

```
      --address string                   The address it will listen (default "127.0.0.1:3456")
      --alsologtostderr                  log to standard error as well as files
      --kubeconfig string                Path to a kubeconfig. Only required if out-of-cluster.
      --log-backtrace-at traceLocation   when logging hits line file:N, emit a stack trace (default :0)
      --log-dir string                   If non-empty, write log files in this directory
      --log-flush-frequency duration     Maximum number of seconds between log flushes (default 5s)
      --logtostderr                      log to standard error instead of files (default true)
      --master string                    The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.
      --pprofAddress string              The address for debug (default "127.0.0.1:3457")
      --stderrthreshold severity         logs at or above this threshold go to stderr (default 2)
  -v, --v Level                          number for the log level verbosity
      --version version[=true]           Print version information and quit
      --vmodule moduleSpec               comma-separated list of pattern=N settings for file-filtered logging
```

### 2.2 Configure kube-scheduler policy file, and run a kubernetes cluster.

Example for scheduler-policy-config.json:
```
{
  "kind": "Policy",
  "apiVersion": "v1",
  "predicates": [
    {
      "name": "PodFitsHostPorts"
    },
    {
      "name": "PodFitsResources"
    },
    {
      "name": "NoDiskConflict"
    },
    {
      "name": "MatchNodeSelector"
    },
    {
      "name": "HostName"
    }
  ],
  "extenders": [
    {
      "urlPrefix": "http://<gpu-admission ip>:<gpu-admission port>/scheduler",
      "apiVersion": "v1beta1",
      "filterVerb": "predicates",
      "enableHttps": false,
      "nodeCacheCapable": false
    }
  ],
  "hardPodAffinitySymmetricWeight": 10,
  "alwaysCheckAllPredicates": false
}
```

Do not forget to add config for scheduler: `--policy-config-file=XXX --use-legacy-policy-config=true`.
Keep this extender as the last one of all scheduler extenders.
