{
   "guestbook-ui": [
      {
         "apiVersion": "v1",
         "kind": "Service",
         "metadata": {
            "name": "guestbook-ui"
         },
         "spec": {
            "ports": [
               {
                  "port": 80,
                  "targetPort": 80
               }
            ],
            "selector": {
               "app": "guestbook-ui"
            },
            "type": "NodePort"
         }
      },
      {
         "apiVersion": "apps/v1beta1",
         "kind": "Deployment",
         "metadata": {
            "name": "guestbook-ui"
         },
         "spec": {
            "replicas": 1,
            "template": {
               "metadata": {
                  "labels": {
                     "app": "guestbook-ui"
                  }
               },
               "spec": {
                  "containers": [
                     {
                        "image": "gcr.io/heptio-images/ks-guestbook-demo:0.1",
                        "name": "guestbook-ui",
                        "ports": [
                           {
                              "containerPort": 80
                           }
                        ]
                     }
                  ]
               }
            }
         }
      }
   ]
}