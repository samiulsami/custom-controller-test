### Custom Controller
A custom controller written as an exercise for learning about k8s controllers, 
using the [sample-controller](https://github.com/kubernetes/sample-controller) as a skeleton. The controller manages
resources of type "Bookstore", as described by the the CRD manifests/bookstores.calico.com_CRD.yaml 

### Running the controller

`kubectl apply -f artifacts`

`go build . `

`./sample-controller -kubeconfig=/path/to/kubeconfig`

`kc port-forward service/bookstorecontrollertestservice 30000:3000`

Now the [GolangBookstoreAPI](https://github.com/samiulsami/GolangBookstoreAPI/) can be accessed from localhost:30000

### References 

- https://github.com/kubernetes/sample-controller
- https://github.com/AbdullahAlShaad/bookstore-sample-controller
- https://github.com/ArnobKumarSaha/custom-controller/tree/main