# exposecontroller

Automatically expose services creating ingress rules or modifying services to use kubernetes nodePort or loadBalancer service types

This is a fork from the dead project https://github.com/jenkins-x/exposecontroller in order to solve many problems

## Deployment

### From helm repository

```bash
helm repo add olli-ai https://olli-ai.github.io/helm-charts/
helm upgrade --install exposecontroller olli-ai/exposecontroller
```

### Using Helm

```bash
helm upgrade --install exposecontroller ./deploy/helm-chart/exposecontroller
```

### Manual

```bash
# Create roles and service accounts
kubectl apply -f https://raw.githubusercontent.com/olli-ai/exposecontroller/master/deploy/rbac.yaml
# Create actual deployment
kubectl apply -f https://raw.githubusercontent.com/olli-ai/exposecontroller/master/deploy/deployment.yaml
```

### In Jenkins-X environments

```bash
export VERSION=latest
# set the repository and version
sed '/\(- \|  \)name: exposecontroller/{n;N;d}' -i env/requirements.yaml
sed "s#\( *\)\(- \|  \)name: exposecontroller#\0\n\1  repository: https://olli-ai.github.io/helm-charts/\n\1  version: $VERSION#" \
  -i env/requirements.yaml
# add a name prefix for the ingress
sed -i 's/^\( *\)urltemplate:.*/\0\n\1namePrefix: exposed-/' env/values.yaml
```

### In Jenkins-X applications for preview

```bash
export VERSION=latest
# set the repository and version
sed '/\(- \|  \)name: exposecontroller/{n;N;d}' -i charts/preview/requirements.yaml
sed "s#\( *\)\(- \|  \)name: exposecontroller#\0\n\1  repository: https://olli-ai.github.io/helm-charts/\n\1  version: $VERSION#" \
  -i charts/preview/requirements.yaml
# add a name prefix for the ingress
sed -i 's/^\( *\)urltemplate:.*/\0\n\1namePrefix: exposed-/' charts/preview/values.yaml
```

## Differences with the original project

- Update everything: the project was based on a very old version of golang and kubernetes, it didn't even have go mod.
- Give up openshift: the openshift library hasn't been updated for 2 years, so give up it's support.
- Test everything: there was almost no test, and definitely nothing critical was tested. Yes, something that is supposed to make public all your services was not tested.
- Fix various bugs everywhere in the code.
- Fix incoherent annotations.
- Fix several severe bugs in `ingress` exposer:
  - ingresses were not properly updated when already existing, especially TLS configuration was not updated
  - the `fabric8.io/ingress.annotations` annotation was parsed using split, so spaces would be kept, an extra newline would result in panic, and it was impossible to set a multiline annotation
  - old ingresses would never be removed when changing the name of the ingress, or when the service is unexposed in run mode

## Usage

You can expose any service with the annotation
```yaml
metadata:
  annoations:
    fabric8.io/expose: "true"
```

