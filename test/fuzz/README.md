## Fuzz

To run fuzzing against the operator:
* Deploy operator: `kubectl apply -k config/default`
* Run fuzzing: `KUBECONFIG=~/.kube/config make fuzz`
* Observe operator logs for Errors or crashes.

Only GaudiSO part of the CRD is fuzzed now.
