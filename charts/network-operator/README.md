# Helm install

Network operator can be installed via a helm chart. Most of its parameters can be modified with helm values.

The most important values are defined below:
|Name|Type|Default|
|---|---|---|
|config.gaudi.enabled|Install Gaudi CR alongside of the operator|false|
|config.gaudi.mode|Gaudi operational mode, L2 or L3|L3|
|config.gaudi.mtu|MTU for the Gaudi network interfaces|8000|
|nfd.install|Install NFD as part of the chart|false|
|nfd.gaudiRule|Install Gaudi NFD rules|true|
|operator.image.repository|Operator container image path|intel/intel-network-operator|
|operator.image.tag|Operator container image tag|1.0.0|
|logLevel|Log level for all entities|2|


See other values in the [values.yaml](values.yaml) file.

## Install

```
helm install network-operator-gaudi helm/network-operator/ \
  --create-namespace --namespace network-operator \
  --set config.gaudi.enabled=true --set config.gaudi.mode=L3 --set config.gaudi.mtu=1500
```

## Uninstall

```
helm uninstall --namespace network-operator network-operator-gaudi
```
