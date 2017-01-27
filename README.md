# Prombeat

Prombeat periodically scrapes a [Prometheus](https://prometheus.io) server and publishes using Elastic [Beat](https://github.com/elastic/beats/), possible to output to for example Logstash or Elasticsearch. 

## Configuration

Configuration is done in the `prombeat.yml` file. Example configuration:

```yaml
prombeat:
  period: 15m
  # Matchers - See https://prometheus.io/docs/operating/federation/#configuring-federation
  matchers:
    - http_requests_total{instance="localhost:9090",job="prometheus",method="get"}
  # Queries - DEPRECATED
  queries:
    - name: series
      query: prometheus_local_storage_memory_series
    - name: http_requests
      query: sum(http_requests_total{instance="localhost:9090",job="prometheus"})
    - name: http_request_rate
      query: rate(http_requests_total{instance="localhost:9090",job="prometheus"}[15m])

output:
  elasticsearch:
    hosts: ["http://localhost:9200"]
    index: "metrics"

  console:
    pretty: true
```

This will result in documents like this:

```json
    {
      "@timestamp": "2016-10-24T13:13:29.770Z",
      "beat": {
        "hostname": "my-machine",
        "name": "my-machine"
      },
      "labels": {
        "__name__": "http_requests_total",
        "instance": "localhost:9090",
        "method": "get",
        "job": "prometheus"
      },
      "series": 461390.0,
      "type": "prombeat"
    }
```

The name of each query will be used as the key in the json for the value.
