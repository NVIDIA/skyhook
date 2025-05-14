# How-To: Provide secrets to packages

Some packages require the use of secret information to accomplish their work. This is often passwords or api keys with examples such as connecting to a private repository or mounting an authenticate nfs share. Currently, the is no mechanism for the Operator to fetch secrets and inject them into your package's container. Instead we recommend using the native Kubernetes tooling to do so. At a high level you will need to do the following:

 1. Setup a Kubernetes secret with the information you need.
 2. Set a package's environment definition to source from the secret
 3. Use the environment variables in the step scripts to do work.

## [Setup a Kubernetes secret](https://kubernetes.io/docs/concepts/configuration/secret/)

There are many ways to do this the details of which are outside the scope of this document. Some examples are:
 * [Use vault to manage secrets](https://developer.hashicorp.com/vault/tutorials/kubernetes/vault-secrets-operator)
 * Manually create a secret

## Set a package's environment definition to source from the secret

The `env` section of a package is passed directly to the pod definition when running the package. [Therefore anything you would set in kubernetes yaml you can set here.](https://kubernetes.io/docs/tasks/inject-data-application/environment-variable-expose-pod-information/) Which means it can be:

A direct key/value:
```yaml
env:
  - name: FOO
    value: bar
```

Set the value for an enviroment variable from a secret
```yaml
env:
  - name: DB_PASSWORD
    valueFrom:
      secretKeyRef:
        name: postgres-db-password
        key: db-password
```

##  Use the environment variables in the step scripts to do work.

Using the example above a script to query the database would be:
```bash
#!/bin/bash

echo "select count(*) from app.users;" | PGPASSWORD=${DB_PASSWORD} psql -h ${DB_HOST} -U ${DB_USER} -d ${DB_NAME}
```

