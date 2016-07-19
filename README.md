## High availability redis cluster in kubernetes

Run redis cluster with sentinel provisioner with autodetect master server in kubernetes cluster.


### HowTo run

    kubectl --namespace=mynamespace create -f mynamespace.yaml
    kubectl --namespace=mynamespace create -f sentinel-service.yaml
    kubectl --namespace=mynamespace create -f sentinel-controller.yaml
    kubectl --namespace=mynamespace create -f controller.yaml


Usage /kubernetes-redis cli

    Usage of /kubernetes-redis:
      -alsologtostderr
            log to standard error as well as files
      -down-after-milliseconds uint
            Sentinel down after milliseconds (default 60000)
      -failover-timeout uint
            Sentinel failover timeout (default 180000)
      -labels value
            --labels key1=value1 --labels key2=value2 ... (default &main.cmdLabels(nil))
      -log_backtrace_at value
            when logging hits line file:N, emit a stack trace (default :0)
      -log_dir string
            If non-empty, write log files in this directory
      -logtostderr
            log to standard error instead of files
      -master-name string
            Sentinel master name (default "redis-master")
      -namespace string
            namespace (default "default")
      -parallel-syncs uint
            Sentinel parallel syncs (default 1)
      -quorum uint
            Sentinel master quorum (default 2)
      -sentinel
            Sentinel
      -sentinel-service string
            Sentinel service name (default "redis-sentinel")
      -stderrthreshold value
            logs at or above this threshold go to stderr
      -v value
            log level for V logs
      -vmodule value
            comma-separated list of pattern=N settings for file-filtered logging


mynamespace.yaml

    apiVersion: v1
    kind: Namespace
    metadata:
      name: mynamespace

sentinel-controller.yaml

    apiVersion: v1
    kind: ReplicationController
    metadata:
      name: redis-sentinel
      namespace: mynamespace
    spec:
      replicas: 3
      selector:
        redis-sentinel: "true"
      template:
        metadata:
          labels:
            name: redis-sentinel
          namespace: mynamespace
        spec:
          containers:
            - name: sentinel
              image: suquant/redisd:3.2.1.kube
              command:
                - /kubernetes-redis
              args:
                - --namespace
                - mynamespace
                - --sentinel
                - --labels
                - name=redis
                - --
                - --bind
                - 0.0.0.0
                - --protected-mode
                - "no"
              ports:
                - containerPort: 26379


sentinel-service.yaml

    apiVersion: v1
    kind: Service
    metadata:
      labels:
        name: redis-sentinel
      name: redis-sentinel
      namespace: mynamespace
    spec:
      ports:
        - port: 26379
          targetPort: 26379
      selector:
        name: redis-sentinel


redis-controller.yaml

    apiVersion: v1
    kind: ReplicationController
    metadata:
      name: redis
      namespace: mynamespace
    spec:
      replicas: 3
      selector:
        name: redis
      template:
        metadata:
          namespace: mynamespace
          labels:
            name: redis
        spec:
          containers:
          - name: redis
            image: suquant/redisd:3.2.1.kube
            command:
              - /kubernetes-redis
            args:
              - --namespace
              - mynamespace
              - --labels
              - name=redis
              - --
              - --bind
              - 0.0.0.0
              - --port
              - "6379"
              - --tcp-backlog
              - "511"
              - --timeout
              - "0"
              - --tcp-keepalive
              - "300"
              - --loglevel
              - notice
              - --logfile
              - ""
              - --databases
              - "16"
              - --save
              - 900 1
              - --save
              - 300 10
              - --save
              - 60 10000
              - --rdbcompression
              - "yes"
              - --rdbchecksum
              - "yes"
              - --dbfilename
              - dump.rdb
              - --dir
              - /data
              - --slave-serve-stale-data
              - "yes"
              - --slave-read-only
              - "yes"
              - --repl-diskless-sync
              - "no"
              - --repl-diskless-sync-delay
              - "5"
              - --repl-disable-tcp-nodelay
              - "no"
              - --slave-priority
              - "100"
              - --appendonly
              - "yes"
              - --appendfilename
              - appendonly.aof
              - --appendfsync
              - everysec
              - --no-appendfsync-on-rewrite
              - "no"
              - --auto-aof-rewrite-percentage
              - "100"
              - --auto-aof-rewrite-min-size
              - 64mb
              - --aof-load-truncated
              - "yes"
              - --lua-time-limit
              - "5000"
              - --slowlog-log-slower-than
              - "10000"
              - --slowlog-max-len
              - "128"
              - --latency-monitor-threshold
              - "0"
              - --notify-keyspace-events
              - ""
              - --hash-max-ziplist-entries
              - "512"
              - --hash-max-ziplist-value
              - "64"
              - --list-max-ziplist-entries
              - "512"
              - --list-max-ziplist-value
              - "64"
              - --set-max-intset-entries
              - "512"
              - --zset-max-ziplist-entries
              - "128"
              - --zset-max-ziplist-value
              - "64"
              - --hll-sparse-max-bytes
              - "3000"
              - --activerehashing
              - "yes"
              - --client-output-buffer-limit
              - normal
              - "0"
              - "0"
              - "0"
              - --client-output-buffer-limit
              - slave
              - 256mb
              - 64mb
              - "60"
              - --client-output-buffer-limit
              - pubsub
              - 32mb
              - 8mb
              - "60"
              - --hz
              - "10"
              - --aof-rewrite-incremental-fsync
              - "yes"
              - --protected-mode
              - "no"
            ports:
              - containerPort: 6379
            resources:
              limits:
                cpu: "0.1"
                memory: "10Mi"
            volumeMounts:
              - mountPath: /data
                name: data
          volumes:
            - name: data
              emptyDir: {}