The available exposers are:
- `Ingress` - [Kubernetes Ingress](http://kubernetes.io/docs/user-guide/ingress/)
- `Ambassador` - [Ambassador](https://www.getambassador.io/)
- `LoadBalancer` - Cloud provider external [load-balancer](http://kubernetes.io/docs/user-guide/load-balancer/)
- `NodePort` - Recomended for local development using minikube / minishift without Ingress or Router running. See also the [Kubernetes NodePort](http://kubernetes.io/docs/user-guide/services/#type-nodeport) documentation.

The default and most versatile exposer is the `Ingress` exposer with `nginx` class. You can configure ingress annotations to your need:
```yaml
metadata:
  annoations:
    fabric8.io/ingress.annotations: |
      kubernetes.io/ingress.class: nginx
      # SSL redirection
      nginx.ingress.kubernetes.io/ssl-redirect: "true"
      # basic auth
      nginx.ingress.kubernetes.io/auth-type: basic
      nginx.ingress.kubernetes.io/auth-secret: jx-basic-auth
      # auth through URL
      nginx.ingress.kubernetes.io/auth-url: http://my-auth-service/check
      nginx.ingress.kubernetes.io/auth-signin: https://my-auth-service.my-domain.com/singin?rd=https%3A%2F%2F$http_host$escaped_request_uri
      # Dynamic CORS Allow-Origin
      nginx.ingress.kubernetes.io/configuration-snippet: |
        # Check that the origin is allowed
        if ($http_origin ~ '\.my-domain\.com$') {
          set $allow_origin $http_origin;
        }
        if ($http_origin ~ '://localhost(:\d+)?$') {
          set $allow_origin $http_origin;
        }
        # Set CORS headers
        add_header 'Access-Control-Allow-Origin' $allow_origin always;
        add_header 'Access-Control-Allow-Credentials' 'true' always;
        add_header 'Access-Control-Allow-Methods' 'PUT, GET, POST, OPTIONS, DELETE' always;
        add_header 'Access-Control-Allow-Headers' 'DNT,X-CustomHeader,Keep-Alive,User-Agent,X-Requested-With,If-Modified-Since,Cache-Control,Content-Type,Authorization' always;
        # Directly return before authentication in case of OPTIONS request
        if ($request_method = 'OPTIONS') {
          return 204;
        }
```

## Helm configuration

You can configure the controller through `helm` values.

| Helm parameter        | Argument                  | Default                                     | Description                                                                                                   |
|-----------------------|---------------------------|---------------------------------------------|---------------------------------------------------------------------------------------------------------------|
| clean                 | --clean                   | `false`                                     | Clean exposed ingresses created by a previous run                                                             |
| daemon                | --daemon                  | `false`                                     | Run as a daemon, exposing any cleaning any created or updated service                                         |
| watchNamespaces       | --watch-namespaces        | `""`                                        | The namespace(s) to watch and expose services from                                                            |
| watchCurrentNamespace | --watch-current-namespace | `true`                                      | Watch the same namespace as the controller                                                                    |
| config.exposer        | --exposer                 | `"ingress"`                                 | The exposer to use, `"ingress"`, `"loadbalancer"`, `"nodeport"`, `"ambassador"`                               |
| config.domain         | --domain                  |                                             | The domain to expose the services with                                                                        |
| config.http           | --http                    | `false`                                     | Expose the URL with HTTP protocol even if HTTPS is vailable                                                   |
| config.internalDomain |                           |                                             | The domain to expose services with the annotation `fabric8.io/use.internal.domain: "true"`                    |
| config.pathMode       |                           |                                             | The mode for the ingress paths. If `"path"`, the services are exposed with the same domain but with `/` paths |
| config.ingressClass   |                           |                                             | The ingress class for ingresses                                                                               |
| config.urltemplate    |                           | `"{{.Service}}.{{.Namespace}}.{{.Domain}}"` | The format for ingress host, if no path mode                                                                  |
| config.tlsSecretName  |                           |                                             | The name of an existing secret for TLS certificate                                                            |
| config.tlsacme        |                           | `false`                                     | Use ACME to generate ingress TLS certificates                                                                 |
| config.tlsUseWildcard |                           | `false`                                     | ACME TLS certificates should use wildcard domain                                                              |
| config.namePrefix     | --name-prefix             | `""`                                        | The prefix to use for the created ingresses                                                                   |
| config.extravalues    |                           |                                             | Extra YAML config                                                                                             |
| timeout               | --timeout                 | `"5m"`                                      | The timeout for non-daemon run                                                                                |
| resyncPeriod          | --resync-period           | `"30m"`                                     | The resync period for the service watcher                                                                     |
| nameOverride          |                           | `{.Chart.Name}`                             | Overrides the name used in the label selector and the default name of the resources                           |
| fullnameOverride      |                           | `{.Release.Name}-{.Values.nameOverride}`    | Overrides the name of the resources                                                                           |
| annotations           |                           |                                             | The annotations to pass to the job or deployment                                                              |
| args                  |                           |                                             | An array of extra arguments to pass to the controller                                                         |
| resources             |                           | 100m CPU / 128Mi RAM                        | Configures the resources of the pod                                                                           |
| nodeSelector          |                           |                                             | Configures the nodeSelector of the pod                                                                        |
| tolerations           |                           |                                             | Configures the tolerations of the pod                                                                         |
| affinity              |                           |                                             | Configures the affinity of the pod                                                                            |

## Service annotations

You can further configure the ingress by adding those annotations to the service.

| Service annotation             | Default                     | Description                                                                                                                   |
|--------------------------------|-----------------------------|-------------------------------------------------------------------------------------------------------------------------------|
| fabric8.io/expose              |                             | `"true"` to expose this service                                                                                               |
| fabric8.io/ingress.name        | service's name              | The name of the ingress generated by the controller                                                                           |
| fabric8.io/host.name           | Generated from URL template | The hostname to use in the ingress                                                                                            |
| fabric8.io/exposePort          | first port available        | The port of the service to expose                                                                                             |
| fabric8.io/ingress.path        | `"/"`                       | The path to use in the ingress                                                                                                |
| fabric8.io/path.mode           |                             | The mode for the ingres path. If `"path"`, the services is exposed with the same domain but with `<namespace>/<service>` path |
| fabric8.io/ingress.annotations |                             | Annotations to pass to the ingress, YAML format                                                                               |
| fabric8.io/use.internal.domain |                             | If `"true"`, uses the internal domain instead of the normal domain                                                            |
| jenkins-x.io/skip.tls          |                             | If `"true"`, ignores TLS configuration of the ambassador annotation                                                           |
| fabric8.io/exposeURL           |                             | Created by the controller, writes the URL to access to the exposed service                                                    |
| fabric8.io/exposeHostNameAs    |                             | The name of the annotation where the controller should write the exposed host                                                 |

## Export info to configmaps

You can export the exposed URL to configMaps by adding annotations in those configMaps.

| ConfigMap annotation                                                 | Description                                                                                          |
|----------------------------------------------------------------------|------------------------------------------------------------------------------------------------------|
| expose.config.fabric8.io/url-key                                     | The field to export the exposed URL of the service with the same name                                |
| expose.config.fabric8.io/url-protocol                                | The field to export the exposed URL's protocol of the service with the same name                     |
| expose.config.fabric8.io/host-key                                    | The field to export the exposed URL's host of the service with the same name                         |
| expose.config.fabric8.io/path-key                                    | The field to export the exposed URL's path of the service with the same name                         |
| expose.config.fabric8.io/clusterip-key                               | The field to export the clusterIP `"10.103.44.12"` of the service with the same name                 |
| expose.config.fabric8.io/clusterip-port-key                          | The field to export the clusterIP with port `"10.103.44.12:666"` of the service with the same name   |
| expose.config.fabric8.io/clusterip-port-if-empty-key                 | The field to export the clusterIP with port of the service with the same name, if the field is empty |
| expose.config.fabric8.io/config-yaml                                 | A YAML of custom fields and values to export of the service with the same name                       |
| expose.service-key.config.fabric8.io/<service-name>                  | The field to export the exposed URL (without `/` prefix) of the named service                        |
| expose-full.service-key.config.fabric8.io/<service-name>             | The field to export the exposed URL (with `/` prefix) of the named service                           |
| expose-no-path.service-key.config.fabric8.io/<service-name>          | The field to export the exposed URL (with path set to `/`) of the named service                      |
| expose-no-protocol.service-key.config.fabric8.io/<service-name>      | The field to export the exposed URL (without protocol and without `/` prefix) of the named service   |
| expose-full-no-protocol.service-key.config.fabric8.io/<service-name> | The field to export the exposed URL (without protocol but with `/` prefix) of the named service      |
| expose-protocol.service-key.config.fabric8.io/<service-name>         | The field to export the exposed URL's protocol of the named service                                  |

  ```yaml
metadata:
  annotations:
    expose.config.fabric8.io/config-yaml: |-
      - key: app.ini
        prefix: "DOMAIN = "
        expression: host
      - key: app.ini
        prefix: "ROOT_URL = "
        expression: url
```
